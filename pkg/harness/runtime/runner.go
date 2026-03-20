package runtime

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/persistence"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

type StepRunOutput struct {
	Session     session.State        `json:"session"`
	Execution   ExecutionResult      `json:"execution"`
	Transitions []TransitionDecision `json:"transitions"`
	Events      []audit.Event        `json:"events"`
	UpdatedPlan *plan.Spec           `json:"updated_plan,omitempty"`
	UpdatedTask *task.Record         `json:"updated_task,omitempty"`
}

func (s *Service) runStep(ctx context.Context, sessionID string, step plan.StepSpec) (StepRunOutput, error) {
	state, err := s.GetSession(sessionID)
	if err != nil {
		return StepRunOutput{}, err
	}
	if state.Phase == session.PhaseComplete || state.Phase == session.PhaseFailed || state.Phase == session.PhaseAborted {
		return StepRunOutput{}, ErrSessionTerminal
	}

	now := time.Now().UnixMilli()
	step.Attempt++
	if step.Status == "" || step.Status == plan.StepPending {
		step.StartedAt = now
	}
	step.Status = plan.StepRunning

	transitions := []TransitionDecision{}
	events := []audit.Event{}
	appendEvent := func(eventType string, stepID string, payload map[string]any) {
		events = append(events, audit.Event{
			EventID:   "evt_" + uuid.NewString(),
			Type:      eventType,
			SessionID: sessionID,
			StepID:    stepID,
			Payload:   payload,
			CreatedAt: time.Now().UnixMilli(),
		})
	}

	for state.Phase != session.PhaseExecute && state.Phase != session.PhaseComplete && state.Phase != session.PhaseFailed && state.Phase != session.PhaseAborted {
		next := DecideNextTransition(state, step.StepID, permission.Decision{Action: permission.Allow, Reason: "state advancement"}, false)
		transitions = append(transitions, next)
		appendEvent(audit.EventStateChanged, step.StepID, map[string]any{"from": state.Phase, "to": next.To, "reason": next.Reason})
		state = ApplyTransition(state, next)
	}

	decision, err := s.EvaluatePolicy(ctx, state, step)
	if err != nil {
		return StepRunOutput{}, err
	}
	execResult := ExecutionResult{
		Step:   step,
		Policy: PolicyDecision{Decision: decision},
	}
	appendEvent(audit.EventStepStarted, step.StepID, map[string]any{"title": step.Title})

	if _, err := s.MarkSessionInFlight(ctx, sessionID, step.StepID); err != nil {
		return StepRunOutput{}, err
	}
	state, _ = s.GetSession(sessionID)

	if decision.Action == permission.Deny {
		s.Metrics.Record("step.run", map[string]any{"success": false, "policy_denied": true, "verify_failed": false, "action_failed": false, "duration_ms": int64(0)})
		step.Status = plan.StepFailed
		step.FinishedAt = time.Now().UnixMilli()
		next := TransitionDecision{From: state.Phase, To: TransitionFailed, StepID: step.StepID, Reason: "policy denied action"}
		transitions = append(transitions, next)
		appendEvent(audit.EventPolicyDenied, step.StepID, map[string]any{"reason": decision.Reason, "matched_rule": decision.MatchedRule})
		appendEvent(audit.EventStateChanged, step.StepID, map[string]any{"from": state.Phase, "to": next.To, "reason": next.Reason})
		state = ApplyTransition(state, next)
		state.ExecutionState = session.ExecutionIdle
		state.InFlightStepID = ""
		var updatedPlan *plan.Spec
		var updatedTask *task.Record
		if s.Runner != nil {
			if err := s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
				pl, err := updateLatestPlanStepInStore(repos.Plans, sessionID, step)
				if err != nil {
					return err
				}
				updatedPlan = pl
				taskRec, err := updateTaskForTerminalInStore(repos.Tasks, state)
				if err != nil {
					return err
				}
				updatedTask = taskRec
				if err := repos.Sessions.Update(state); err != nil {
					return err
				}
				for _, event := range events {
					if repos.Audits != nil {
						if err := repos.Audits.Emit(event); err != nil {
							return err
						}
					}
				}
				return nil
			}); err != nil {
				return StepRunOutput{}, err
			}
		} else {
			updatedPlan, _ = updateLatestPlanStepInStore(s.Plans, sessionID, step)
			updatedTask, _ = updateTaskForTerminalInStore(s.Tasks, state)
			if err := s.Sessions.Update(state); err != nil {
				return StepRunOutput{}, err
			}
		}
		if s.Runner == nil {
			s.emitEvents(ctx, events)
		}
		return StepRunOutput{Session: state, Execution: execResult, Transitions: transitions, Events: events, UpdatedPlan: updatedPlan, UpdatedTask: updatedTask}, nil
	}

	appendEvent(audit.EventToolCalled, step.StepID, map[string]any{"tool_name": step.Action.ToolName})
	actResult, actErr := s.InvokeAction(ctx, step.Action)
	execResult.Action = actResult
	if actErr != nil {
		appendEvent(audit.EventToolFailed, step.StepID, map[string]any{"tool_name": step.Action.ToolName, "error": actErr.Error()})
	} else if actResult.OK {
		appendEvent(audit.EventToolCompleted, step.StepID, map[string]any{"tool_name": step.Action.ToolName})
	} else {
		appendEvent(audit.EventToolFailed, step.StepID, map[string]any{"tool_name": step.Action.ToolName, "error": actionErrorMessage(actResult)})
	}

	state.Phase = session.PhaseVerify
	verifyResult, verifyErr := s.EvaluateVerify(ctx, step.Verify, actResult, state)
	execResult.Verify = verifyResult
	appendEvent(audit.EventVerifyCompleted, step.StepID, map[string]any{"success": verifyResult.Success, "reason": verifyResult.Reason})
	verified := verifyErr == nil && verifyResult.Success

	next := DecideNextTransition(state, step.StepID, decision, verified)
	if verified && latestPlanHasRemainingSteps(s.Plans, sessionID, step.StepID) {
		next = TransitionDecision{From: state.Phase, To: TransitionPlan, StepID: step.StepID, Reason: "step completed, continue plan"}
	}
	transitions = append(transitions, next)
	appendEvent(audit.EventStateChanged, step.StepID, map[string]any{"from": state.Phase, "to": next.To, "reason": next.Reason})
	state = ApplyTransition(state, next)
	state.ExecutionState = session.ExecutionIdle
	state.InFlightStepID = ""

	if verified {
		step.Status = plan.StepCompleted
	} else {
		step.Status = plan.StepFailed
		state.RetryCount++
	}
	step.FinishedAt = time.Now().UnixMilli()
	execResult.Step = step

	var updatedPlan *plan.Spec
	var updatedTask *task.Record
	if s.Runner != nil {
		if err := s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
			pl, err := updateLatestPlanStepInStore(repos.Plans, sessionID, step)
			if err != nil {
				return err
			}
			updatedPlan = pl
			taskRec, err := updateTaskForTerminalInStore(repos.Tasks, state)
			if err != nil {
				return err
			}
			updatedTask = taskRec
			if err := repos.Sessions.Update(state); err != nil {
				return err
			}
			for _, event := range events {
				if repos.Audits != nil {
					if err := repos.Audits.Emit(event); err != nil {
						return err
					}
				}
			}
			return nil
		}); err != nil {
			return StepRunOutput{}, err
		}
	} else {
		updatedPlan, _ = updateLatestPlanStepInStore(s.Plans, sessionID, step)
		updatedTask, _ = updateTaskForTerminalInStore(s.Tasks, state)
		if err := s.Sessions.Update(state); err != nil {
			return StepRunOutput{}, err
		}
	}
	if s.Runner == nil {
		s.emitEvents(ctx, events)
	}
	s.Metrics.Record("step.run", map[string]any{
		"success":       verified,
		"policy_denied": false,
		"verify_failed": !verified,
		"action_failed": !actResult.OK,
		"duration_ms":   time.Now().UnixMilli() - now,
	})

	return StepRunOutput{
		Session:     state,
		Execution:   execResult,
		Transitions: transitions,
		Events:      events,
		UpdatedPlan: updatedPlan,
		UpdatedTask: updatedTask,
	}, nil
}

