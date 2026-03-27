package runtime

import (
	"context"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/persistence"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

const (
	sessionApprovalScopeKey   = "approval_scope_kind"
	sessionApprovalScopeEntry = "session_entry"
	sessionApprovalStepID     = "__session_approval_gate__"
	sessionApprovalToolName   = "session.approval"
	sessionApprovalStepTitle  = "session approval gate"
)

type SessionApprovalRequest struct {
	Reason      string         `json:"reason,omitempty"`
	MatchedRule string         `json:"matched_rule,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

func (s *Service) RequestSessionApproval(ctx context.Context, sessionID string, request SessionApprovalRequest) (approval.Record, session.State, error) {
	now := s.nowMilli()
	created := approval.Record{}
	updated := session.State{}
	events := []audit.Event{}

	persist := func(repos persistence.RepositorySet) error {
		if repos.Sessions == nil {
			return session.ErrSessionNotFound
		}
		if repos.Approvals == nil {
			return approval.ErrApprovalNotFound
		}
		st, err := repos.Sessions.Get(sessionID)
		if err != nil {
			return err
		}
		if err := validateSessionApprovalGateState(repos, st); err != nil {
			return err
		}

		gateStep := sessionApprovalGateStep(now)
		reqMetadata := cloneAnyMap(request.Metadata)
		reqMetadata[sessionApprovalScopeKey] = sessionApprovalScopeEntry
		reqMetadata[approvalResumeContextKey] = approvalResumeContext{
			Kind:    approvalResumeContextSessionGate,
			StepID:  gateStep.StepID,
			CycleID: executionCycleIDFromStep(gateStep),
		}
		rec, err := repos.Approvals.CreatePending(approval.Request{
			SessionID:   sessionID,
			TaskID:      st.TaskID,
			StepID:      gateStep.StepID,
			ToolName:    gateStep.Action.ToolName,
			Reason:      request.Reason,
			MatchedRule: request.MatchedRule,
			Step:        gateStep,
			Metadata:    reqMetadata,
		})
		if err != nil {
			return err
		}
		created = rec

		next := st
		next.ExecutionState = session.ExecutionAwaitingApproval
		next.PendingApprovalID = rec.ApprovalID
		next.CurrentStepID = gateStep.StepID
		next.InFlightStepID = ""
		persistedState, err := persistSessionUpdate(repos.Sessions, next, "")
		if err != nil {
			rollbackErr := terminalizeApprovalForRollback(repos.Approvals, rec, "session_approval_request_persistence_failed", now)
			return joinRollbackError(err, rollbackErr)
		}
		updated = persistedState

		events = []audit.Event{{
			EventID:     "evt_" + created.ApprovalID,
			Type:        audit.EventApprovalRequested,
			SessionID:   created.SessionID,
			TaskID:      updated.TaskID,
			StepID:      created.StepID,
			ApprovalID:  created.ApprovalID,
			CycleID:     executionCycleIDFromStep(created.Step),
			TraceID:     created.ApprovalID,
			CausationID: created.ApprovalID,
			Payload: map[string]any{
				"approval_id":  created.ApprovalID,
				"reason":       created.Reason,
				"matched_rule": created.MatchedRule,
				"tool_name":    created.ToolName,
				"scope":        sessionApprovalScopeEntry,
			},
			CreatedAt: now,
		}}
		return nil
	}

	if s.Runner != nil {
		if err := s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
			repoSet := s.repositoriesWithFallback(repos)
			if err := persist(repoSet); err != nil {
				return err
			}
			return s.emitEventsWithSink(ctx, s.eventSinkForRepos(repos), events)
		}); err != nil {
			return approval.Record{}, session.State{}, err
		}
	} else {
		if err := persist(s.repositoriesWithFallback(persistence.RepositorySet{})); err != nil {
			return approval.Record{}, session.State{}, err
		}
		s.emitEventsBestEffort(ctx, events)
	}

	s.exportApprovalRequestObservability(ctx, updated, execution.Attempt{
		SessionID: updated.SessionID,
		TaskID:    updated.TaskID,
		StepID:    created.StepID,
		CycleID:   executionCycleIDFromStep(created.Step),
		TraceID:   created.ApprovalID,
		Step:      created.Step,
		StartedAt: created.RequestedAt,
	}, created)
	return created, updated, nil
}

func validateSessionApprovalGateState(repos persistence.RepositorySet, st session.State) error {
	if isTerminalPhase(st.Phase) {
		return ErrSessionTerminal
	}
	if st.PendingApprovalID != "" {
		return ErrSessionAwaitingApproval
	}
	if st.ExecutionState == session.ExecutionBlocked || hasCurrentBlockedRuntime(st) {
		return ErrSessionBlocked
	}
	if st.ExecutionState == session.ExecutionInFlight || st.ExecutionState == session.ExecutionInterrupted {
		return ErrSessionApprovalGateTooLate
	}
	if st.InFlightStepID != "" {
		return ErrSessionApprovalGateTooLate
	}
	if repos.Attempts != nil {
		attempts, err := repos.Attempts.List(st.SessionID)
		if err != nil {
			return err
		}
		if len(attempts) > 0 {
			return ErrSessionApprovalGateTooLate
		}
	}
	return nil
}

func sessionApprovalGateStep(now int64) plan.StepSpec {
	step := plan.StepSpec{
		StepID:    sessionApprovalStepID,
		Title:     sessionApprovalStepTitle,
		Action:    action.Spec{ToolName: sessionApprovalToolName},
		Status:    plan.StepBlocked,
		StartedAt: now,
		Metadata: map[string]any{
			sessionApprovalScopeKey: sessionApprovalScopeEntry,
		},
	}
	ensureExecutionCycleID(&step, "")
	return step
}

func isSessionApprovalRecord(rec approval.Record) bool {
	if resumeContext, ok := approvalResumeContextFromMetadata(rec.Metadata); ok && resumeContext.Kind == approvalResumeContextSessionGate {
		return true
	}
	if rec.Step.Action.ToolName == sessionApprovalToolName && rec.Step.StepID == sessionApprovalStepID {
		return true
	}
	scope, _ := rec.Metadata[sessionApprovalScopeKey].(string)
	return scope == sessionApprovalScopeEntry
}

func (s *Service) resumeSessionApprovalGateWithLease(ctx context.Context, st session.State, leaseID string, rec approval.Record) (StepRunOutput, error) {
	now := s.nowMilli()
	step := cloneStepSpec(rec.Step)
	if step.StepID == "" {
		step = sessionApprovalGateStep(now)
	}
	step.Status = plan.StepCompleted
	step.FinishedAt = now

	finalizedApproval := cloneApprovalRecord(rec)
	if finalizedApproval.Reply == approval.ReplyOnce {
		finalizedApproval.Status = approval.StatusConsumed
		finalizedApproval.ConsumedAt = now
	}
	finalizedApproval.Version++

	next := st
	next.PendingApprovalID = ""
	next.ExecutionState = session.ExecutionIdle
	next.InFlightStepID = ""
	if next.CurrentStepID == sessionApprovalStepID {
		next.CurrentStepID = ""
	}

	updated := session.State{}
	persist := func(repos persistence.RepositorySet) error {
		persistedState, err := persistSessionUpdate(repos.Sessions, next, leaseID)
		if err != nil {
			return err
		}
		updated = persistedState
		if repos.Approvals != nil {
			if err := repos.Approvals.Update(finalizedApproval); err != nil {
				rollbackErr := rollbackSessionState(repos.Sessions, st, persistedState, leaseID)
				return joinRollbackError(err, rollbackErr)
			}
		}
		return nil
	}

	if s.Runner != nil {
		if err := s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
			return persist(s.repositoriesWithFallback(repos))
		}); err != nil {
			return StepRunOutput{}, err
		}
	} else {
		if err := persist(s.repositoriesWithFallback(persistence.RepositorySet{})); err != nil {
			return StepRunOutput{}, err
		}
	}

	return StepRunOutput{
		Session: updated,
		Execution: ExecutionResult{
			Step:   step,
			Policy: PolicyDecision{Decision: permissionDecisionForSessionApproval(finalizedApproval)},
		},
	}, nil
}

func permissionDecisionForSessionApproval(rec approval.Record) permission.Decision {
	switch rec.Reply {
	case approval.ReplyAlways:
		return permission.Decision{
			Action:      permission.Allow,
			Reason:      "session approval previously granted",
			MatchedRule: "approval/session/always",
		}
	default:
		return permission.Decision{
			Action:      permission.Allow,
			Reason:      "session approval granted",
			MatchedRule: "approval/session/once",
		}
	}
}
