package runtime

import (
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

// DecideNextTransition applies the minimal deterministic part of the runtime loop.
// It does not replace planning or verification logic; it only encodes the common
// state-machine transitions the kernel can own safely.
func DecideNextTransition(state session.State, stepID string, policy permission.Decision, verified bool) TransitionDecision {
	if policy.Action == permission.Deny {
		return TransitionDecision{
			From:   state.Phase,
			To:     TransitionFailed,
			StepID: stepID,
			Reason: "policy denied action",
		}
	}

	switch state.Phase {
	case session.PhaseReceived:
		return TransitionDecision{From: state.Phase, To: TransitionPrepare, StepID: stepID, Reason: "task accepted"}
	case session.PhasePrepare:
		return TransitionDecision{From: state.Phase, To: TransitionPlan, StepID: stepID, Reason: "preparation complete"}
	case session.PhasePlan:
		return TransitionDecision{From: state.Phase, To: TransitionExecute, StepID: stepID, Reason: "step selected"}
	case session.PhaseExecute:
		return TransitionDecision{From: state.Phase, To: TransitionVerify, StepID: stepID, Reason: "action finished"}
	case session.PhaseVerify:
		if verified {
			return TransitionDecision{From: state.Phase, To: TransitionComplete, StepID: stepID, Reason: "verification succeeded"}
		}
		return TransitionDecision{From: state.Phase, To: TransitionRecover, StepID: stepID, Reason: "verification failed"}
	case session.PhaseRecover:
		return TransitionDecision{From: state.Phase, To: TransitionPlan, StepID: stepID, Reason: "recovery requires replanning"}
	case session.PhaseComplete:
		return TransitionDecision{From: state.Phase, To: TransitionStay, StepID: stepID, Reason: "already complete"}
	case session.PhaseFailed:
		return TransitionDecision{From: state.Phase, To: TransitionStay, StepID: stepID, Reason: "already failed"}
	case session.PhaseAborted:
		return TransitionDecision{From: state.Phase, To: TransitionStay, StepID: stepID, Reason: "already aborted"}
	default:
		return TransitionDecision{From: state.Phase, To: TransitionFailed, StepID: stepID, Reason: "unknown phase"}
	}
}

func ApplyTransition(state session.State, next TransitionDecision) session.State {
	updated := state
	switch next.To {
	case TransitionPrepare:
		updated.Phase = session.PhasePrepare
	case TransitionPlan:
		updated.Phase = session.PhasePlan
	case TransitionExecute:
		updated.Phase = session.PhaseExecute
	case TransitionVerify:
		updated.Phase = session.PhaseVerify
	case TransitionRecover:
		updated.Phase = session.PhaseRecover
	case TransitionComplete:
		updated.Phase = session.PhaseComplete
	case TransitionFailed:
		updated.Phase = session.PhaseFailed
	case TransitionAborted:
		updated.Phase = session.PhaseAborted
	case TransitionStay:
		// no-op
	}
	if next.StepID != "" {
		updated.CurrentStepID = next.StepID
	}
	return updated
}
