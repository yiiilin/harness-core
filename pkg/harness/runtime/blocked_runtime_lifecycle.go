package runtime

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/persistence"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

type BlockedRuntimeRequest struct {
	Kind      execution.BlockedRuntimeKind      `json:"kind"`
	Subject   execution.BlockedRuntimeSubject   `json:"subject,omitempty"`
	Condition execution.BlockedRuntimeCondition `json:"condition"`
	Metadata  map[string]any                    `json:"metadata,omitempty"`
}

type BlockedRuntimeResponse struct {
	Status   execution.BlockedRuntimeStatus `json:"status"`
	Metadata map[string]any                 `json:"metadata,omitempty"`
}

type BlockedRuntimeAbortRequest struct {
	Reason   string         `json:"reason,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type ConfirmationRequest struct {
	Subject    execution.BlockedRuntimeSubject `json:"subject,omitempty"`
	WaitingFor string                          `json:"waiting_for,omitempty"`
	Metadata   map[string]any                  `json:"metadata,omitempty"`
}

func (s *Service) RequestConfirmation(ctx context.Context, sessionID string, request ConfirmationRequest) (execution.BlockedRuntimeRecord, session.State, error) {
	waitingFor := request.WaitingFor
	if waitingFor == "" {
		waitingFor = "human_confirmation"
	}
	return s.CreateBlockedRuntime(ctx, sessionID, BlockedRuntimeRequest{
		Kind:    execution.BlockedRuntimeConfirmation,
		Subject: cloneBlockedRuntimeSubject(request.Subject),
		Condition: execution.BlockedRuntimeCondition{
			Kind:       execution.BlockedRuntimeConditionConfirmation,
			WaitingFor: waitingFor,
		},
		Metadata: cloneAnyMap(request.Metadata),
	})
}

func (s *Service) CreateBlockedRuntime(ctx context.Context, sessionID string, request BlockedRuntimeRequest) (execution.BlockedRuntimeRecord, session.State, error) {
	if err := validateBlockedRuntimeRequest(request); err != nil {
		return execution.BlockedRuntimeRecord{}, session.State{}, err
	}

	now := s.nowMilli()
	prepared := execution.BlockedRuntimeRecord{
		BlockedRuntimeID: "blocked_" + uuid.NewString(),
		Kind:             request.Kind,
		Status:           execution.BlockedRuntimePending,
		SessionID:        sessionID,
		Subject:          cloneBlockedRuntimeSubject(request.Subject),
		Condition:        normalizeBlockedRuntimeCondition(request.Condition),
		Metadata:         cloneAnyMap(request.Metadata),
		RequestedAt:      now,
		UpdatedAt:        now,
	}

	var (
		created execution.BlockedRuntimeRecord
		updated session.State
	)
	persist := func(repos persistence.RepositorySet) error {
		st, err := repos.Sessions.Get(sessionID)
		if err != nil {
			return err
		}
		if err := validateBlockedRuntimeSessionState(st); err != nil {
			return err
		}
		prepared.TaskID = firstNonEmptyString(st.TaskID, prepared.TaskID)

		next := setCurrentBlockedRuntime(st, prepared.BlockedRuntimeID)
		next.ExecutionState = session.ExecutionBlocked
		updated, err = persistSessionUpdate(repos.Sessions, next, "")
		if err != nil {
			return err
		}

		created, err = repos.BlockedRuntimes.Create(prepared)
		if err != nil {
			rollbackErr := rollbackSessionState(repos.Sessions, st, updated, "")
			return joinRollbackError(err, rollbackErr)
		}
		return nil
	}

	if s.Runner != nil {
		if err := s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
			repoSet := s.repositoriesWithFallback(repos)
			if repoSet.BlockedRuntimes == nil {
				return execution.ErrRecordNotFound
			}
			if err := persist(repoSet); err != nil {
				return err
			}
			return s.emitEventsWithSink(ctx, s.eventSinkForRepos(repos), []audit.Event{blockedRuntimeAuditEvent(now, audit.EventBlockedRuntimeCreated, created, updated.TaskID, nil)})
		}); err != nil {
			return execution.BlockedRuntimeRecord{}, session.State{}, err
		}
		return created, updated, nil
	}

	if s.BlockedRuntimes == nil {
		return execution.BlockedRuntimeRecord{}, session.State{}, execution.ErrRecordNotFound
	}
	if err := persist(s.repositoriesWithFallback(persistence.RepositorySet{})); err != nil {
		return execution.BlockedRuntimeRecord{}, session.State{}, err
	}
	s.emitEventsBestEffort(ctx, []audit.Event{blockedRuntimeAuditEvent(now, audit.EventBlockedRuntimeCreated, created, updated.TaskID, nil)})
	return created, updated, nil
}

func (s *Service) RespondBlockedRuntime(ctx context.Context, blockedRuntimeID string, response BlockedRuntimeResponse) (execution.BlockedRuntimeRecord, session.State, error) {
	if err := validateBlockedRuntimeResponse(response); err != nil {
		return execution.BlockedRuntimeRecord{}, session.State{}, err
	}

	now := s.nowMilli()
	var (
		updated execution.BlockedRuntimeRecord
		state   session.State
	)
	persist := func(repos persistence.RepositorySet) error {
		rec, err := repos.BlockedRuntimes.Get(blockedRuntimeID)
		if err != nil {
			return err
		}
		state, err = repos.Sessions.Get(rec.SessionID)
		if err != nil {
			return err
		}
		if err := requireCurrentBlockedRuntime(state, rec); err != nil {
			return err
		}
		if rec.Status != execution.BlockedRuntimePending {
			return ErrBlockedRuntimeNotPending
		}
		if err := validateBlockedRuntimeResponseForKind(rec.Kind, response); err != nil {
			return err
		}
		rec.Status = response.Status
		rec.UpdatedAt = now
		rec.ResolvedAt = now
		rec.Metadata = mergeBlockedRuntimeResponseMetadata(rec.Metadata, response)
		if err := repos.BlockedRuntimes.Update(rec); err != nil {
			return err
		}
		updated = rec
		return nil
	}

	if s.Runner != nil {
		if err := s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
			repoSet := s.repositoriesWithFallback(repos)
			if repoSet.BlockedRuntimes == nil {
				return execution.ErrRecordNotFound
			}
			if err := persist(repoSet); err != nil {
				return err
			}
			return s.emitEventsWithSink(ctx, s.eventSinkForRepos(repos), []audit.Event{blockedRuntimeAuditEvent(now, audit.EventBlockedRuntimeResponded, updated, state.TaskID, map[string]any{
				"response_status": string(response.Status),
			})})
		}); err != nil {
			return execution.BlockedRuntimeRecord{}, session.State{}, err
		}
		return updated, state, nil
	}

	if s.BlockedRuntimes == nil {
		return execution.BlockedRuntimeRecord{}, session.State{}, execution.ErrRecordNotFound
	}
	if err := persist(s.repositoriesWithFallback(persistence.RepositorySet{})); err != nil {
		return execution.BlockedRuntimeRecord{}, session.State{}, err
	}
	s.emitEventsBestEffort(ctx, []audit.Event{blockedRuntimeAuditEvent(now, audit.EventBlockedRuntimeResponded, updated, state.TaskID, map[string]any{
		"response_status": string(response.Status),
	})})
	return updated, state, nil
}

func (s *Service) ResumeBlockedRuntime(ctx context.Context, blockedRuntimeID string) (execution.BlockedRuntimeRecord, session.State, error) {
	return s.resolveBlockedRuntimeState(ctx, blockedRuntimeID, execution.BlockedRuntimeResumed, "", nil, audit.EventBlockedRuntimeResumed)
}

func (s *Service) AbortBlockedRuntime(ctx context.Context, blockedRuntimeID string, request BlockedRuntimeAbortRequest) (execution.BlockedRuntimeRecord, session.State, error) {
	return s.resolveBlockedRuntimeState(ctx, blockedRuntimeID, execution.BlockedRuntimeAborted, request.Reason, request.Metadata, audit.EventBlockedRuntimeAborted)
}

func (s *Service) resolveBlockedRuntimeState(ctx context.Context, blockedRuntimeID string, nextStatus execution.BlockedRuntimeStatus, reason string, metadata map[string]any, eventType string) (execution.BlockedRuntimeRecord, session.State, error) {
	now := s.nowMilli()
	var (
		updatedRecord execution.BlockedRuntimeRecord
		updatedState  session.State
	)
	persist := func(repos persistence.RepositorySet) error {
		rec, err := repos.BlockedRuntimes.Get(blockedRuntimeID)
		if err != nil {
			return err
		}
		st, err := repos.Sessions.Get(rec.SessionID)
		if err != nil {
			return err
		}
		if err := requireCurrentBlockedRuntime(st, rec); err != nil {
			return err
		}
		if rec.Status == execution.BlockedRuntimeResumed || rec.Status == execution.BlockedRuntimeAborted {
			return ErrBlockedRuntimeNotActive
		}

		next := setCurrentBlockedRuntime(st, "")
		next.ExecutionState = session.ExecutionIdle
		updatedState, err = persistSessionUpdate(repos.Sessions, next, "")
		if err != nil {
			return err
		}

		rec.Status = nextStatus
		rec.UpdatedAt = now
		rec.ResolvedAt = now
		rec.Metadata = mergeBlockedRuntimeTerminalMetadata(rec.Metadata, reason, metadata, nextStatus)
		if err := repos.BlockedRuntimes.Update(rec); err != nil {
			rollbackErr := rollbackSessionState(repos.Sessions, st, updatedState, "")
			return joinRollbackError(err, rollbackErr)
		}
		updatedRecord = rec
		return nil
	}

	if s.Runner != nil {
		if err := s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
			repoSet := s.repositoriesWithFallback(repos)
			if repoSet.BlockedRuntimes == nil {
				return execution.ErrRecordNotFound
			}
			if err := persist(repoSet); err != nil {
				return err
			}
			payload := map[string]any{"status": string(nextStatus)}
			if reason != "" {
				payload["reason"] = reason
			}
			return s.emitEventsWithSink(ctx, s.eventSinkForRepos(repos), []audit.Event{blockedRuntimeAuditEvent(now, eventType, updatedRecord, updatedState.TaskID, payload)})
		}); err != nil {
			return execution.BlockedRuntimeRecord{}, session.State{}, err
		}
		return updatedRecord, updatedState, nil
	}

	if s.BlockedRuntimes == nil {
		return execution.BlockedRuntimeRecord{}, session.State{}, execution.ErrRecordNotFound
	}
	if err := persist(s.repositoriesWithFallback(persistence.RepositorySet{})); err != nil {
		return execution.BlockedRuntimeRecord{}, session.State{}, err
	}
	payload := map[string]any{"status": string(nextStatus)}
	if reason != "" {
		payload["reason"] = reason
	}
	s.emitEventsBestEffort(ctx, []audit.Event{blockedRuntimeAuditEvent(now, eventType, updatedRecord, updatedState.TaskID, payload)})
	return updatedRecord, updatedState, nil
}

func validateBlockedRuntimeRequest(request BlockedRuntimeRequest) error {
	switch request.Kind {
	case execution.BlockedRuntimeConfirmation, execution.BlockedRuntimeExternal, execution.BlockedRuntimeInteractive:
	default:
		return ErrInvalidBlockedRuntimeRequest
	}
	if request.Condition.Kind == "" || request.Condition.Kind == execution.BlockedRuntimeConditionApproval {
		return ErrInvalidBlockedRuntimeRequest
	}
	if !blockedRuntimeKindMatchesCondition(request.Kind, request.Condition.Kind) {
		return ErrInvalidBlockedRuntimeRequest
	}
	return nil
}

func validateBlockedRuntimeResponse(response BlockedRuntimeResponse) error {
	switch response.Status {
	case execution.BlockedRuntimeApproved, execution.BlockedRuntimeRejected, execution.BlockedRuntimeConfirmed:
		return nil
	default:
		return ErrInvalidBlockedRuntimeResponse
	}
}

func validateBlockedRuntimeResponseForKind(kind execution.BlockedRuntimeKind, response BlockedRuntimeResponse) error {
	if err := validateBlockedRuntimeResponse(response); err != nil {
		return err
	}
	switch kind {
	case execution.BlockedRuntimeConfirmation:
		if response.Status != execution.BlockedRuntimeConfirmed {
			return ErrInvalidBlockedRuntimeResponse
		}
	case execution.BlockedRuntimeExternal, execution.BlockedRuntimeInteractive:
		if response.Status != execution.BlockedRuntimeApproved && response.Status != execution.BlockedRuntimeRejected {
			return ErrInvalidBlockedRuntimeResponse
		}
	default:
		return ErrInvalidBlockedRuntimeResponse
	}
	return nil
}

func validateBlockedRuntimeSessionState(st session.State) error {
	if isTerminalPhase(st.Phase) {
		return ErrSessionTerminal
	}
	if st.PendingApprovalID != "" {
		return ErrSessionAwaitingApproval
	}
	if hasCurrentBlockedRuntime(st) || st.ExecutionState == session.ExecutionBlocked {
		return ErrBlockedRuntimeAlreadyActive
	}
	return nil
}

func requireCurrentBlockedRuntime(st session.State, rec execution.BlockedRuntimeRecord) error {
	if currentBlockedRuntimeID(st) != rec.BlockedRuntimeID || st.ExecutionState != session.ExecutionBlocked {
		return ErrBlockedRuntimeNotActive
	}
	return nil
}

func blockedRuntimeKindMatchesCondition(kind execution.BlockedRuntimeKind, condition execution.BlockedRuntimeConditionKind) bool {
	switch kind {
	case execution.BlockedRuntimeConfirmation:
		return condition == execution.BlockedRuntimeConditionConfirmation
	case execution.BlockedRuntimeExternal:
		return condition == execution.BlockedRuntimeConditionExternal
	case execution.BlockedRuntimeInteractive:
		return condition == execution.BlockedRuntimeConditionInteractive
	default:
		return false
	}
}

func normalizeBlockedRuntimeCondition(condition execution.BlockedRuntimeCondition) execution.BlockedRuntimeCondition {
	out := condition
	out.Metadata = cloneAnyMap(condition.Metadata)
	if out.WaitingFor == "" {
		out.WaitingFor = string(out.Kind)
	}
	return out
}

func mergeBlockedRuntimeResponseMetadata(existing map[string]any, response BlockedRuntimeResponse) map[string]any {
	merged := cloneAnyMap(existing)
	if len(response.Metadata) == 0 {
		return merged
	}
	merged["response"] = cloneAnyMap(response.Metadata)
	return merged
}

func mergeBlockedRuntimeTerminalMetadata(existing map[string]any, reason string, metadata map[string]any, status execution.BlockedRuntimeStatus) map[string]any {
	merged := cloneAnyMap(existing)
	if status != "" {
		merged["terminal_status"] = string(status)
	}
	if reason != "" {
		merged["terminal_reason"] = reason
	}
	if len(metadata) > 0 {
		merged["terminal_metadata"] = cloneAnyMap(metadata)
	}
	return merged
}

func cloneBlockedRuntimeSubject(subject execution.BlockedRuntimeSubject) execution.BlockedRuntimeSubject {
	out := subject
	out.Metadata = cloneAnyMap(subject.Metadata)
	return out
}

func blockedRuntimeAuditEvent(now int64, eventType string, record execution.BlockedRuntimeRecord, taskID string, payload map[string]any) audit.Event {
	combined := map[string]any{
		"blocked_runtime_id": record.BlockedRuntimeID,
		"kind":               string(record.Kind),
		"status":             string(record.Status),
		"waiting_for":        record.Condition.WaitingFor,
	}
	for key, value := range payload {
		combined[key] = value
	}
	return audit.Event{
		EventID:     "evt_" + uuid.NewString(),
		Type:        eventType,
		SessionID:   record.SessionID,
		TaskID:      taskID,
		StepID:      record.Subject.StepID,
		AttemptID:   record.Subject.AttemptID,
		ActionID:    record.Subject.ActionID,
		CycleID:     record.Subject.CycleID,
		TraceID:     record.BlockedRuntimeID,
		CausationID: record.BlockedRuntimeID,
		Payload:     combined,
		CreatedAt:   now,
	}
}

func blockedRuntimeRecordOrErr(store execution.BlockedRuntimeStore, blockedRuntimeID string) (execution.BlockedRuntimeRecord, error) {
	if store == nil {
		return execution.BlockedRuntimeRecord{}, execution.ErrBlockedRuntimeNotFound
	}
	rec, err := store.Get(blockedRuntimeID)
	if err != nil {
		if errors.Is(err, execution.ErrRecordNotFound) {
			return execution.BlockedRuntimeRecord{}, execution.ErrBlockedRuntimeNotFound
		}
		return execution.BlockedRuntimeRecord{}, err
	}
	return rec, nil
}