func updateLatestPlanStepInStore(store plan.Store, sessionID string, step plan.StepSpec) (*plan.Spec, error) {
	latest, ok := store.LatestBySession(sessionID)
	if !ok {
		return nil, nil
	}
	changed := false
	for i := range latest.Steps {
		if latest.Steps[i].StepID == step.StepID {
			latest.Steps[i] = step
			changed = true
			break
		}
	}
	if !changed {
		return &latest, nil
	}
	if step.Status == plan.StepCompleted {
		allDone := true
		for _, st := range latest.Steps {
			if st.Status != plan.StepCompleted {
				allDone = false
				break
			}
		}
		if allDone {
			latest.Status = plan.StatusCompleted
		}
	}
	if step.Status == plan.StepFailed {
		latest.Status = plan.StatusActive
	}
	if err := store.Update(latest); err != nil {
		return nil, err
	}
	return &latest, nil
}

func updateTaskForTerminalInStore(store task.Store, state session.State) (*task.Record, error) {
	if state.TaskID == "" {
		return nil, nil
	}
	rec, err := store.Get(state.TaskID)
	if err != nil {
		return nil, err
	}
	switch state.Phase {
	case session.PhaseComplete:
		rec.Status = task.StatusCompleted
	case session.PhaseFailed:
		rec.Status = task.StatusFailed
	case session.PhaseAborted:
		rec.Status = task.StatusAborted
	default:
		return &rec, nil
	}
	if err := store.Update(rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

func latestPlanHasRemainingSteps(store plan.Store, sessionID, completedStepID string) bool {
	if store == nil {
		return false
	}
	latest, ok := store.LatestBySession(sessionID)
	if !ok {
		return false
	}
	for _, st := range latest.Steps {
		status := st.Status
		if st.StepID == completedStepID {
			status = plan.StepCompleted
		}
		if status != plan.StepCompleted {
			return true
		}
	}
	return false
}

func actionErrorMessage(result action.Result) string {
	if result.Error != nil && result.Error.Message != "" {
		return result.Error.Message
	}
	return "tool failed"
}

func (s *Service) emitEvents(ctx context.Context, events []audit.Event) {
	if s.Audit == nil {
		return
	}
	for _, event := range events {
		_ = s.Audit.Emit(event)
	}
	_ = ctx
}
