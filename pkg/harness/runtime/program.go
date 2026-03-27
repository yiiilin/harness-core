package runtime

import (
	"context"
	"slices"

	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

const (
	programPlanReasonPrefix = "runtime/program"
)

func (s *Service) CreatePlanFromProgram(sessionID, changeReason string, program execution.Program) (plan.Spec, error) {
	steps, err := s.stepsFromProgram(context.Background(), sessionID, program)
	if err != nil {
		return plan.Spec{}, err
	}
	return s.CreatePlan(sessionID, firstNonEmptyProgramValue(changeReason, programChangeReason(program)), steps)
}

func (s *Service) RunProgram(ctx context.Context, sessionID string, program execution.Program) (SessionRunOutput, error) {
	if _, err := s.CreatePlanFromProgram(sessionID, "", program); err != nil {
		return SessionRunOutput{}, err
	}
	return s.RunSession(ctx, sessionID)
}

func (s *Service) stepsFromProgram(ctx context.Context, sessionID string, program execution.Program) ([]plan.StepSpec, error) {
	if len(program.Nodes) == 0 {
		return nil, nil
	}

	nodesByID := make(map[string]execution.ProgramNode, len(program.Nodes))
	knownNodes := make(map[string]struct{}, len(program.Nodes))
	nodeOrder := make(map[string]int, len(program.Nodes))
	indegree := make(map[string]int, len(program.Nodes))
	dependents := make(map[string][]string, len(program.Nodes))

	for i, node := range program.Nodes {
		if node.NodeID == "" {
			return nil, ErrProgramMissingNodeID
		}
		if _, exists := nodesByID[node.NodeID]; exists {
			return nil, ErrProgramDuplicateNodeID
		}
		nodesByID[node.NodeID] = node
		knownNodes[node.NodeID] = struct{}{}
		nodeOrder[node.NodeID] = i
		indegree[node.NodeID] = 0
	}

	for _, entryID := range program.EntryNodes {
		if _, ok := nodesByID[entryID]; !ok {
			return nil, ErrProgramEntryNodeNotFound
		}
	}

	for _, node := range program.Nodes {
		for _, depID := range node.DependsOn {
			if _, ok := nodesByID[depID]; !ok {
				return nil, ErrProgramDependencyNotFound
			}
			indegree[node.NodeID]++
			dependents[depID] = append(dependents[depID], node.NodeID)
		}
	}
	if err := validateProgramBindingDependencies(program.Nodes, knownNodes); err != nil {
		return nil, err
	}

	ready := make([]string, 0, len(program.Nodes))
	for _, node := range program.Nodes {
		if indegree[node.NodeID] == 0 {
			ready = append(ready, node.NodeID)
		}
	}
	slices.SortStableFunc(ready, func(a, b string) int {
		return nodeOrder[a] - nodeOrder[b]
	})

	steps := make([]plan.StepSpec, 0, len(program.Nodes))
	processedNodes := 0
	for len(ready) > 0 {
		nodeID := ready[0]
		ready = ready[1:]

		node := nodesByID[nodeID]
		processedNodes++
		compiled, err := s.expandProgramNodeSteps(ctx, sessionID, program, node)
		if err != nil {
			return nil, err
		}
		steps = append(steps, compiled...)

		for _, nextID := range dependents[nodeID] {
			indegree[nextID]--
			if indegree[nextID] == 0 {
				ready = append(ready, nextID)
			}
		}
		slices.SortStableFunc(ready, func(a, b string) int {
			return nodeOrder[a] - nodeOrder[b]
		})
	}

	if processedNodes != len(program.Nodes) {
		return nil, ErrProgramCycleDetected
	}
	return steps, nil
}

func (s *Service) expandProgramNodeSteps(ctx context.Context, sessionID string, program execution.Program, node execution.ProgramNode) ([]plan.StepSpec, error) {
	targets, err := s.resolvedTargetsForProgramNode(ctx, sessionID, program, node)
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		step, err := stepFromProgramNode(program, node, nil, 0, 0, "")
		if err != nil {
			return nil, err
		}
		return []plan.StepSpec{step}, nil
	}
	out := make([]plan.StepSpec, 0, len(targets))
	aggregateID := compiledRuntimeProgramNodeStepID(program, node.NodeID, nil)
	for i := range targets {
		target := targets[i]
		step, err := stepFromProgramNode(program, node, &target, i+1, len(targets), aggregateID)
		if err != nil {
			return nil, err
		}
		out = append(out, step)
	}
	return out, nil
}

