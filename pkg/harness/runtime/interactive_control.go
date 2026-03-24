package runtime

import (
	"context"

	"github.com/google/uuid"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/persistence"
)

type InteractiveStartRequest struct {
	HandleID  string         `json:"handle_id,omitempty"`
	SessionID string         `json:"session_id,omitempty"`
	TaskID    string         `json:"task_id,omitempty"`
	AttemptID string         `json:"attempt_id,omitempty"`
	CycleID   string         `json:"cycle_id,omitempty"`
	TraceID   string         `json:"trace_id,omitempty"`
	Kind      string         `json:"kind,omitempty"`
	Spec      map[string]any `json:"spec,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type InteractiveStartResult struct {
	Kind         string                            `json:"kind,omitempty"`
	Value        string                            `json:"value,omitempty"`
	Capabilities execution.InteractiveCapabilities `json:"capabilities,omitempty"`
	Observation  execution.InteractiveObservation  `json:"observation,omitempty"`
	Metadata     map[string]any                    `json:"metadata,omitempty"`
}

type InteractiveReopenRequest struct {
	Metadata map[string]any `json:"metadata,omitempty"`
}

type InteractiveReopenResult struct {
	Capabilities *execution.InteractiveCapabilities `json:"capabilities,omitempty"`
	Observation  *execution.InteractiveObservation  `json:"observation,omitempty"`
	Metadata     map[string]any                     `json:"metadata,omitempty"`
}

type InteractiveViewRequest struct {
	Offset   int64          `json:"offset,omitempty"`
	MaxBytes int            `json:"max_bytes,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type InteractiveViewResult struct {
	Runtime   execution.InteractiveRuntime `json:"runtime"`
	Data      string                       `json:"data,omitempty"`
	Truncated bool                         `json:"truncated,omitempty"`
	Metadata  map[string]any               `json:"metadata,omitempty"`
}

type InteractiveWriteRequest struct {
	Input    string         `json:"input,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type InteractiveWriteResult struct {
	Runtime  execution.InteractiveRuntime `json:"runtime"`
	Bytes    int64                        `json:"bytes,omitempty"`
	Metadata map[string]any               `json:"metadata,omitempty"`
}

type InteractiveCloseRequest struct {
	Reason   string         `json:"reason,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type InteractiveCloseResult struct {
	Runtime  execution.InteractiveRuntime `json:"runtime"`
	Metadata map[string]any               `json:"metadata,omitempty"`
}

func (s *Service) StartInteractive(ctx context.Context, sessionID string, request InteractiveStartRequest) (execution.InteractiveRuntime, error) {
	if s.InteractiveController == nil {
		return execution.InteractiveRuntime{}, ErrInteractiveControllerNotConfigured
	}
	state, err := s.getSessionRecord(ctx, sessionID)
	if err != nil {
		return execution.InteractiveRuntime{}, err
	}
	if isTerminalPhase(state.Phase) {
		return execution.InteractiveRuntime{}, ErrSessionTerminal
	}

	prepared := request
	prepared.SessionID = sessionID
	prepared.TaskID = firstNonEmptyString(prepared.TaskID, state.TaskID)
	prepared.HandleID = firstNonEmptyString(prepared.HandleID, "hdl_"+uuid.NewString())
	prepared.Metadata = cloneAnyMap(prepared.Metadata)
	prepared.Spec = cloneAnyMap(prepared.Spec)

	started, err := s.InteractiveController.StartInteractive(ctx, prepared)
	if err != nil {
		return execution.InteractiveRuntime{}, err
	}

	now := s.nowMilli()
	observation := normalizeInteractiveObservation(started.Observation, execution.RuntimeHandleActive, "interactive runtime active", now)
	handle := execution.RuntimeHandle{
		HandleID:     prepared.HandleID,
		SessionID:    prepared.SessionID,
		TaskID:       prepared.TaskID,
		AttemptID:    prepared.AttemptID,
		CycleID:      prepared.CycleID,
		TraceID:      prepared.TraceID,
		Kind:         firstNonEmptyString(started.Kind, prepared.Kind),
		Value:        started.Value,
		Status:       execution.RuntimeHandleActive,
		StatusReason: firstNonEmptyString(observation.StatusReason, "interactive runtime active"),
		Metadata: execution.ApplyInteractiveRuntimeMetadata(
			mergeMaps(cloneAnyMap(prepared.Metadata), cloneAnyMap(started.Metadata)),
			interactiveCapabilitiesPtr(started.Capabilities),
			&observation,
			nil,
		),
		CreatedAt: now,
		UpdatedAt: now,
	}
	created, err := s.createRuntimeHandleWithAudit(ctx, handle)
	if err != nil {
		_, _ = s.InteractiveController.CloseInteractive(ctx, handle, InteractiveCloseRequest{Reason: "interactive runtime persistence failed"})
		return execution.InteractiveRuntime{}, err
	}
	return interactiveRuntimeFromHandleOrErr(created)
}

