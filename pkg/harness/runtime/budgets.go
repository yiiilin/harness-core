package runtime

import (
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

const stepRetryNotBeforeKey = "retry_not_before"

func ensurePlanRevisionBudgetInStore(store plan.Store, sessionID string, budgets LoopBudgets) error {
	if store == nil || budgets.MaxPlanRevisions <= 0 {
		return nil
	}
	latest, ok, err := store.LatestBySession(sessionID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if latest.Revision >= budgets.MaxPlanRevisions {
		return ErrPlanRevisionBudgetExceeded
	}
	return nil
}

func ensureRuntimeBudget(state session.State, budgets LoopBudgets, now int64) error {
	if budgets.MaxTotalRuntimeMS <= 0 || state.CreatedAt == 0 {
		return nil
	}
	if now-state.CreatedAt > budgets.MaxTotalRuntimeMS {
		return ErrRuntimeBudgetExceeded
	}
	return nil
}

func ensureStepRetryBudget(step plan.StepSpec, budgets LoopBudgets) error {
	if step.Attempt >= allowedAttempts(step, budgets) {
		return ErrStepRetryBudgetExceeded
	}
	return nil
}

func allowedAttempts(step plan.StepSpec, budgets LoopBudgets) int {
	retries := budgets.MaxRetriesPerStep
	if retries < 0 {
		retries = 0
	}
	if step.OnFail.MaxRetries > 0 && (retries == 0 || step.OnFail.MaxRetries < retries) {
		retries = step.OnFail.MaxRetries
	}
	return retries + 1
}

func normalizedOnFailStrategy(step plan.StepSpec) string {
	switch step.OnFail.Strategy {
	case "", "retry":
		return "retry"
	case "abort":
		return "abort"
	case "replan":
		return "replan"
	case "reinspect":
		return "reinspect"
	default:
		return "retry"
	}
}

func nextTransitionAfterVerification(state session.State, step plan.StepSpec, decision permission.Decision, verified bool, budgets LoopBudgets) TransitionDecision {
	if verified {
		return DecideNextTransition(state, step.StepID, decision, true)
	}

	switch normalizedOnFailStrategy(step) {
	case "abort":
		return TransitionDecision{From: state.Phase, To: TransitionFailed, StepID: step.StepID, Reason: "verification failed and on_fail=abort"}
	case "replan":
		return TransitionDecision{From: state.Phase, To: TransitionPlan, StepID: step.StepID, Reason: "verification failed and on_fail=replan"}
	case "reinspect":
		if step.Attempt < allowedAttempts(step, budgets) {
			return TransitionDecision{From: state.Phase, To: TransitionPrepare, StepID: step.StepID, Reason: "verification failed, reinspect allowed"}
		}
		return TransitionDecision{From: state.Phase, To: TransitionFailed, StepID: step.StepID, Reason: "verification failed and retry budget exhausted"}
	default:
		if step.Attempt < allowedAttempts(step, budgets) {
			return TransitionDecision{From: state.Phase, To: TransitionRecover, StepID: step.StepID, Reason: "verification failed, retry allowed"}
		}
		return TransitionDecision{From: state.Phase, To: TransitionFailed, StepID: step.StepID, Reason: "verification failed and retry budget exhausted"}
	}
}

func stepRetryNotBefore(step plan.StepSpec) (int64, bool) {
	if len(step.Metadata) == 0 {
		return 0, false
	}
	value, ok := step.Metadata[stepRetryNotBeforeKey]
	if !ok {
		return 0, false
	}
	switch typed := value.(type) {
	case int64:
		return typed, true
	case int:
		return int64(typed), true
	case float64:
		return int64(typed), true
	default:
		return 0, false
	}
}

func backoffActive(step plan.StepSpec, now int64) bool {
	retryNotBefore, ok := stepRetryNotBefore(step)
	return ok && retryNotBefore > now
}

func applyStepRetryBackoff(step *plan.StepSpec, next TransitionDecision, now int64) {
	if step == nil {
		return
	}
	switch next.To {
	case TransitionRecover, TransitionPrepare:
		if step.OnFail.BackoffMS <= 0 {
			clearStepRetryBackoff(step)
			return
		}
		if step.Metadata == nil {
			step.Metadata = map[string]any{}
		}
		step.Metadata[stepRetryNotBeforeKey] = now + int64(step.OnFail.BackoffMS)
	default:
		clearStepRetryBackoff(step)
	}
}

func clearStepRetryBackoff(step *plan.StepSpec) {
	if step == nil || len(step.Metadata) == 0 {
		return
	}
	delete(step.Metadata, stepRetryNotBeforeKey)
	if len(step.Metadata) == 0 {
		step.Metadata = nil
	}
}
