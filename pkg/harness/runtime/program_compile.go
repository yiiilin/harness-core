package runtime

import (
	"errors"
	"fmt"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/capability"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
)

var ErrProgramStepNotCompiled = errors.New("program step must be compiled before execution")
var ErrProgramFanoutUnsupported = ErrProgramMultiTargetUnsupported

const (
	programCompiledMetadataKey   = "program_compiled"
	programParentStepMetadataKey = "program_parent_step_id"
	programDependsOnMetadataKey  = "program_depends_on"
)

func normalizePlanStepsForStorage(steps []plan.StepSpec) ([]plan.StepSpec, error) {
	return normalizePlanStepsForStorageWithCapabilityView(steps, capability.View{})
}

func normalizePlanStepsForStorageWithCapabilityView(steps []plan.StepSpec, view capability.View) ([]plan.StepSpec, error) {
	out := make([]plan.StepSpec, 0, len(steps))
	for _, step := range steps {
		expanded, err := expandProgramStep(step, view)
		if err != nil {
			return nil, err
		}
		out = append(out, expanded...)
	}
	return out, nil
}

func expandProgramStep(step plan.StepSpec, view capability.View) ([]plan.StepSpec, error) {
	program, ok := execution.ProgramFromStep(step)
	if !ok {
		if view.ViewID != "" {
			pinned, err := pinStepToCapabilityView(step, view)
			if err != nil {
				return nil, err
			}
			return []plan.StepSpec{pinned}, nil
		}
		return []plan.StepSpec{step}, nil
	}

	ordered, err := orderedProgramNodes(*program)
	if err != nil {
		return nil, err
	}
	if len(ordered) == 0 {
		return nil, fmt.Errorf("program step %q has no nodes", step.StepID)
	}

	out := make([]plan.StepSpec, 0, len(ordered))
	for _, node := range ordered {
		compiled, err := compileAttachedProgramNodeSteps(step, *program, node, view)
		if err != nil {
			return nil, err
		}
		out = append(out, compiled...)
	}
	return out, nil
}

func orderedProgramNodes(program execution.Program) ([]execution.ProgramNode, error) {
	index := make(map[string]int, len(program.Nodes))
	dependents := make(map[string][]string, len(program.Nodes))
	indegree := make(map[string]int, len(program.Nodes))

	for i, node := range program.Nodes {
		if node.NodeID == "" {
			return nil, fmt.Errorf("%w at index %d", ErrProgramMissingNodeID, i)
		}
		if _, exists := index[node.NodeID]; exists {
			return nil, fmt.Errorf("%w %q", ErrProgramDuplicateNodeID, node.NodeID)
		}
		index[node.NodeID] = i
	}

	for _, node := range program.Nodes {
		indegree[node.NodeID] = len(node.DependsOn)
		for _, dep := range node.DependsOn {
			if _, ok := index[dep]; !ok {
				return nil, fmt.Errorf("%w: node %q depends on %q", ErrProgramDependencyNotFound, node.NodeID, dep)
			}
			dependents[dep] = append(dependents[dep], node.NodeID)
		}
	}

	queue := make([]string, 0, len(program.Nodes))
	for _, node := range program.Nodes {
		if indegree[node.NodeID] == 0 {
			queue = append(queue, node.NodeID)
		}
	}

	out := make([]execution.ProgramNode, 0, len(program.Nodes))
	for len(queue) > 0 {
		nodeID := queue[0]
		queue = queue[1:]
		out = append(out, program.Nodes[index[nodeID]])
		for _, dependent := range dependents[nodeID] {
			indegree[dependent]--
			if indegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}

	if len(out) != len(program.Nodes) {
		return nil, ErrProgramCycleDetected
	}
	return out, nil
}

func compileAttachedProgramNodeSteps(parent plan.StepSpec, program execution.Program, node execution.ProgramNode, view capability.View) ([]plan.StepSpec, error) {
	targets, err := targetsForProgramNode(node)
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		step, err := compileAttachedProgramNodeStep(parent, program, node, nil, 0, 0, "", view)
		if err != nil {
			return nil, err
		}
		return []plan.StepSpec{step}, nil
	}
	out := make([]plan.StepSpec, 0, len(targets))
	aggregateID := compiledAttachedProgramNodeStepID(parent.StepID, program, node.NodeID, nil)
	for i := range targets {
		target := targets[i]
		step, err := compileAttachedProgramNodeStep(parent, program, node, &target, i+1, len(targets), aggregateID, view)
		if err != nil {
			return nil, err
		}
		out = append(out, step)
	}
	return out, nil
}