func (s *Service) resolvedTargetsForProgramNode(ctx context.Context, sessionID string, program execution.Program, node execution.ProgramNode) ([]execution.Target, error) {
	if node.Targeting == nil {
		return nil, nil
	}
	switch node.Targeting.Mode {
	case execution.TargetSelectionFanoutAll:
		if s.TargetResolver == nil {
			return nil, ErrProgramTargetDiscoveryUnsupported
		}
		state, err := s.getSessionRecord(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		var spec task.Record
		if state.TaskID != "" {
			spec, err = s.getTaskRecord(ctx, state.TaskID)
			if err != nil {
				return nil, err
			}
		}
		targets, err := s.TargetResolver.ResolveTargets(ctx, state, spec, program, node)
		if err != nil {
			return nil, err
		}
		if len(targets) == 0 {
			return nil, ErrProgramTargetDiscoveryUnsupported
		}
		return append([]execution.Target(nil), targets...), nil
	}
	return targetsForProgramNode(node)
}

func stepFromProgramNode(program execution.Program, node execution.ProgramNode, target *execution.Target, index, total int, aggregateID string) (plan.StepSpec, error) {
	step := plan.StepSpec{
		StepID:   compiledRuntimeProgramNodeStepID(program, node.NodeID, target),
		Title:    compiledRuntimeProgramNodeTitle(node, target),
		Action:   node.Action,
		Verify:   compileProgramNodeVerifySpec(node, total),
		OnFail:   compileRuntimeProgramNodeOnFail(node, total),
		Status:   plan.StepPending,
		Metadata: cloneAnyMap(node.Metadata),
	}
	step.Action.Args = cloneAnyMap(node.Action.Args)
	unresolvedBindings, err := applyCompiledProgramBindings(step.Action.Args, node.InputBinds)
	if err != nil {
		return plan.StepSpec{}, err
	}
	if len(unresolvedBindings) > 0 {
		step = execution.AttachProgramInputBindings(step, unresolvedBindings)
	}
	step.Metadata = applyProgramLineageMetadata(
		step.Metadata,
		program.ProgramID,
		runtimeProgramGroupID(program),
		"",
		node.NodeID,
		node.DependsOn,
	)
	step.Metadata = applyProgramConcurrencyMetadata(step.Metadata, program.Concurrency, node.Concurrency)
	step.Metadata = applyProgramNodeAggregateMetadata(
		step.Metadata,
		aggregateID,
		program.ProgramID,
		node.NodeID,
		firstNonEmptyProgramValue(node.Title, node.Action.ToolName, node.NodeID),
		normalizedProgramTargetFailureStrategy(node.Targeting),
		total,
		normalizedProgramTargetMaxConcurrency(node.Targeting, total),
	)
	step.Metadata = applyProgramVerifyMetadata(step.Metadata, node, total)
	if target != nil {
		step.Metadata = execution.ApplyTargetMetadata(step.Metadata, *target, index, total)
		step.Action.Args[execution.TargetArgKey] = execution.TargetArgValue(*target)
	}
	return step, nil
}

func targetsForProgramNode(node execution.ProgramNode) ([]execution.Target, error) {
	if node.Targeting == nil {
		return nil, nil
	}
	switch node.Targeting.Mode {
	case execution.TargetSelectionFanoutAll:
		return nil, ErrProgramTargetDiscoveryUnsupported
	}
	if len(node.Targeting.Targets) == 0 {
		if node.Targeting.MultiTargetRequested() {
			return nil, ErrProgramMultiTargetUnsupported
		}
		return nil, nil
	}
	return append([]execution.Target(nil), node.Targeting.Targets...), nil
}

func compiledRuntimeProgramNodeStepID(program execution.Program, nodeID string, target *execution.Target) string {
	base := nodeID
	if program.ProgramID != "" {
		base = program.ProgramID + "__" + nodeID
	}
	if target == nil || target.TargetID == "" {
		return base
	}
	return base + "__" + target.TargetID
}

func compiledRuntimeProgramNodeTitle(node execution.ProgramNode, target *execution.Target) string {
	title := firstNonEmptyProgramValue(node.Title, node.Action.ToolName, node.NodeID)
	if target == nil {
		return title
	}
	label := firstNonEmptyProgramValue(target.DisplayName, target.TargetID)
	if label == "" {
		return title
	}
	return title + " @ " + label
}

func programChangeReason(program execution.Program) string {
	if program.ProgramID == "" {
		return programPlanReasonPrefix
	}
	return programPlanReasonPrefix + ":" + program.ProgramID
}

func firstNonEmptyProgramValue(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func runtimeProgramGroupID(program execution.Program) string {
	return programChangeReason(program)
}
