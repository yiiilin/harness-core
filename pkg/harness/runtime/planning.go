package runtime

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/yiiilin/harness-core/pkg/harness/persistence"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/planning"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

func (s *Service) AssembleContextForSession(ctx context.Context, sessionID string) (ContextPackage, *ContextSummary, session.State, task.Spec, error) {
	state, err := s.GetSession(sessionID)
	if err != nil {
		return ContextPackage{}, nil, session.State{}, task.Spec{}, err
	}
	if state.TaskID == "" {
		return ContextPackage{}, nil, session.State{}, task.Spec{}, errors.New("session has no task attached")
	}
	rec, err := s.GetTask(state.TaskID)
	if err != nil {
		return ContextPackage{}, nil, session.State{}, task.Spec{}, err
	}
	spec := task.Spec{
		TaskID:      rec.TaskID,
		TaskType:    rec.TaskType,
		Goal:        rec.Goal,
		Constraints: rec.Constraints,
		Metadata:    rec.Metadata,
	}
	assembled, summary, err := s.CompactSessionContext(ctx, sessionID, CompactionTriggerPlan)
	if err != nil {
		return ContextPackage{}, nil, session.State{}, task.Spec{}, err
	}
	return assembled, summary, state, spec, nil
}

func (s *Service) latestPlanForSession(sessionID string) (plan.Spec, bool, error) {
	if s.Plans == nil {
		return plan.Spec{}, false, nil
	}
	return s.Plans.LatestBySession(sessionID)
}

