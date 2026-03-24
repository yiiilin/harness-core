package runtime

import (
	"context"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

func (s *Service) aggregateVerifyInputForStep(ctx context.Context, sessionID string, step plan.StepSpec, rawSuccess bool) (*verify.Spec, action.Result, bool, error) {
	if programVerifyScopeFromStep(step) != execution.VerificationScopeAggregate {
		return nil, action.Result{}, false, nil
	}
	spec, ok := programAggregateVerifySpecFromStep(step)
	if !ok || spec == nil {
		return nil, action.Result{}, false, nil
	}
	aggregateID, _, ok := execution.AggregateRefFromMetadata(step.Metadata)
	if !ok || aggregateID == "" {
		return nil, action.Result{}, false, nil
	}
	latest, ok, err := s.latestPlanForSession(ctx, sessionID)
	if err != nil {
		return nil, action.Result{}, false, err
	}
	if !ok {
		return spec, action.Result{}, false, nil
	}
	provisional := step
	if rawSuccess {
		provisional.Status = plan.StepCompleted
	} else {
		provisional.Status = plan.StepFailed
	}
	simulated := replacePlanStep(latest, provisional)
	if !aggregateVerificationReady(simulated, aggregateID, s.LoopBudgets) {
		return spec, action.Result{}, false, nil
	}
	for _, aggregate := range execution.AggregateResultsFromPlan(simulated) {
		if aggregate.AggregateID == aggregateID {
			return spec, actionResultFromAggregate(aggregate), true, nil
		}
	}
	return spec, action.Result{}, false, nil
}

func aggregateVerificationReady(spec plan.Spec, aggregateID string, budgets LoopBudgets) bool {
	for _, step := range spec.Steps {
		id, _, ok := execution.AggregateRefFromMetadata(step.Metadata)
		if !ok || id != aggregateID {
			continue
		}
		switch step.Status {
		case "", plan.StepPending, plan.StepRunning, plan.StepBlocked:
			return false
		case plan.StepFailed:
			if step.Attempt < allowedAttempts(step, budgets) {
				return false
			}
		}
	}
	return true
}

func actionResultFromAggregate(aggregate execution.AggregateResult) action.Result {
	targets := make([]map[string]any, 0, len(aggregate.Targets))
	for _, target := range aggregate.Targets {
		item := map[string]any{
			"step_id": target.StepID,
			"status":  target.Status,
			"attempt": target.Attempt,
			"reason":  target.Reason,
		}
		if target.Target.TargetID != "" {
			item["target_id"] = target.Target.TargetID
		}
		if target.Target.Kind != "" {
			item["target_kind"] = target.Target.Kind
		}
		targets = append(targets, item)
	}
	return action.Result{
		OK: aggregate.Status != execution.AggregateStatusFailed,
		Data: map[string]any{
			"aggregate_id": aggregate.AggregateID,
			"scope":        aggregate.Scope,
			"strategy":     aggregate.Strategy,
			"status":       aggregate.Status,
			"program_id":   aggregate.ProgramID,
			"node_id":      aggregate.NodeID,
			"title":        aggregate.Title,
			"expected":     aggregate.Expected,
			"completed":    aggregate.Completed,
			"failed":       aggregate.Failed,
			"pending":      aggregate.Pending,
			"targets":      targets,
		},
		Meta: map[string]any{
			"verification_scope": execution.VerificationScopeAggregate,
		},
	}
}