func (s *Service) ReopenInteractive(ctx context.Context, handleID string, request InteractiveReopenRequest) (execution.InteractiveRuntime, error) {
	if s.InteractiveController == nil {
		return execution.InteractiveRuntime{}, ErrInteractiveControllerNotConfigured
	}
	current, existing, err := s.requireActiveInteractiveHandle(ctx, handleID)
	if err != nil {
		return execution.InteractiveRuntime{}, err
	}
	result, err := s.InteractiveController.ReopenInteractive(ctx, current, request)
	if err != nil {
		return execution.InteractiveRuntime{}, err
	}
	return s.persistInteractiveRuntimeState(ctx, handleID, existing,
		result.Capabilities,
		result.Observation,
		interactiveOperation(execution.InteractiveOperationReopen, s.nowMilli(), 0, 0, request.Metadata),
		cloneAnyMap(result.Metadata),
	)
}

func (s *Service) ViewInteractive(ctx context.Context, handleID string, request InteractiveViewRequest) (InteractiveViewResult, error) {
	if s.InteractiveController == nil {
		return InteractiveViewResult{}, ErrInteractiveControllerNotConfigured
	}
	current, existing, err := s.requireActiveInteractiveHandle(ctx, handleID)
	if err != nil {
		return InteractiveViewResult{}, err
	}
	viewed, err := s.InteractiveController.ViewInteractive(ctx, current, request)
	if err != nil {
		return InteractiveViewResult{}, err
	}
	updated, err := s.persistInteractiveRuntimeState(ctx, handleID, existing,
		mergeInteractiveCapabilities(existing.Capabilities, viewed.Runtime.Capabilities),
		mergeInteractiveObservation(existing.Observation, viewed.Runtime.Observation),
		interactiveOperation(execution.InteractiveOperationView, s.nowMilli(), request.Offset, 0, request.Metadata),
		mergeMaps(cloneAnyMap(viewed.Metadata), cloneAnyMap(viewed.Runtime.Metadata)),
	)
	if err != nil {
		return InteractiveViewResult{}, err
	}
	viewed.Runtime = updated
	viewed.Metadata = mergeMaps(cloneAnyMap(viewed.Metadata), cloneAnyMap(updated.Metadata))
	return viewed, nil
}

func (s *Service) WriteInteractive(ctx context.Context, handleID string, request InteractiveWriteRequest) (InteractiveWriteResult, error) {
	if s.InteractiveController == nil {
		return InteractiveWriteResult{}, ErrInteractiveControllerNotConfigured
	}
	current, existing, err := s.requireActiveInteractiveHandle(ctx, handleID)
	if err != nil {
		return InteractiveWriteResult{}, err
	}
	written, err := s.InteractiveController.WriteInteractive(ctx, current, request)
	if err != nil {
		return InteractiveWriteResult{}, err
	}
	updated, err := s.persistInteractiveRuntimeState(ctx, handleID, existing,
		mergeInteractiveCapabilities(existing.Capabilities, written.Runtime.Capabilities),
		mergeInteractiveObservation(existing.Observation, written.Runtime.Observation),
		interactiveOperation(execution.InteractiveOperationWrite, s.nowMilli(), 0, written.Bytes, request.Metadata),
		mergeMaps(cloneAnyMap(written.Metadata), cloneAnyMap(written.Runtime.Metadata)),
	)
	if err != nil {
		return InteractiveWriteResult{}, err
	}
	written.Runtime = updated
	written.Metadata = mergeMaps(cloneAnyMap(written.Metadata), cloneAnyMap(updated.Metadata))
	return written, nil
}