func (s *Service) CreatePlanFromPlanner(ctx context.Context, sessionID, changeReason string, maxSteps int) (plan.Spec, ContextPackage, error) {
	requestedMaxSteps := maxSteps
	if maxSteps <= 0 {
		maxSteps = s.LoopBudgets.MaxSteps
	}
	if s.LoopBudgets.MaxSteps > 0 && maxSteps > s.LoopBudgets.MaxSteps {
		maxSteps = s.LoopBudgets.MaxSteps
	}

	planningID := "pln_" + uuid.NewString()
	startedAt := s.nowMilli()
	metadata := map[string]any{
		"requested_max_steps": requestedMaxSteps,
		"effective_max_steps": maxSteps,
	}

	assembled, summary, planningState, spec, err := s.AssembleContextForSession(ctx, sessionID)
	if err != nil {
		err = s.joinPlanningPersistenceError(ctx, err, planning.Record{
			PlanningID: planningID,
			SessionID:  sessionID,
			Status:     planning.StatusFailed,
			Reason:     changeReason,
			Error:      err.Error(),
			Metadata:   metadata,
			StartedAt:  startedAt,
			FinishedAt: s.nowMilli(),
		})
		return plan.Spec{}, ContextPackage{}, err
	}
	contextSummaryID := ""
	if summary != nil {
		contextSummaryID = summary.SummaryID
	}
	view, err := s.freezeCapabilityView(ctx, planningState.SessionID, spec.TaskID)
	if err != nil {
		err = s.joinPlanningPersistenceError(ctx, err, planning.Record{
			PlanningID:       planningID,
			SessionID:        planningState.SessionID,
			TaskID:           spec.TaskID,
			Status:           planning.StatusFailed,
			Reason:           changeReason,
			Error:            err.Error(),
			ContextSummaryID: contextSummaryID,
			Metadata:         metadata,
			StartedAt:        startedAt,
			FinishedAt:       s.nowMilli(),
		})
		return plan.Spec{}, ContextPackage{}, err
	}
	assembled = attachCapabilityViewToContext(assembled, view)

	steps := make([]plan.StepSpec, 0, maxSteps)
	lastAssembled := assembled
	for i := 0; i < maxSteps; i++ {
		step, err := s.Planner.PlanNext(ctx, planningState, spec, lastAssembled)
		if err != nil {
			if len(steps) == 0 {
				metadata["step_count"] = len(steps)
				err = s.joinPlanningPersistenceError(ctx, err, planning.Record{
					PlanningID:       planningID,
					SessionID:        planningState.SessionID,
					TaskID:           spec.TaskID,
					Status:           planning.StatusFailed,
					Reason:           changeReason,
					Error:            err.Error(),
					CapabilityViewID: view.ViewID,
					ContextSummaryID: contextSummaryID,
					Metadata:         metadata,
					StartedAt:        startedAt,
					FinishedAt:       s.nowMilli(),
				})
				return plan.Spec{}, ContextPackage{}, err
			}
			break
		}
		step, err = pinStepToCapabilityView(step, view)
		if err != nil {
			return plan.Spec{}, ContextPackage{}, err
		}
		steps = append(steps, step)
		planningState.CurrentStepID = step.StepID
		planningState.Phase = session.PhasePlan
		lastAssembled, summary, err = s.compactAssembledContext(ctx, planningState, spec, CompactionTriggerPlan)
		if err != nil {
			metadata["step_count"] = len(steps)
			err = s.joinPlanningPersistenceError(ctx, err, planning.Record{
				PlanningID:       planningID,
				SessionID:        planningState.SessionID,
				TaskID:           spec.TaskID,
				Status:           planning.StatusFailed,
				Reason:           changeReason,
				Error:            err.Error(),
				CapabilityViewID: view.ViewID,
				ContextSummaryID: contextSummaryID,
				Metadata:         metadata,
				StartedAt:        startedAt,
				FinishedAt:       s.nowMilli(),
			})
			return plan.Spec{}, ContextPackage{}, err
		}
		if summary != nil {
			contextSummaryID = summary.SummaryID
		}
		lastAssembled = attachCapabilityViewToContext(lastAssembled, view)
	}

	if len(steps) == 0 {
		err := errors.New("planner did not produce any steps")
		metadata["step_count"] = len(steps)
		err = s.joinPlanningPersistenceError(ctx, err, planning.Record{
			PlanningID:       planningID,
			SessionID:        planningState.SessionID,
			TaskID:           spec.TaskID,
			Status:           planning.StatusFailed,
			Reason:           changeReason,
			Error:            err.Error(),
			CapabilityViewID: view.ViewID,
			ContextSummaryID: contextSummaryID,
			Metadata:         metadata,
			StartedAt:        startedAt,
			FinishedAt:       s.nowMilli(),
		})
		return plan.Spec{}, ContextPackage{}, err
	}

	metadata["step_count"] = len(steps)
	pl, err := s.createPlanWithCapabilityView(ctx, sessionID, changeReason, steps, view, planning.Record{
		PlanningID:       planningID,
		SessionID:        planningState.SessionID,
		TaskID:           spec.TaskID,
		Status:           planning.StatusCompleted,
		Reason:           changeReason,
		CapabilityViewID: view.ViewID,
		ContextSummaryID: contextSummaryID,
		Metadata:         metadata,
		StartedAt:        startedAt,
		FinishedAt:       s.nowMilli(),
	})
	if err != nil {
		err = s.joinPlanningPersistenceError(ctx, err, planning.Record{
			PlanningID:       planningID,
			SessionID:        planningState.SessionID,
			TaskID:           spec.TaskID,
			Status:           planning.StatusFailed,
			Reason:           changeReason,
			Error:            err.Error(),
			CapabilityViewID: view.ViewID,
			ContextSummaryID: contextSummaryID,
			Metadata:         metadata,
			StartedAt:        startedAt,
			FinishedAt:       s.nowMilli(),
		})
		return plan.Spec{}, ContextPackage{}, err
	}
	s.exportPlanningObservability(ctx, planning.Record{
		PlanningID:       planningID,
		SessionID:        planningState.SessionID,
		TaskID:           spec.TaskID,
		Status:           planning.StatusCompleted,
		Reason:           changeReason,
		PlanID:           pl.PlanID,
		PlanRevision:     pl.Revision,
		CapabilityViewID: view.ViewID,
		ContextSummaryID: contextSummaryID,
		Metadata:         metadata,
		StartedAt:        startedAt,
		FinishedAt:       s.nowMilli(),
	})
	return pl, assembled, nil
}

func (s *Service) joinPlanningPersistenceError(ctx context.Context, cause error, record planning.Record) error {
	s.exportPlanningObservability(ctx, record)
	if err := s.persistPlanningRecord(ctx, record); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

func (s *Service) persistPlanningRecord(ctx context.Context, record planning.Record) error {
	if s.PlanningRecords == nil {
		return nil
	}
	create := func(store planning.Store) error {
		if store == nil {
			return nil
		}
		_, err := store.Create(record)
		return err
	}
	if s.Runner != nil {
		return s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
			return create(s.repositoriesWithFallback(repos).PlanningRecords)
		})
	}
	return create(s.PlanningRecords)
}
