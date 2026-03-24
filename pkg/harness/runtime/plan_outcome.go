package runtime

import (
	"context"

	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

type planExecutionOutcome struct {
	Aggregates []execution.AggregateResult
	Continue   bool
	Fail       bool
	Reason     string
}

func planExecutionOutcomeForSpec(spec plan.Spec, budgets LoopBudgets) planExecutionOutcome {
	aggregates := execution.AggregateResultsFromPlan(spec)
	aggregateByID := make(map[string]execution.AggregateResult, len(aggregates))
	for _, aggregate := range aggregates {
		aggregateByID[aggregate.AggregateID] = aggregate
	}

	hasPending := false
	hasRecoverableFailure := false
	for _, step := range spec.Steps {
		switch step.Status {
		case "", plan.StepPending, plan.StepRunning, plan.StepBlocked:
			hasPending = true
		case plan.StepFailed:
			switch normalizedOnFailStrategy(step) {
			case "abort":
				return planExecutionOutcome{
					Aggregates: aggregates,
					Fail:       true,
					Reason:     "plan contains aborting failed step",
				}
			case "replan", "retry", "reinspect":
				hasRecoverableFailure = true
			case "continue":
				if step.Attempt < allowedAttempts(step, budgets) {
					hasRecoverableFailure = true
					continue
				}
				aggregateID, _, ok := execution.AggregateRefFromMetadata(step.Metadata)
				if !ok {
					continue
				}
				aggregate := aggregateByID[aggregateID]
				switch aggregate.Status {
				case execution.AggregateStatusFailed:
					if !aggregateExhaustedAsFailed(spec, aggregateID, budgets) {
						hasRecoverableFailure = true
						continue
					}
					return planExecutionOutcome{
						Aggregates: aggregates,
						Fail:       true,
						Reason:     "fan-out aggregate failed",
					}
				case execution.AggregateStatusPending:
					hasPending = true
				}
			default:
				hasRecoverableFailure = true
			}
		}
	}

	if hasPending {
		return planExecutionOutcome{
			Aggregates: aggregates,
			Continue:   true,
			Reason:     "plan has remaining steps",
		}
	}
	if hasRecoverableFailure {
		return planExecutionOutcome{
			Aggregates: aggregates,
			Continue:   true,
			Reason:     "plan contains recoverable failed steps",
		}
	}
	for _, aggregate := range aggregates {
		if aggregate.Status == execution.AggregateStatusFailed && aggregateExhaustedAsFailed(spec, aggregate.AggregateID, budgets) {
			return planExecutionOutcome{
				Aggregates: aggregates,
				Fail:       true,
				Reason:     "fan-out aggregate failed",
			}
		}
	}
	return planExecutionOutcome{Aggregates: aggregates}
}

func aggregateExhaustedAsFailed(spec plan.Spec, aggregateID string, budgets LoopBudgets) bool {
	found := false
	for _, step := range spec.Steps {
		id, _, ok := execution.AggregateRefFromMetadata(step.Metadata)
		if !ok || id != aggregateID {
			continue
		}
		found = true
		switch step.Status {
		case "", plan.StepPending, plan.StepRunning, plan.StepBlocked, plan.StepCompleted:
			return false
		case plan.StepFailed:
			if step.Attempt < allowedAttempts(step, budgets) {
				return false
			}
		}
	}
	return found
}

func planStatusForSpec(spec plan.Spec, budgets LoopBudgets) plan.Status {
	outcome := planExecutionOutcomeForSpec(spec, budgets)
	switch {
	case outcome.Fail:
		return plan.StatusFailed
	case outcome.Continue:
		return plan.StatusActive
	default:
		return plan.StatusCompleted
	}
}

func replacePlanStep(spec plan.Spec, step plan.StepSpec) plan.Spec {
	out := spec
	for i := range out.Steps {
		if out.Steps[i].StepID == step.StepID {
			out.Steps[i] = step
			return out
		}
	}
	return out
}

func transitionForPlanOutcome(state session.State, step plan.StepSpec, outcome planExecutionOutcome) TransitionDecision {
	switch {
	case outcome.Fail:
		return TransitionDecision{From: state.Phase, To: TransitionFailed, StepID: step.StepID, Reason: outcome.Reason}
	case outcome.Continue:
		return TransitionDecision{From: state.Phase, To: TransitionPlan, StepID: step.StepID, Reason: outcome.Reason}
	default:
		return TransitionDecision{From: state.Phase, To: TransitionComplete, StepID: step.StepID, Reason: "plan completed"}
	}
}

func (s *Service) reconcileTransitionWithPlan(ctx context.Context, sessionID string, state session.State, step plan.StepSpec, verified bool, next TransitionDecision) (TransitionDecision, error) {
	shouldReconcile := verified
	if !shouldReconcile && normalizedOnFailStrategy(step) == "continue" && step.Attempt >= allowedAttempts(step, s.LoopBudgets) {
		shouldReconcile = true
	}
	if !shouldReconcile {
		return next, nil
	}

	latest, ok, err := s.latestPlanForSession(ctx, sessionID)
	if err != nil {
		return TransitionDecision{}, err
	}
	if !ok {
		return next, nil
	}
	outcome := planExecutionOutcomeForSpec(replacePlanStep(latest, step), s.LoopBudgets)
	return transitionForPlanOutcome(state, step, outcome), nil
}