func (s *Service) CloseInteractive(ctx context.Context, handleID string, request InteractiveCloseRequest) (execution.InteractiveRuntime, error) {
	if s.InteractiveController == nil {
		return execution.InteractiveRuntime{}, ErrInteractiveControllerNotConfigured
	}
	current, existing, err := s.requireActiveInteractiveHandle(ctx, handleID)
	if err != nil {
		return execution.InteractiveRuntime{}, err
	}
	closed, err := s.InteractiveController.CloseInteractive(ctx, current, request)
	if err != nil {
		return execution.InteractiveRuntime{}, err
	}
	observation := mergeInteractiveObservation(existing.Observation, closed.Runtime.Observation)
	if observation == nil {
		observation = &execution.InteractiveObservation{}
	}
	observation.Closed = true
	observation.Status = firstNonEmptyString(observation.Status, "closed")
	observation.StatusReason = firstNonEmptyString(request.Reason, observation.StatusReason, "interactive runtime closed")
	updateMetadata := execution.ApplyInteractiveRuntimeMetadata(
		mergeMaps(cloneAnyMap(closed.Metadata), cloneAnyMap(closed.Runtime.Metadata)),
		mergeInteractiveCapabilities(existing.Capabilities, closed.Runtime.Capabilities),
		observation,
		interactiveOperation(execution.InteractiveOperationClose, s.nowMilli(), 0, 0, request.Metadata),
	)
	handle, err := s.CloseRuntimeHandle(ctx, handleID, RuntimeHandleCloseRequest{
		Reason:   observation.StatusReason,
		Metadata: updateMetadata,
	})
	if err != nil {
		return execution.InteractiveRuntime{}, err
	}
	return interactiveRuntimeFromHandleOrErr(handle)
}

func (s *Service) createRuntimeHandleWithAudit(ctx context.Context, handle execution.RuntimeHandle) (execution.RuntimeHandle, error) {
	var created execution.RuntimeHandle
	create := func(store execution.RuntimeHandleStore, sink EventSink) error {
		if store == nil {
			return execution.ErrRecordNotFound
		}
		var err error
		created, err = store.Create(handle)
		if err != nil {
			return err
		}
		events := runtimeHandleAuditEvents(s.nowMilli(), audit.EventRuntimeHandleCreated, []execution.RuntimeHandle{created})
		if sink != nil {
			return s.emitEventsWithSink(ctx, sink, events)
		}
		s.emitEventsBestEffort(ctx, events)
		return nil
	}

	if s.Runner != nil {
		if err := s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
			repoSet := s.repositoriesWithFallback(repos)
			return create(repoSet.RuntimeHandles, s.eventSinkForRepos(repos))
		}); err != nil {
			return execution.RuntimeHandle{}, err
		}
		return created, nil
	}

	if err := create(s.RuntimeHandles, nil); err != nil {
		return execution.RuntimeHandle{}, err
	}
	return created, nil
}

func (s *Service) requireActiveInteractiveHandle(ctx context.Context, handleID string) (execution.RuntimeHandle, execution.InteractiveRuntime, error) {
	handle, err := s.getRuntimeHandleRecord(ctx, handleID)
	if err != nil {
		return execution.RuntimeHandle{}, execution.InteractiveRuntime{}, err
	}
	runtime, err := interactiveRuntimeFromHandleOrErr(handle)
	if err != nil {
		return execution.RuntimeHandle{}, execution.InteractiveRuntime{}, err
	}
	if !isRuntimeHandleActive(handle) {
		return execution.RuntimeHandle{}, execution.InteractiveRuntime{}, ErrRuntimeHandleNotActive
	}
	return handle, runtime, nil
}

