package runtime

import (
	"context"
	"errors"

	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

func (s *Service) AssembleContextForSession(ctx context.Context, sessionID string) (ContextPackage, session.State, task.Spec, error) {
	state, err := s.GetSession(sessionID)
	if err != nil {
		return ContextPackage{}, session.State{}, task.Spec{}, err
	}
	if state.TaskID == "" {
		return ContextPackage{}, session.State{}, task.Spec{}, errors.New("session has no task attached")
	}
	rec, err := s.GetTask(state.TaskID)
	if err != nil {
		return ContextPackage{}, session.State{}, task.Spec{}, err
	}
	spec := task.Spec{
		TaskID:      rec.TaskID,
		TaskType:    rec.TaskType,
		Goal:        rec.Goal,
		Constraints: rec.Constraints,
		Metadata:    rec.Metadata,
	}
	assembled, err := s.assembleAndCompactContext(ctx, state, spec)
	if err != nil {
		return ContextPackage{}, session.State{}, task.Spec{}, err
	}
	return assembled, state, spec, nil
}

func (s *Service) CreatePlanFromPlanner(ctx context.Context, sessionID, changeReason string, maxSteps int) (plan.Spec, ContextPackage, error) {
	if maxSteps <= 0 {
		maxSteps = s.LoopBudgets.MaxSteps
	}
	if s.LoopBudgets.MaxSteps > 0 && maxSteps > s.LoopBudgets.MaxSteps {
		maxSteps = s.LoopBudgets.MaxSteps
	}

	assembled, planningState, spec, err := s.AssembleContextForSession(ctx, sessionID)
	if err != nil {
		return plan.Spec{}, ContextPackage{}, err
	}

	steps := make([]plan.StepSpec, 0, maxSteps)
	lastAssembled := assembled
	for i := 0; i < maxSteps; i++ {
		step, err := s.Planner.PlanNext(ctx, planningState, spec, lastAssembled)
		if err != nil {
			if len(steps) == 0 {
				return plan.Spec{}, ContextPackage{}, err
			}
			break
		}
		steps = append(steps, step)
		planningState.CurrentStepID = step.StepID
		planningState.Phase = session.PhasePlan
		lastAssembled, err = s.assembleAndCompactContext(ctx, planningState, spec)
		if err != nil {
			return plan.Spec{}, ContextPackage{}, err
		}
	}

	if len(steps) == 0 {
		return plan.Spec{}, ContextPackage{}, errors.New("planner did not produce any steps")
	}

	pl, err := s.CreatePlan(sessionID, changeReason, steps)
	if err != nil {
		return plan.Spec{}, ContextPackage{}, err
	}
	return pl, assembled, nil
}

func (s *Service) assembleAndCompactContext(ctx context.Context, state session.State, spec task.Spec) (ContextPackage, error) {
	assembled, err := s.ContextAssembler.Assemble(ctx, state, spec)
	if err != nil {
		return ContextPackage{}, err
	}
	if s.Compactor == nil {
		return assembled, nil
	}
	compacted, summary, err := s.Compactor.Compact(ctx, assembled, state, spec, s.LoopBudgets)
	if err != nil {
		return ContextPackage{}, err
	}
	assembled = compacted
	if summary != nil && s.ContextSummaries != nil {
		if summary.SessionID == "" {
			summary.SessionID = state.SessionID
		}
		if summary.TaskID == "" {
			summary.TaskID = spec.TaskID
		}
		persisted, err := s.ContextSummaries.Create(*summary)
		if err != nil {
			return ContextPackage{}, err
		}
		assembled.Compaction = &ContextCompaction{
			SummaryID:      persisted.SummaryID,
			Strategy:       persisted.Strategy,
			OriginalBytes:  persisted.OriginalBytes,
			CompactedBytes: persisted.CompactedBytes,
			Metadata:       persisted.Metadata,
		}
	}
	return assembled, nil
}
