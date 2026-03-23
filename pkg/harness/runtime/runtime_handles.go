package runtime

import (
	"context"

	"github.com/google/uuid"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/persistence"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

type RuntimeHandleUpdate struct {
	Kind     *string        `json:"kind,omitempty"`
	Value    *string        `json:"value,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type RuntimeHandleCloseRequest struct {
	Reason   string         `json:"reason,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type RuntimeHandleInvalidateRequest struct {
	Reason   string         `json:"reason,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

func (s *Service) UpdateRuntimeHandle(ctx context.Context, handleID string, update RuntimeHandleUpdate) (execution.RuntimeHandle, error) {
	return s.mutateRuntimeHandle(ctx, handleID, "", audit.EventRuntimeHandleUpdated, func(handle execution.RuntimeHandle) execution.RuntimeHandle {
		if update.Kind != nil {
			handle.Kind = *update.Kind
		}
		if update.Value != nil {
			handle.Value = *update.Value
		}
		if len(update.Metadata) > 0 {
			handle.Metadata = mergeMaps(handle.Metadata, cloneAnyMap(update.Metadata))
		}
		if handle.Status == "" {
			handle.Status = execution.RuntimeHandleActive
		}
		return handle
	})
}

func (s *Service) UpdateClaimedRuntimeHandle(ctx context.Context, handleID, leaseID string, update RuntimeHandleUpdate) (execution.RuntimeHandle, error) {
	if leaseID == "" {
		return execution.RuntimeHandle{}, session.ErrSessionLeaseNotHeld
	}
	return s.mutateRuntimeHandle(ctx, handleID, leaseID, audit.EventRuntimeHandleUpdated, func(handle execution.RuntimeHandle) execution.RuntimeHandle {
		if update.Kind != nil {
			handle.Kind = *update.Kind
		}
		if update.Value != nil {
			handle.Value = *update.Value
		}
		if len(update.Metadata) > 0 {
			handle.Metadata = mergeMaps(handle.Metadata, cloneAnyMap(update.Metadata))
		}
		if handle.Status == "" {
			handle.Status = execution.RuntimeHandleActive
		}
		return handle
	})
}

func (s *Service) CloseRuntimeHandle(ctx context.Context, handleID string, request RuntimeHandleCloseRequest) (execution.RuntimeHandle, error) {
	return s.mutateRuntimeHandle(ctx, handleID, "", audit.EventRuntimeHandleClosed, func(handle execution.RuntimeHandle) execution.RuntimeHandle {
		handle.Status = execution.RuntimeHandleClosed
		handle.StatusReason = runtimeHandleReasonOrDefault(request.Reason, "runtime handle closed")
		handle.ClosedAt = s.nowMilli()
		if len(request.Metadata) > 0 {
			handle.Metadata = mergeMaps(handle.Metadata, cloneAnyMap(request.Metadata))
		}
		return handle
	})
}

func (s *Service) CloseClaimedRuntimeHandle(ctx context.Context, handleID, leaseID string, request RuntimeHandleCloseRequest) (execution.RuntimeHandle, error) {
	if leaseID == "" {
		return execution.RuntimeHandle{}, session.ErrSessionLeaseNotHeld
	}
	return s.mutateRuntimeHandle(ctx, handleID, leaseID, audit.EventRuntimeHandleClosed, func(handle execution.RuntimeHandle) execution.RuntimeHandle {
		handle.Status = execution.RuntimeHandleClosed
		handle.StatusReason = runtimeHandleReasonOrDefault(request.Reason, "runtime handle closed")
		handle.ClosedAt = s.nowMilli()
		if len(request.Metadata) > 0 {
			handle.Metadata = mergeMaps(handle.Metadata, cloneAnyMap(request.Metadata))
		}
		return handle
	})
}

func (s *Service) InvalidateRuntimeHandle(ctx context.Context, handleID string, request RuntimeHandleInvalidateRequest) (execution.RuntimeHandle, error) {
	return s.mutateRuntimeHandle(ctx, handleID, "", audit.EventRuntimeHandleInvalidated, func(handle execution.RuntimeHandle) execution.RuntimeHandle {
		handle.Status = execution.RuntimeHandleInvalidated
		handle.StatusReason = runtimeHandleReasonOrDefault(request.Reason, "runtime handle invalidated")
		handle.InvalidatedAt = s.nowMilli()
		if len(request.Metadata) > 0 {
			handle.Metadata = mergeMaps(handle.Metadata, cloneAnyMap(request.Metadata))
		}
		return handle
	})
}

func (s *Service) InvalidateClaimedRuntimeHandle(ctx context.Context, handleID, leaseID string, request RuntimeHandleInvalidateRequest) (execution.RuntimeHandle, error) {
	if leaseID == "" {
		return execution.RuntimeHandle{}, session.ErrSessionLeaseNotHeld
	}
	return s.mutateRuntimeHandle(ctx, handleID, leaseID, audit.EventRuntimeHandleInvalidated, func(handle execution.RuntimeHandle) execution.RuntimeHandle {
		handle.Status = execution.RuntimeHandleInvalidated
		handle.StatusReason = runtimeHandleReasonOrDefault(request.Reason, "runtime handle invalidated")
		handle.InvalidatedAt = s.nowMilli()
		if len(request.Metadata) > 0 {
			handle.Metadata = mergeMaps(handle.Metadata, cloneAnyMap(request.Metadata))
		}
		return handle
	})
}

func (s *Service) mutateRuntimeHandle(ctx context.Context, handleID string, leaseID string, eventType string, mutate func(execution.RuntimeHandle) execution.RuntimeHandle) (execution.RuntimeHandle, error) {
	var updated execution.RuntimeHandle
	apply := func(store execution.RuntimeHandleStore, sessions session.Store, sink EventSink) error {
		if store == nil {
			return execution.ErrRecordNotFound
		}
		current, err := store.Get(handleID)
		if err != nil {
			return err
		}
		if current.SessionID != "" && sessions != nil {
			st, err := sessions.Get(current.SessionID)
			if err != nil {
				return err
			}
			if err := requireSessionLease(st, leaseID, s.nowMilli()); err != nil {
				return err
			}
		}
		if !isRuntimeHandleActive(current) {
			return ErrRuntimeHandleNotActive
		}
		updated = mutate(current)
		updated.HandleID = current.HandleID
		if updated.SessionID == "" {
			updated.SessionID = current.SessionID
		}
		if updated.TaskID == "" {
			updated.TaskID = current.TaskID
		}
		if updated.AttemptID == "" {
			updated.AttemptID = current.AttemptID
		}
		if updated.CycleID == "" {
			updated.CycleID = current.CycleID
		}
		if updated.TraceID == "" {
			updated.TraceID = current.TraceID
		}
		if updated.Status == "" {
			updated.Status = execution.RuntimeHandleActive
		}
		if updated.CreatedAt == 0 {
			updated.CreatedAt = current.CreatedAt
		}
		updated.Version = current.Version + 1
		updated.UpdatedAt = s.nowMilli()
		if err := store.Update(updated); err != nil {
			return err
		}
		events := runtimeHandleAuditEvents(s.nowMilli(), eventType, []execution.RuntimeHandle{updated})
		if sink != nil {
			return s.emitEventsWithSink(ctx, sink, events)
		}
		_ = s.emitEvents(ctx, events)
		return nil
	}

	if s.Runner != nil {
		if err := s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
			repoSet := s.repositoriesWithFallback(repos)
			return apply(repoSet.RuntimeHandles, repoSet.Sessions, s.eventSinkForRepos(repos))
		}); err != nil {
			return execution.RuntimeHandle{}, err
		}
		return updated, nil
	}

	if err := apply(s.RuntimeHandles, s.Sessions, nil); err != nil {
		return execution.RuntimeHandle{}, err
	}
	return updated, nil
}

func reconcileActiveRuntimeHandlesInStore(store execution.RuntimeHandleStore, sessionID, reason string, now int64) ([]execution.RuntimeHandle, error) {
	if store == nil || sessionID == "" {
		return nil, nil
	}
	handles, err := store.List(sessionID)
	if err != nil {
		return nil, err
	}
	updated := make([]execution.RuntimeHandle, 0)
	for _, handle := range handles {
		if !isRuntimeHandleActive(handle) {
			continue
		}
		handle.Status = execution.RuntimeHandleInvalidated
		handle.StatusReason = reason
		handle.InvalidatedAt = now
		handle.UpdatedAt = now
		handle.Version++
		if err := store.Update(handle); err != nil {
			return nil, err
		}
		updated = append(updated, handle)
	}
	return updated, nil
}

func isRuntimeHandleActive(handle execution.RuntimeHandle) bool {
	return handle.Status == "" || handle.Status == execution.RuntimeHandleActive
}

func runtimeHandleReasonOrDefault(reason, fallback string) string {
	if reason != "" {
		return reason
	}
	return fallback
}

func runtimeHandleAuditEvents(now int64, eventType string, handles []execution.RuntimeHandle) []audit.Event {
	events := make([]audit.Event, 0, len(handles))
	for _, handle := range handles {
		events = append(events, audit.Event{
			EventID:     "evt_" + uuid.NewString(),
			Type:        eventType,
			SessionID:   handle.SessionID,
			TaskID:      handle.TaskID,
			AttemptID:   handle.AttemptID,
			CycleID:     handle.CycleID,
			TraceID:     handle.TraceID,
			CausationID: handle.HandleID,
			Payload: map[string]any{
				"handle_id":      handle.HandleID,
				"kind":           handle.Kind,
				"value":          handle.Value,
				"status":         handle.Status,
				"status_reason":  handle.StatusReason,
				"closed_at":      handle.ClosedAt,
				"invalidated_at": handle.InvalidatedAt,
				"version":        handle.Version,
			},
			CreatedAt: now,
		})
	}
	return events
}
