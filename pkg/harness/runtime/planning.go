package runtime

import (
	"context"
	"errors"

	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

func (s *Service) AssembleContextForSession(ctx context.Context, sessionID string) (map[string]any, session.State, task.Spec, error) {
	state, err := s.GetSession(sessionID)
	if err != nil {
		return nil, session.State{}, task.Spec{}, err
	}
	if state.TaskID == "" {
		return nil, session.State{}, task.Spec{}, errors.New("session has no task attached")
	}
	rec, err := s.GetTask(state.TaskID)
	if err != nil {
		return nil, session.State{}, task.Spec{}, err
	}
	spec := task.Spec{
		TaskID:      rec.TaskID,
		TaskType:    rec.TaskType,
		Goal:        rec.Goal,
		Constraints: rec.Constraints,
		Metadata:    rec.Metadata,
	}
	assembled, err := s.ContextAssembler.Assemble(ctx, state, spec)
	if err != nil {
		return nil, session.State{}, task.Spec{}, err
	}
	return assembled, state, spec, nil
}

func (s *Service) CreatePlanFromPlanner(ctx context.Context, sessionID, changeReason string, maxSteps int) (plan.Spec, map[string]any, error) {
	if maxSteps <= 0 {
		maxSteps = 1
	}

	assembled, planningState, spec, err := s.AssembleContextForSession(ctx, sessionID)
	if err != nil {
		return plan.Spec{}, nil, err
	}

	steps := make([]plan.StepSpec, 0, maxSteps)
	lastAssembled := assembled
	for i := 0; i < maxSteps; i++ {
		step, err := s.Planner.PlanNext(ctx, planningState, spec, lastAssembled)
		if err != nil {
			if len(steps) == 0 {
				return plan.Spec{}, nil, err
			}
			break
		}
		steps = append(steps, step)
		planningState.CurrentStepID = step.StepID
		planningState.Phase = session.PhasePlan
		lastAssembled, err = s.ContextAssembler.Assemble(ctx, planningState, spec)
		if err != nil {
			return plan.Spec{}, nil, err
		}
	}

	if len(steps) == 0 {
		return plan.Spec{}, nil, errors.New("planner did not produce any steps")
	}

	pl, err := s.CreatePlan(sessionID, changeReason, steps)
	if err != nil {
		return plan.Spec{}, nil, err
	}
	return pl, assembled, nil
}
