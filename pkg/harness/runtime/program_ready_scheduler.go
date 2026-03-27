package runtime

import (
	"context"

	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

type programReadyRound struct {
	GroupID        string
	AnchorStepID   string
	MaxConcurrency int
	Steps          []plan.StepSpec
}

type programNodeRoundState struct {
	steps           int
	completed       int
	acceptedFailed  int
	terminalFailed  int
	retryableFailed int
	allowPartial    bool
}

func programReadyRoundForSelection(spec plan.Spec, selected plan.StepSpec, budgets LoopBudgets) (programReadyRound, bool) {
	groupID, ok := programGroupIDFromStep(selected)
	if !ok {
		return programReadyRound{}, false
	}
	nodeStateCache := map[string]map[string]programNodeRoundState{}
	nodeStates := programNodeStatesForGroup(spec, groupID, budgets)
	nodeStateCache[groupID] = nodeStates
	candidates := make([]plan.StepSpec, 0)
	for _, step := range spec.Steps {
		if !programStepInGroup(step, groupID) {
			continue
		}
		if !programStepEligibleForReadyRound(step) {
			continue
		}
		if !programStepRunnableForReadyRound(step) {
			continue
		}
		if !programStepDependenciesSatisfiedWithCache(spec, step, budgets, nodeStateCache) {
			continue
		}
		candidates = append(candidates, step)
	}
	if len(candidates) <= 1 {
		return programReadyRound{}, false
	}
	if programReadyRoundDistinctNodeCount(candidates) <= 1 {
		return programReadyRound{}, false
	}
	maxConcurrency := programReadyRoundMaxConcurrency(selected, len(candidates))
	if maxConcurrency <= 1 {
		return programReadyRound{}, false
	}
	return programReadyRound{
		GroupID:        groupID,
		AnchorStepID:   selected.StepID,
		MaxConcurrency: maxConcurrency,
		Steps:          candidates,
	}, true
}

func programReadyRoundDistinctNodeCount(steps []plan.StepSpec) int {
	nodeIDs := map[string]struct{}{}
	for _, step := range steps {
		nodeID, _ := step.Metadata[execution.ProgramMetadataKeyNodeID].(string)
		if nodeID == "" {
			continue
		}
		nodeIDs[nodeID] = struct{}{}
	}
	return len(nodeIDs)
}

func (s *Service) tryRunProgramReadyRound(ctx context.Context, sessionID, leaseID string, state session.State, latest plan.Spec, selected plan.StepSpec) (fanoutRoundOutput, bool, error) {
	round, ok := programReadyRoundForSelection(latest, selected, s.LoopBudgets)
	if !ok {
		return fanoutRoundOutput{}, false, nil
	}
	out, ok, err := s.runFanoutRound(ctx, sessionID, leaseID, state, latest, fanoutRound{
		AnchorStepID:   round.AnchorStepID,
		AllowSingle:    true,
		MaxConcurrency: round.MaxConcurrency,
		Steps:          round.Steps,
	})
	if err != nil {
		return fanoutRoundOutput{}, false, err
	}
	if !ok {
		return fanoutRoundOutput{}, false, nil
	}
	return out, true, nil
}

func programGroupIDFromStep(step plan.StepSpec) (string, bool) {
	if len(step.Metadata) == 0 {
		return "", false
	}
	groupID, _ := step.Metadata[programGroupMetadataKey].(string)
	if groupID == "" {
		nodeID, _ := step.Metadata[execution.ProgramMetadataKeyNodeID].(string)
		if nodeID == "" {
			return "", false
		}
		parentStepID, _ := step.Metadata[programParentStepMetadataKey].(string)
		programID, _ := step.Metadata[execution.ProgramMetadataKeyID].(string)
		switch {
		case parentStepID != "" && programID != "":
			return parentStepID + "__" + programID, true
		case parentStepID != "":
			return parentStepID, true
		case programID != "":
			return runtimeProgramGroupID(execution.Program{ProgramID: programID}), true
		case step.PlanID != "":
			return step.PlanID, true
		default:
			return "", false
		}
	}
	return groupID, true
}

func programStepInGroup(step plan.StepSpec, groupID string) bool {
	if groupID == "" {
		return false
	}
	candidate, ok := programGroupIDFromStep(step)
	return ok && candidate == groupID
}

func programStepEligibleForReadyRound(step plan.StepSpec) bool {
	if len(step.Metadata) == 0 {
		return false
	}
	if _, ok := step.Metadata[execution.ProgramMetadataKeyNodeID].(string); !ok {
		return false
	}
	return true
}

func programStepRunnableForReadyRound(step plan.StepSpec) bool {
	switch step.Status {
	case "", plan.StepPending:
		return true
	default:
		return false
	}
}

func programNodeStatesForGroup(spec plan.Spec, groupID string, budgets LoopBudgets) map[string]programNodeRoundState {
	states := map[string]programNodeRoundState{}
	for _, step := range spec.Steps {
		if !programStepInGroup(step, groupID) {
			continue
		}
		nodeID, _ := step.Metadata[execution.ProgramMetadataKeyNodeID].(string)
		if nodeID == "" {
			continue
		}
		state := states[nodeID]
		state.steps++
		if programStepAllowsPartialDependency(step) {
			state.allowPartial = true
		}
		switch step.Status {
		case plan.StepCompleted:
			state.completed++
		case plan.StepFailed:
			if programStepFailureCanSatisfyDependency(step, budgets) {
				state.acceptedFailed++
			} else if programStepFailureRetryable(step, budgets) {
				state.retryableFailed++
			} else {
				state.terminalFailed++
			}
		}
		states[nodeID] = state
	}
	return states
}

func programStepDependenciesSatisfied(step plan.StepSpec, nodeStates map[string]programNodeRoundState) bool {
	for _, dependency := range programStepDependenciesFromMetadata(step) {
		state, ok := nodeStates[dependency]
		if !ok || !programNodeDependencySatisfied(state) {
			return false
		}
	}
	return true
}

func programStepDependenciesSatisfiedWithCache(spec plan.Spec, step plan.StepSpec, budgets LoopBudgets, cache map[string]map[string]programNodeRoundState) bool {
	groupID, ok := programGroupIDFromStep(step)
	if !ok {
		return true
	}
	nodeStates, ok := cache[groupID]
	if !ok {
		nodeStates = programNodeStatesForGroup(spec, groupID, budgets)
		cache[groupID] = nodeStates
	}
	return programStepDependenciesSatisfied(step, nodeStates)
}

func programNodeDependencySatisfied(state programNodeRoundState) bool {
	if state.steps == 0 {
		return false
	}
	if state.completed == state.steps {
		return true
	}
	if !state.allowPartial || state.completed == 0 || state.terminalFailed > 0 || state.retryableFailed > 0 {
		return false
	}
	return state.completed+state.acceptedFailed == state.steps
}

func programStepFailureCanSatisfyDependency(step plan.StepSpec, budgets LoopBudgets) bool {
	if step.Status != plan.StepFailed {
		return false
	}
	if !programStepAllowsPartialDependency(step) {
		return false
	}
	return step.Attempt >= allowedAttempts(step, budgets)
}

func programStepFailureRetryable(step plan.StepSpec, budgets LoopBudgets) bool {
	if step.Status != plan.StepFailed {
		return false
	}
	if normalizedOnFailStrategy(step) == "abort" {
		return false
	}
	return step.Attempt < allowedAttempts(step, budgets)
}

func programStepAllowsPartialDependency(step plan.StepSpec) bool {
	if len(step.Metadata) == 0 {
		return false
	}
	_, scope, ok := execution.AggregateRefFromMetadata(step.Metadata)
	if !ok || scope != execution.AggregateScopeTargetFanout {
		return false
	}
	strategy, _ := step.Metadata[execution.AggregateMetadataKeyStrategy].(string)
	return execution.TargetFailureStrategy(strategy) == execution.TargetFailureContinue
}

func programNodeDependencyTerminallyUnsatisfied(state programNodeRoundState) bool {
	if state.steps == 0 {
		return true
	}
	if programNodeDependencySatisfied(state) {
		return false
	}
	if state.retryableFailed > 0 {
		return false
	}
	return state.completed+state.acceptedFailed+state.terminalFailed == state.steps
}

func programPlanDependencyDeadlocked(spec plan.Spec, budgets LoopBudgets) bool {
	nodeStateCache := map[string]map[string]programNodeRoundState{}
	for _, step := range spec.Steps {
		if step.Status != "" && step.Status != plan.StepPending {
			continue
		}
		if programStepDependenciesSatisfiedWithCache(spec, step, budgets, nodeStateCache) {
			continue
		}
		groupID, ok := programGroupIDFromStep(step)
		if !ok {
			continue
		}
		nodeStates, ok := nodeStateCache[groupID]
		if !ok {
			nodeStates = programNodeStatesForGroup(spec, groupID, budgets)
			nodeStateCache[groupID] = nodeStates
		}
		for _, dependency := range programStepDependenciesFromMetadata(step) {
			if programNodeDependencyTerminallyUnsatisfied(nodeStates[dependency]) {
				return true
			}
		}
	}
	return false
}

func programStepDependenciesFromMetadata(step plan.StepSpec) []string {
	if len(step.Metadata) == 0 {
		return nil
	}
	raw, ok := step.Metadata[programDependsOnMetadataKey]
	if !ok || raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			value, _ := item.(string)
			if value == "" {
				continue
			}
			out = append(out, value)
		}
		return out
	default:
		return nil
	}
}