func compileAttachedProgramNodeStep(parent plan.StepSpec, program execution.Program, node execution.ProgramNode, target *execution.Target, index, total int, aggregateID string, view capability.View) (plan.StepSpec, error) {
	args := cloneProgramArgs(node.Action.Args)
	unresolvedBindings, err := applyCompiledProgramBindings(args, node.InputBinds)
	if err != nil {
		return plan.StepSpec{}, fmt.Errorf("%w %q", err, node.NodeID)
	}

	compiled := plan.StepSpec{
		StepID: compiledAttachedProgramNodeStepID(parent.StepID, program, node.NodeID, target),
		Title:  compiledAttachedProgramNodeTitle(parent.Title, node, target),
		Action: action.Spec{
			ToolName:    node.Action.ToolName,
			ToolVersion: node.Action.ToolVersion,
			Args:        args,
		},
		Verify:   compileProgramNodeVerifySpec(node, total),
		OnFail:   compileAttachedProgramNodeOnFail(parent.OnFail, node, total),
		Metadata: cloneProgramMetadata(parent.Metadata),
	}
	if view.ViewID != "" {
		var err error
		compiled, err = pinStepToCapabilityView(compiled, view)
		if err != nil {
			return plan.StepSpec{}, err
		}
	}
	if compiled.Metadata == nil {
		compiled.Metadata = map[string]any{}
	}
	compiled.Metadata[programCompiledMetadataKey] = true
	compiled.Metadata[execution.ProgramMetadataKeyID] = compiledProgramID(parent, program)
	compiled.Metadata[programParentStepMetadataKey] = parent.StepID
	compiled.Metadata[execution.ProgramMetadataKeyNodeID] = node.NodeID
	if len(node.DependsOn) > 0 {
		compiled.Metadata[programDependsOnMetadataKey] = append([]string(nil), node.DependsOn...)
	}
	compiled.Metadata = applyProgramNodeAggregateMetadata(
		compiled.Metadata,
		aggregateID,
		compiledProgramID(parent, program),
		node.NodeID,
		firstNonEmptyProgramValue(node.Title, node.Action.ToolName, node.NodeID),
		normalizedProgramTargetFailureStrategy(node.Targeting),
		total,
	)
	compiled.Metadata = applyProgramVerifyMetadata(compiled.Metadata, node, total)
	if target != nil {
		compiled.Metadata = execution.ApplyTargetMetadata(compiled.Metadata, *target, index, total)
		compiled.Action.Args[execution.TargetArgKey] = execution.TargetArgValue(*target)
	}
	if len(unresolvedBindings) > 0 {
		compiled = execution.AttachProgramInputBindings(compiled, unresolvedBindings)
	}
	return compiled, nil
}

func cloneProgramArgs(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneProgramMetadata(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func compiledAttachedProgramNodeStepID(parentStepID string, program execution.Program, nodeID string, target *execution.Target) string {
	base := parentStepID + "__" + nodeID
	if program.ProgramID != "" {
		base = parentStepID + "__" + program.ProgramID + "__" + nodeID
	}
	if target == nil || target.TargetID == "" {
		return base
	}
	return base + "__" + target.TargetID
}

func compiledAttachedProgramNodeTitle(parentTitle string, node execution.ProgramNode, target *execution.Target) string {
	nodeTitle := node.Title
	nodeID := node.NodeID
	switch {
	case parentTitle != "" && nodeTitle != "":
		if target != nil {
			return parentTitle + ": " + compiledRuntimeProgramNodeTitle(node, target)
		}
		return parentTitle + ": " + nodeTitle
	case nodeTitle != "":
		if target != nil {
			return compiledRuntimeProgramNodeTitle(node, target)
		}
		return nodeTitle
	case parentTitle != "":
		if target != nil {
			return parentTitle + ": " + compiledRuntimeProgramNodeTitle(node, target)
		}
		return parentTitle + ": " + nodeID
	default:
		if target != nil {
			return compiledRuntimeProgramNodeTitle(node, target)
		}
		return nodeID
	}
}

func compiledProgramID(step plan.StepSpec, program execution.Program) string {
	if program.ProgramID != "" {
		return program.ProgramID
	}
	return step.StepID
}
