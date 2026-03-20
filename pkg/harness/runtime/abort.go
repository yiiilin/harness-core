package runtime

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/persistence"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

type AbortRequest struct {
	Code     string         `json:"code,omitempty"`
	Reason   string         `json:"reason,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type AbortOutput struct {
	Session     session.State `json:"session"`
	UpdatedTask *task.Record  `json:"updated_task,omitempty"`
	Events      []audit.Event `json:"events,omitempty"`
}

func (s *Service) AbortSession(ctx context.Context, sessionID string, request AbortRequest) (AbortOutput, error) {
	current, err := s.GetSession(sessionID)
	if err != nil {
		return AbortOutput{}, err
	}
	if current.Phase == session.PhaseAborted {
		return AbortOutput{Session: current}, nil
	}
	if isTerminalPhase(current.Phase) {
		return AbortOutput{}, ErrSessionTerminal
	}

	reason := request.Reason
	if reason == "" {
		reason = "abort requested"
	}
	aborted := ApplyTransition(current, TransitionDecision{
		From:   current.Phase,
		To:     TransitionAborted,
		StepID: current.CurrentStepID,
		Reason: reason,
	})
	aborted.ExecutionState = session.ExecutionIdle
	aborted.InFlightStepID = ""
	aborted.PendingApprovalID = ""
	aborted.Version++

	payload := map[string]any{
		"code":   request.Code,
		"reason": reason,
	}
	if len(request.Metadata) > 0 {
		payload["metadata"] = cloneAnyMap(request.Metadata)
	}
	now := time.Now().UnixMilli()
	events := []audit.Event{
		{
			EventID:   "evt_" + uuid.NewString(),
			Type:      audit.EventStateChanged,
			SessionID: current.SessionID,
			TaskID:    current.TaskID,
			StepID:    current.CurrentStepID,
			Payload:   map[string]any{"from": current.Phase, "to": TransitionAborted, "reason": reason},
			CreatedAt: now,
		},
		{
			EventID:   "evt_" + uuid.NewString(),
			Type:      audit.EventSessionAborted,
			SessionID: current.SessionID,
			TaskID:    current.TaskID,
			StepID:    current.CurrentStepID,
			Payload:   payload,
			CreatedAt: now,
		},
	}

	var updatedTask *task.Record
	persist := func(sessStore session.Store, taskStore task.Store, handleStore execution.RuntimeHandleStore) error {
		taskRec, err := updateTaskForTerminalInStore(taskStore, aborted)
		if err != nil {
			return err
		}
		updatedTask = taskRec
		if updatedTask != nil {
			events = append(events, audit.Event{
				EventID:   "evt_" + uuid.NewString(),
				Type:      audit.EventTaskAborted,
				SessionID: aborted.SessionID,
				TaskID:    updatedTask.TaskID,
				StepID:    aborted.CurrentStepID,
				Payload: map[string]any{
					"task_id": updatedTask.TaskID,
					"status":  updatedTask.Status,
				},
				CreatedAt: time.Now().UnixMilli(),
			})
		}
		if err := reconcileActiveRuntimeHandlesInStore(handleStore, current.SessionID, "session aborted"); err != nil {
			return err
		}
		if err := sessStore.Update(aborted); err != nil {
			return err
		}
		return nil
	}

	if s.Runner != nil {
		if err := s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
			repoSet := s.repositoriesWithFallback(repos)
			if err := persist(repoSet.Sessions, repoSet.Tasks, repoSet.RuntimeHandles); err != nil {
				return err
			}
			return s.emitEventsWithSink(ctx, s.eventSinkForRepos(repos), events)
		}); err != nil {
			return AbortOutput{}, err
		}
		return AbortOutput{Session: aborted, UpdatedTask: updatedTask, Events: events}, nil
	}

	if err := persist(s.Sessions, s.Tasks, s.RuntimeHandles); err != nil {
		return AbortOutput{}, err
	}
	_ = s.emitEventsWithSink(ctx, s.EventSink, events)
	return AbortOutput{Session: aborted, UpdatedTask: updatedTask, Events: events}, nil
}
