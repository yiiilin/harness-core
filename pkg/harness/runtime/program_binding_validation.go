package runtime

import (
	"fmt"
	"strings"

	"github.com/yiiilin/harness-core/pkg/harness/execution"
)

func validateProgramBindingDependencies(nodes []execution.ProgramNode, knownNodes map[string]struct{}) error {
	dependenciesByNode := make(map[string][]string, len(nodes))
	for _, node := range nodes {
		dependenciesByNode[node.NodeID] = append([]string(nil), node.DependsOn...)
	}
	for _, node := range nodes {
		reachableDeps := reachableProgramDependencies(node.NodeID, dependenciesByNode)
		for _, binding := range node.InputBinds {
			stepID := referencedProgramBindingStepID(binding)
			if stepID == "" {
				continue
			}
			if _, ok := knownNodes[stepID]; !ok {
				return fmt.Errorf("%w: node %q binding %q references step %q", ErrProgramDependencyNotFound, node.NodeID, binding.Name, stepID)
			}
			if _, ok := reachableDeps[stepID]; !ok {
				return fmt.Errorf("%w: node %q binding %q references step %q", ErrProgramBindingDependencyMissing, node.NodeID, binding.Name, stepID)
			}
		}
	}
	return nil
}

func reachableProgramDependencies(nodeID string, dependenciesByNode map[string][]string) map[string]struct{} {
	reachable := map[string]struct{}{}
	stack := append([]string(nil), dependenciesByNode[nodeID]...)
	for len(stack) > 0 {
		last := len(stack) - 1
		depID := stack[last]
		stack = stack[:last]
		if _, ok := reachable[depID]; ok {
			continue
		}
		reachable[depID] = struct{}{}
		stack = append(stack, dependenciesByNode[depID]...)
	}
	return reachable
}

func referencedProgramBindingStepID(binding execution.ProgramInputBinding) string {
	switch binding.Kind {
	case execution.ProgramInputBindingOutputRef:
		if binding.Ref == nil {
			return ""
		}
		return strings.TrimSpace(binding.Ref.StepID)
	case execution.ProgramInputBindingRuntimeHandleRef:
		if binding.RuntimeHandle == nil {
			return ""
		}
		return strings.TrimSpace(binding.RuntimeHandle.StepID)
	default:
		return ""
	}
}
