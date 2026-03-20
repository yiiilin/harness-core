package runtime

import (
	"context"

	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

const (
	runSessionPlanReason = "runtime/session-driver"
	replanSessionReason  = "runtime/replan"
)

type SessionRunOutput struct {
	Session    session.State   `json:"session"`
	Plan       *plan.Spec      `json:"plan,omitempty"`
	Executions []StepRunOutput `json:"executions,omitempty"`
}

type sessionStepSelection struct {
	Step          plan.StepSpec
	HasStep       bool
	NeedsPlanning bool
}

func (s *Service) runSession(ctx context.Context, sessionID string) (SessionRunOutput, error) {
	out := SessionRunOutput{}
	for {
		state, err := s.GetSession(sessionID)
		if err != nil {
			return SessionRunOutput{}, err
		}
		out.Session = state

		latest, ok, err := s.latestPlanForSession(sessionID)
		if err != nil {
			return SessionRunOutput{}, err
		}
		if ok {
			out.Plan = planPointer(latest)
		}

		if isTerminalPhase(state.Phase) || state.PendingApprovalID != "" {
			return out, nil
		}

		if !ok {
			planned, err := s.createDriverPlan(ctx, sessionID, runSessionPlanReason)
			if err != nil {
				return SessionRunOutput{}, err
			}
			out.Plan = planPointer(planned)
			latest = planned
		}

		selection := selectNextStepForSession(state, latest)
		if selection.NeedsPlanning {
			planned, err := s.createDriverPlan(ctx, sessionID, replanSessionReason)
			if err != nil {
				return SessionRunOutput{}, err
			}
			out.Plan = planPointer(planned)
			continue
		}
		if !selection.HasStep {
			return out, nil
		}

		stepOut, err := s.runStep(ctx, sessionID, selection.Step)
		if err != nil {
			return SessionRunOutput{}, err
		}
		out.Executions = append(out.Executions, stepOut)
		out.Session = stepOut.Session
		if stepOut.UpdatedPlan != nil {
			out.Plan = stepOut.UpdatedPlan
		}
		if _, _, err := s.CompactSessionContext(ctx, sessionID, CompactionTriggerExecute); err != nil {
			return SessionRunOutput{}, err
		}
		if isTerminalPhase(stepOut.Session.Phase) || stepOut.Session.PendingApprovalID != "" {
			return out, nil
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

func selectNextStepForSession(state session.State, latest plan.Spec) sessionStepSelection {
	if step, ok := pinnedStepForSession(state, latest); ok {
		return sessionStepSelection{Step: step, HasStep: true}
	}
	if step, ok := firstPendingPlanStep(latest); ok {
		return sessionStepSelection{Step: step, HasStep: true}
	}
	if step, ok := firstFailedPlanStep(latest); ok {
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

func pinnedStepForSession(state session.State, latest plan.Spec) (plan.StepSpec, bool) {
	if state.InFlightStepID != "" {
		if step, ok := executableStepByID(latest, state.InFlightStepID); ok {
			return step, true
		}
	}
	if state.CurrentStepID != "" {
		if step, ok := executableStepByID(latest, state.CurrentStepID); ok {
			return step, true
		}
	}
	return plan.StepSpec{}, false
}

func executableStepByID(latest plan.Spec, stepID string) (plan.StepSpec, bool) {
	if stepID == "" {
		return plan.StepSpec{}, false
	}
	step, ok := findPlanStepByID(latest, stepID)
	if !ok {
		return plan.StepSpec{}, false
	}
	switch step.Status {
	case "", plan.StepPending, plan.StepRunning:
		return step, true
	case plan.StepFailed:
		switch normalizedOnFailStrategy(step) {
		case "replan", "abort":
			return plan.StepSpec{}, false
		default:
			return step, true
		}
	default:
		return plan.StepSpec{}, false
	}
}

func firstPendingPlanStep(latest plan.Spec) (plan.StepSpec, bool) {
	for _, step := range latest.Steps {
		if step.Status == "" || step.Status == plan.StepPending {
			return step, true
		}
	}
	return plan.StepSpec{}, false
}

func firstFailedPlanStep(latest plan.Spec) (plan.StepSpec, bool) {
	for _, step := range latest.Steps {
		if step.Status == plan.StepFailed {
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
	out.Session = next.Session
	return out
}

func planPointer(pl plan.Spec) *plan.Spec {
	copyPlan := pl
	return &copyPlan
}

func isTerminalPhase(phase session.Phase) bool {
	switch phase {
	case session.PhaseComplete, session.PhaseFailed, session.PhaseAborted:
		return true
	default:
		return false
	}
}