func (s *Service) persistInteractiveRuntimeState(ctx context.Context, handleID string, current execution.InteractiveRuntime, capabilities *execution.InteractiveCapabilities, observation *execution.InteractiveObservation, operation *execution.InteractiveOperation, metadata map[string]any) (execution.InteractiveRuntime, error) {
	if observation != nil && observation.Closed {
		reason := firstNonEmptyString(observation.StatusReason, current.Observation.StatusReason, "interactive runtime closed")
		handle, err := s.CloseRuntimeHandle(ctx, handleID, RuntimeHandleCloseRequest{
			Reason: reason,
			Metadata: execution.ApplyInteractiveRuntimeMetadata(
				cloneAnyMap(metadata),
				capabilities,
				observation,
				operation,
			),
		})
		if err != nil {
			return execution.InteractiveRuntime{}, err
		}
		return interactiveRuntimeFromHandleOrErr(handle)
	}
	return s.UpdateInteractiveRuntime(ctx, handleID, InteractiveRuntimeUpdate{
		Capabilities:  capabilities,
		Observation:   observation,
		LastOperation: operation,
		Metadata:      metadata,
	})
}

func interactiveRuntimeFromHandleOrErr(handle execution.RuntimeHandle) (execution.InteractiveRuntime, error) {
	out, ok := execution.InteractiveRuntimeFromHandle(handle)
	if !ok {
		return execution.InteractiveRuntime{}, ErrInteractiveRuntimeNotFound
	}
	return out, nil
}

func interactiveCapabilitiesPtr(value execution.InteractiveCapabilities) *execution.InteractiveCapabilities {
	if !value.Reopen && !value.View && !value.Write && !value.Close {
		return nil
	}
	out := value
	return &out
}

func mergeInteractiveCapabilities(base, next execution.InteractiveCapabilities) *execution.InteractiveCapabilities {
	if next != (execution.InteractiveCapabilities{}) {
		return &next
	}
	if base != (execution.InteractiveCapabilities{}) {
		return &base
	}
	return nil
}

func mergeInteractiveObservation(base, next execution.InteractiveObservation) *execution.InteractiveObservation {
	if hasInteractiveObservation(next) {
		out := next
		if out.Status == "" {
			out.Status = base.Status
		}
		if out.StatusReason == "" {
			out.StatusReason = base.StatusReason
		}
		if out.Snapshot.ArtifactID == "" {
			out.Snapshot = base.Snapshot
		}
		if out.UpdatedAt == 0 {
			out.UpdatedAt = base.UpdatedAt
		}
		if out.Metadata == nil {
			out.Metadata = cloneAnyMap(base.Metadata)
		}
		if out.ExitCode == nil && base.ExitCode != nil {
			code := *base.ExitCode
			out.ExitCode = &code
		}
		return &out
	}
	if hasInteractiveObservation(base) {
		out := base
		return &out
	}
	return nil
}

func interactiveOperation(kind execution.InteractiveOperationKind, at, offset, bytes int64, metadata map[string]any) *execution.InteractiveOperation {
	return &execution.InteractiveOperation{
		Kind:     kind,
		At:       at,
		Offset:   offset,
		Bytes:    bytes,
		Metadata: cloneAnyMap(metadata),
	}
}

func normalizeInteractiveObservation(observation execution.InteractiveObservation, status execution.RuntimeHandleStatus, fallbackReason string, now int64) execution.InteractiveObservation {
	if observation.Status == "" {
		observation.Status = string(status)
	}
	if observation.StatusReason == "" {
		observation.StatusReason = fallbackReason
	}
	if observation.UpdatedAt == 0 {
		observation.UpdatedAt = now
	}
	return observation
}

func hasInteractiveObservation(value execution.InteractiveObservation) bool {
	return value.NextOffset != 0 ||
		value.Closed ||
		value.ExitCode != nil ||
		value.Status != "" ||
		value.StatusReason != "" ||
		value.Snapshot.ArtifactID != "" ||
		len(value.Snapshot.Metadata) > 0 ||
		value.UpdatedAt != 0 ||
		len(value.Metadata) > 0
}
