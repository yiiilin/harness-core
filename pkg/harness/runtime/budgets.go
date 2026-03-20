package runtime

import (
	"time"

	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

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

func ensureRuntimeBudget(state session.State, budgets LoopBudgets) error {
	if budgets.MaxTotalRuntimeMS <= 0 || state.CreatedAt == 0 {
		return nil
	}
	if time.Now().UnixMilli()-state.CreatedAt > budgets.MaxTotalRuntimeMS {
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
	default:
		if step.Attempt < allowedAttempts(step, budgets) {
			return TransitionDecision{From: state.Phase, To: TransitionRecover, StepID: step.StepID, Reason: "verification failed, retry allowed"}
		}
		return TransitionDecision{From: state.Phase, To: TransitionFailed, StepID: step.StepID, Reason: "verification failed and retry budget exhausted"}
	}
}
