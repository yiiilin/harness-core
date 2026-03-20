package runtime

import (
	"context"
	"time"

	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/persistence"
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
	return s.mutateRuntimeHandle(ctx, handleID, func(handle execution.RuntimeHandle) execution.RuntimeHandle {
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
	return s.mutateRuntimeHandle(ctx, handleID, func(handle execution.RuntimeHandle) execution.RuntimeHandle {
		handle.Status = execution.RuntimeHandleClosed
		handle.StatusReason = runtimeHandleReasonOrDefault(request.Reason, "runtime handle closed")
		handle.ClosedAt = time.Now().UnixMilli()
		if len(request.Metadata) > 0 {
			handle.Metadata = mergeMaps(handle.Metadata, cloneAnyMap(request.Metadata))
		}
		return handle
	})
}

func (s *Service) InvalidateRuntimeHandle(ctx context.Context, handleID string, request RuntimeHandleInvalidateRequest) (execution.RuntimeHandle, error) {
	return s.mutateRuntimeHandle(ctx, handleID, func(handle execution.RuntimeHandle) execution.RuntimeHandle {
		handle.Status = execution.RuntimeHandleInvalidated
		handle.StatusReason = runtimeHandleReasonOrDefault(request.Reason, "runtime handle invalidated")
		handle.InvalidatedAt = time.Now().UnixMilli()
		if len(request.Metadata) > 0 {
			handle.Metadata = mergeMaps(handle.Metadata, cloneAnyMap(request.Metadata))
		}
		return handle
	})
}

func (s *Service) mutateRuntimeHandle(ctx context.Context, handleID string, mutate func(execution.RuntimeHandle) execution.RuntimeHandle) (execution.RuntimeHandle, error) {
	var updated execution.RuntimeHandle
	apply := func(store execution.RuntimeHandleStore) error {
		if store == nil {
			return execution.ErrRecordNotFound
		}
		current, err := store.Get(handleID)
		if err != nil {
			return err
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
		if updated.TraceID == "" {
			updated.TraceID = current.TraceID
		}
		if updated.Status == "" {
			updated.Status = execution.RuntimeHandleActive
		}
		if updated.CreatedAt == 0 {
			updated.CreatedAt = current.CreatedAt
		}
		updated.UpdatedAt = time.Now().UnixMilli()
		return store.Update(updated)
	}

	if s.Runner != nil {
		if err := s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
			return apply(s.repositoriesWithFallback(repos).RuntimeHandles)
		}); err != nil {
			return execution.RuntimeHandle{}, err
		}
		return updated, nil
	}

	if err := apply(s.RuntimeHandles); err != nil {
		return execution.RuntimeHandle{}, err
	}
	return updated, nil
}

func reconcileActiveRuntimeHandlesInStore(store execution.RuntimeHandleStore, sessionID, reason string) error {
	if store == nil || sessionID == "" {
		return nil
	}
	handles, err := store.List(sessionID)
	if err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	for _, handle := range handles {
		if !isRuntimeHandleActive(handle) {
			continue
		}
		handle.Status = execution.RuntimeHandleInvalidated
		handle.StatusReason = reason
		handle.InvalidatedAt = now
		handle.UpdatedAt = now
		if err := store.Update(handle); err != nil {
			return err
		}
	}
	return nil
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
