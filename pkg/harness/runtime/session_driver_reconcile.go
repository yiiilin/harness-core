package runtime

import (
	"context"

	"github.com/google/uuid"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/persistence"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

func (s *Service) reconcileNoSelectionPlanFailure(ctx context.Context, sessionID, leaseID string, state session.State, latest plan.Spec, outcome planExecutionOutcome) (SessionRunOutput, bool, error) {
	if !outcome.Fail || isTerminalPhase(state.Phase) {
		return SessionRunOutput{}, false, nil
	}

	stepID := attributedPlanFailureStepID(state, latest, s.LoopBudgets)

	next := setSessionPlanRef(state, plan.StepSpec{PlanID: latest.PlanID, PlanRevision: latest.Revision})
	transition := TransitionDecision{
		From:   next.Phase,
		To:     TransitionFailed,
		StepID: stepID,
		Reason: outcome.Reason,
	}
	next = ApplyTransition(next, transition)

	event := audit.Event{
		EventID:   "evt_" + uuid.NewString(),
		Type:      audit.EventStateChanged,
		SessionID: sessionID,
		TaskID:    state.TaskID,
		StepID:    stepID,
		Payload: map[string]any{
			"from":   transition.From,
			"to":     transition.To,
			"reason": transition.Reason,
		},
		CreatedAt: s.nowMilli(),
	}

	updatedPlan := latest
	updatedPlan.Status = plan.StatusFailed
	annotatedPlan := annotatePlanIdentity(updatedPlan)
	var updatedTask any

	persist := func(repos persistence.RepositorySet) error {
		if repos.Sessions == nil {
			return session.ErrSessionNotFound
		}
		if repos.Plans != nil {
			if err := repos.Plans.Update(updatedPlan); err != nil {
				return err
			}
		}
		persisted, err := persistSessionUpdate(repos.Sessions, next, leaseID)
		if err != nil {
			return err
		}
		next = persisted
		taskRec, err := updateTaskForTerminalInStore(repos.Tasks, next)
		if err != nil {
			return err
		}
		updatedTask = taskRec
		return nil
	}

	if s.Runner != nil {
		if err := s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
			repoSet := s.repositoriesWithFallback(repos)
			if err := persist(repoSet); err != nil {
				return err
			}
			return s.emitEventsWithSink(ctx, s.eventSinkForRepos(repos), []audit.Event{event})
		}); err != nil {
			return SessionRunOutput{}, false, err
		}
	} else {
		if err := persist(s.repositoriesWithFallback(persistence.RepositorySet{})); err != nil {
			return SessionRunOutput{}, false, err
		}
		s.emitEventsBestEffort(ctx, []audit.Event{event})
	}

	_ = updatedTask
	return SessionRunOutput{
		Session:    next,
		Plan:       &annotatedPlan,
		Aggregates: outcome.Aggregates,
	}, true, nil
}

func attributedPlanFailureStepID(state session.State, latest plan.Spec, budgets LoopBudgets) string {
	if stepID, ok := currentAttributablePlanStepID(state, latest, budgets); ok {
		return stepID
	}
	if failedStep, ok := firstFailedPlanStep(latest, budgets); ok {
		return failedStep.StepID
	}
	if step, ok := firstUnfinishedPlanStep(latest); ok {
		return step.StepID
	}
	return ""
}

func currentAttributablePlanStepID(state session.State, latest plan.Spec, budgets LoopBudgets) (string, bool) {
	if state.CurrentStepID == "" {
		return "", false
	}
	step, ok := findPlanStepByID(latest, state.CurrentStepID)
	if !ok {
		return "", false
	}
	switch step.Status {
	case "", plan.StepPending, plan.StepRunning, plan.StepBlocked:
		return step.StepID, true
	case plan.StepFailed:
		if normalizedOnFailStrategy(step) == "continue" && step.Attempt >= allowedAttempts(step, budgets) {
			return "", false
		}
		return step.StepID, true
	default:
		return "", false
	}
}

func firstUnfinishedPlanStep(latest plan.Spec) (plan.StepSpec, bool) {
	for _, step := range latest.Steps {
		switch step.Status {
		case "", plan.StepPending, plan.StepRunning, plan.StepBlocked:
			return step, true
		}
	}
	return plan.StepSpec{}, false
}
