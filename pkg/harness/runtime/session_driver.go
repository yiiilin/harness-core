package runtime

import (
	"context"
	"errors"

	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

const (
	runSessionPlanReason = "runtime/session-driver"
	replanSessionReason  = "runtime/replan"
)

type SessionRunOutput struct {
	Session    session.State               `json:"session"`
	Plan       *plan.Spec                  `json:"plan,omitempty"`
	Aggregates []execution.AggregateResult `json:"aggregates,omitempty"`
	Executions []StepRunOutput             `json:"executions,omitempty"`
}

type sessionStepSelection struct {
	Step          plan.StepSpec
	HasStep       bool
	NeedsPlanning bool
}

func (s *Service) runSession(ctx context.Context, sessionID, leaseID string) (SessionRunOutput, error) {
	out := SessionRunOutput{}
	for {
		state, err := s.ensureSessionLease(sessionID, leaseID)
		if err != nil {
			return SessionRunOutput{}, err
		}
		out.Session = state

		latest, ok, err := s.latestPlanForSession(ctx, sessionID)
		if err != nil {
			return SessionRunOutput{}, err
		}
		if ok {
			out.Plan = planPointer(latest)
		}

		if isTerminalPhase(state.Phase) {
			return populateSessionRunAggregates(out), nil
		}
		if state.ExecutionState == session.ExecutionBlocked {
			return populateSessionRunAggregates(out), nil
		}
		if state.PendingApprovalID != "" {
			resumed, handled, err := s.resolvePendingApprovalForSession(ctx, sessionID, leaseID)
			if err != nil {
				return SessionRunOutput{}, err
			}
			if handled {
				if resumed == nil {
					return populateSessionRunAggregates(out), nil
				}
				out.Executions = append(out.Executions, *resumed)
				out.Session = resumed.Session
				if resumed.UpdatedPlan != nil {
					out.Plan = resumed.UpdatedPlan
				}
				s.compactSessionContextBestEffort(ctx, sessionID, CompactionTriggerExecute)
				if isTerminalPhase(resumed.Session.Phase) || resumed.Session.PendingApprovalID != "" {
					return populateSessionRunAggregates(out), nil
				}
				continue
			}
		}

		if !ok {
			planned, err := s.createDriverPlan(ctx, sessionID, runSessionPlanReason)
			if err != nil {
				return SessionRunOutput{}, err
			}
			out.Plan = planPointer(planned)
			latest = planned
		}

		pinnedPlan := latest
		if pinned, pinnedOK, err := s.pinnedPlanForSession(ctx, state); err != nil {
			return SessionRunOutput{}, err
		} else if pinnedOK {
			pinnedPlan = pinned
		}

		selection := selectNextStepForSession(state, pinnedPlan, latest, s.LoopBudgets)
		if selection.NeedsPlanning {
			planned, err := s.createDriverPlan(ctx, sessionID, replanSessionReason)
			if err != nil {
				return SessionRunOutput{}, err
			}
			out.Plan = planPointer(planned)
			continue
		}
		if !selection.HasStep {
			outcome := planExecutionOutcomeForSpec(latest, s.LoopBudgets)
			if !outcome.Fail && programPlanDependencyDeadlocked(latest, s.LoopBudgets) {
				outcome.Fail = true
				outcome.Continue = false
				outcome.Reason = "program dependency failed"
			}
			if outcome.Fail {
				reconciled, handled, err := s.reconcileNoSelectionPlanFailure(ctx, sessionID, leaseID, state, latest, outcome)
				if err != nil {
					return SessionRunOutput{}, err
				}
				if handled {
					return mergeSessionRunOutputs(out, reconciled), nil
				}
			}
			return populateSessionRunAggregates(out), nil
		}

		if roundOut, handled, err := s.tryRunProgramReadyRound(ctx, sessionID, leaseID, state, latest, selection.Step); err != nil {
			return SessionRunOutput{}, err
		} else if handled {
			out.Executions = append(out.Executions, roundOut.Executions...)
			out.Session = roundOut.Session
			if roundOut.UpdatedPlan != nil {
				out.Plan = roundOut.UpdatedPlan
			}
			s.compactSessionContextBestEffort(ctx, sessionID, CompactionTriggerExecute)
			if isTerminalPhase(roundOut.Session.Phase) || roundOut.Session.PendingApprovalID != "" {
				return populateSessionRunAggregates(out), nil
			}
			continue
		}
		if roundOut, handled, err := s.tryRunFanoutRound(ctx, sessionID, leaseID, state, latest, selection.Step); err != nil {
			return SessionRunOutput{}, err
		} else if handled {
			out.Executions = append(out.Executions, roundOut.Executions...)
			out.Session = roundOut.Session
			if roundOut.UpdatedPlan != nil {
				out.Plan = roundOut.UpdatedPlan
			}
			s.compactSessionContextBestEffort(ctx, sessionID, CompactionTriggerExecute)
			if isTerminalPhase(roundOut.Session.Phase) || roundOut.Session.PendingApprovalID != "" {
				return populateSessionRunAggregates(out), nil
			}
			continue
		}

		stepOut, err := s.runStepWithDecision(ctx, sessionID, leaseID, selection.Step, nil, nil)
		if err != nil {
			if errors.Is(err, ErrStepBackoffActive) {
				return out, nil
			}
			return SessionRunOutput{}, err
		}
		out.Executions = append(out.Executions, stepOut)
		out.Session = stepOut.Session
		if stepOut.UpdatedPlan != nil {
			out.Plan = stepOut.UpdatedPlan
		}
		s.compactSessionContextBestEffort(ctx, sessionID, CompactionTriggerExecute)
		if isTerminalPhase(stepOut.Session.Phase) || stepOut.Session.PendingApprovalID != "" {
			return populateSessionRunAggregates(out), nil
		}
	}
}

func (s *Service) createDriverPlan(ctx context.Context, sessionID, changeReason string) (plan.Spec, error) {
	pl, _, err := s.CreatePlanFromPlanner(ctx, sessionID, changeReason, 0)
	if err != nil {
		return plan.Spec{}, err
	}
	return pl, nil
}

func selectNextStepForSession(state session.State, pinned plan.Spec, latest plan.Spec, budgets LoopBudgets) sessionStepSelection {
	if step, ok := pinnedStepForSession(state, pinned, budgets); ok {
		return sessionStepSelection{Step: step, HasStep: true}
	}
	if step, ok := firstPendingPlanStep(latest, budgets); ok {
		return sessionStepSelection{Step: step, HasStep: true}
	}
	if step, ok := firstFailedPlanStep(latest, budgets); ok {
		switch normalizedOnFailStrategy(step) {
		case "replan":
			return sessionStepSelection{NeedsPlanning: true}
		case "abort":
			return sessionStepSelection{}
		default:
			return sessionStepSelection{Step: step, HasStep: true}
		}
	}
	return sessionStepSelection{}
}

func pinnedStepForSession(state session.State, latest plan.Spec, budgets LoopBudgets) (plan.StepSpec, bool) {
	if state.InFlightStepID != "" {
		if step, ok := executableStepByID(latest, state.InFlightStepID, budgets); ok {
			return step, true
		}
	}
	if state.CurrentStepID != "" {
		if step, ok := executableStepByID(latest, state.CurrentStepID, budgets); ok {
			return step, true
		}
	}
	return plan.StepSpec{}, false
}

func executableStepByID(latest plan.Spec, stepID string, budgets LoopBudgets) (plan.StepSpec, bool) {
	if stepID == "" {
		return plan.StepSpec{}, false
	}
	step, ok := findPlanStepByID(latest, stepID)
	if !ok {
		return plan.StepSpec{}, false
	}
	nodeStateCache := map[string]map[string]programNodeRoundState{}
	switch step.Status {
	case "", plan.StepPending, plan.StepRunning:
		if step.Status != plan.StepRunning && !programStepDependenciesSatisfiedWithCache(latest, step, budgets, nodeStateCache) {
			return plan.StepSpec{}, false
		}
		return step, true
	case plan.StepFailed:
		switch normalizedOnFailStrategy(step) {
		case "replan", "abort":
			return plan.StepSpec{}, false
		case "continue":
			if step.Attempt < allowedAttempts(step, budgets) {
				return step, true
			}
			return plan.StepSpec{}, false
		default:
			return step, true
		}
	default:
		return plan.StepSpec{}, false
	}
}

func firstPendingPlanStep(latest plan.Spec, budgets LoopBudgets) (plan.StepSpec, bool) {
	nodeStateCache := map[string]map[string]programNodeRoundState{}
	for _, step := range latest.Steps {
		if step.Status != "" && step.Status != plan.StepPending {
			continue
		}
		if programStepDependenciesSatisfiedWithCache(latest, step, budgets, nodeStateCache) {
			return step, true
		}
		if sibling, ok := firstReadyProgramSiblingStep(latest, step, budgets, nodeStateCache); ok {
			return sibling, true
		}
		return plan.StepSpec{}, false
	}
	return plan.StepSpec{}, false
}

func firstReadyProgramSiblingStep(latest plan.Spec, blocked plan.StepSpec, budgets LoopBudgets, nodeStateCache map[string]map[string]programNodeRoundState) (plan.StepSpec, bool) {
	groupID, ok := programGroupIDFromStep(blocked)
	if !ok {
		return plan.StepSpec{}, false
	}
	for _, candidate := range latest.Steps {
		if candidate.StepID == blocked.StepID {
			continue
		}
		if !programStepInGroup(candidate, groupID) {
			continue
		}
		if candidate.Status != "" && candidate.Status != plan.StepPending {
			continue
		}
		if !programStepDependenciesSatisfiedWithCache(latest, candidate, budgets, nodeStateCache) {
			continue
		}
		return candidate, true
	}
	return plan.StepSpec{}, false
}

func firstFailedPlanStep(latest plan.Spec, budgets LoopBudgets) (plan.StepSpec, bool) {
	for _, step := range latest.Steps {
		if step.Status == plan.StepFailed {
			if normalizedOnFailStrategy(step) == "continue" && step.Attempt >= allowedAttempts(step, budgets) {
				continue
			}
			return step, true
		}
	}
	return plan.StepSpec{}, false
}

func findPlanStepByID(latest plan.Spec, stepID string) (plan.StepSpec, bool) {
	for _, step := range latest.Steps {
		if step.StepID == stepID {
			return step, true
		}
	}
	return plan.StepSpec{}, false
}

func mergeSessionRunOutputs(base, next SessionRunOutput) SessionRunOutput {
	out := base
	out.Executions = append(out.Executions, next.Executions...)
	if next.Plan != nil {
		out.Plan = next.Plan
	}
	if len(next.Aggregates) > 0 || next.Plan != nil {
		out.Aggregates = next.Aggregates
	}
	out.Session = next.Session
	return out
}

func planPointer(pl plan.Spec) *plan.Spec {
	copyPlan := pl
	return &copyPlan
}

func populateSessionRunAggregates(out SessionRunOutput) SessionRunOutput {
	if out.Plan == nil {
		out.Aggregates = nil
		return out
	}
	out.Aggregates = execution.AggregateResultsFromPlan(*out.Plan)
	return out
}

func isTerminalPhase(phase session.Phase) bool {
	switch phase {
	case session.PhaseComplete, session.PhaseFailed, session.PhaseAborted:
		return true
	default:
		return false
	}
}
