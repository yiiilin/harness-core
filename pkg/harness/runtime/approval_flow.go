package runtime

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/capability"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/persistence"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

func (s *Service) RespondApproval(approvalID string, response approval.Response) (approval.Record, session.State, error) {
	rec, err := s.GetApproval(approvalID)
	if err != nil {
		return approval.Record{}, session.State{}, err
	}
	st, err := s.GetSession(rec.SessionID)
	if err != nil {
		return approval.Record{}, session.State{}, err
	}
	if rec.Status != approval.StatusPending || st.PendingApprovalID != approvalID {
		return approval.Record{}, session.State{}, approval.ErrApprovalNotPending
	}
	if !approval.ValidReply(response.Reply) {
		return approval.Record{}, session.State{}, approval.ErrInvalidReply
	}

	now := s.nowMilli()
	originalApproval := cloneApprovalRecord(rec)
	rec.Reply = response.Reply
	rec.Metadata = mergeApprovalResponseMetadata(rec.Metadata, response)
	rec.RespondedAt = now

	events := []audit.Event{}
	appendEvent := func(eventType string, payload map[string]any) {
		events = append(events, audit.Event{
			EventID:     "evt_" + uuid.NewString(),
			Type:        eventType,
			SessionID:   rec.SessionID,
			TaskID:      st.TaskID,
			ApprovalID:  rec.ApprovalID,
			StepID:      rec.StepID,
			CycleID:     executionCycleIDFromStep(rec.Step),
			TraceID:     rec.ApprovalID,
			CausationID: rec.ApprovalID,
			Payload:     payload,
			CreatedAt:   now,
		})
	}

	var updatedPlan *plan.Spec
	var updatedTaskUpdated bool
	var updatedTaskErr error
	step := rec.Step
	originalStep := cloneStepSpec(step)
	var originalAttempt execution.Attempt
	var hadOriginalAttempt bool
	var originalTask task.Record
	var hadOriginalTask bool
	if response.Reply == approval.ReplyReject {
		originalAttempt, hadOriginalAttempt, err = findBlockedAttemptForApprovalInStore(s.Attempts, rec.SessionID, rec)
		if err != nil {
			return approval.Record{}, session.State{}, err
		}
		if hadOriginalAttempt {
			originalAttempt = cloneAttemptRecord(originalAttempt)
		}
		if s.Tasks != nil && st.TaskID != "" {
			originalTask, err = s.Tasks.Get(st.TaskID)
			if err != nil {
				return approval.Record{}, session.State{}, err
			}
			hadOriginalTask = true
		}
	}

	switch response.Reply {
	case approval.ReplyReject:
		rec.Status = approval.StatusRejected
		step.Status = plan.StepFailed
		step.FinishedAt = now
		appendEvent(audit.EventApprovalRejected, map[string]any{
			"approval_id": approvalID,
			"tool_name":   rec.ToolName,
			"reason":      rec.Reason,
		})
		next := TransitionDecision{From: st.Phase, To: TransitionFailed, StepID: rec.StepID, Reason: "approval rejected"}
		appendEvent(audit.EventStateChanged, map[string]any{"from": st.Phase, "to": next.To, "reason": next.Reason})
		st = ApplyTransition(st, next)
		st.PendingApprovalID = ""
		st.ExecutionState = session.ExecutionIdle
		st.InFlightStepID = ""
	case approval.ReplyOnce, approval.ReplyAlways:
		rec.Status = approval.StatusApproved
		appendEvent(audit.EventApprovalApproved, map[string]any{
			"approval_id": approvalID,
			"reply":       response.Reply,
			"tool_name":   rec.ToolName,
		})
		st.ExecutionState = session.ExecutionIdle
	}
	rec.Version++
	st.Version++

	if s.Runner != nil {
		err = s.Runner.Within(context.Background(), func(repos persistence.RepositorySet) error {
			repoSet := s.repositoriesWithFallback(repos)
			if repoSet.Approvals == nil {
				return approval.ErrApprovalNotFound
			}
			if err := repoSet.Approvals.Update(rec); err != nil {
				return err
			}
			if response.Reply == approval.ReplyReject {
				if err := finalizeBlockedAttemptForApprovalInStore(repoSet.Attempts, rec.SessionID, rec, execution.AttemptFailed, step, string(response.Reply), now); err != nil {
					return err
				}
			}
			if response.Reply == approval.ReplyReject {
				pl, err := updateLatestPlanStepInStore(repoSet.Plans, rec.SessionID, step)
				if err != nil {
					return err
				}
				updatedPlan = pl
				if _, updatedTaskErr = updateTaskForTerminalInStore(repoSet.Tasks, st); updatedTaskErr != nil {
					return updatedTaskErr
				}
				updatedTaskUpdated = true
			}
			if err := repoSet.Sessions.Update(st); err != nil {
				return err
			}
			if err := s.emitEventsWithSink(context.Background(), s.eventSinkForRepos(repos), events); err != nil {
				return err
			}
			return nil
		})
	} else {
		if s.Approvals == nil {
			return approval.Record{}, session.State{}, approval.ErrApprovalNotFound
		}
		approvalUpdated := false
		attemptUpdated := false
		planUpdated := false
		taskUpdated := false
		if err := s.Approvals.Update(rec); err != nil {
			return approval.Record{}, session.State{}, err
		}
		approvalUpdated = true
		if response.Reply == approval.ReplyReject {
			if err := finalizeBlockedAttemptForApprovalInStore(s.Attempts, rec.SessionID, rec, execution.AttemptFailed, step, string(response.Reply), now); err != nil {
				rollbackErr := restoreApprovalRecord(s.Approvals, originalApproval)
				return approval.Record{}, session.State{}, joinRollbackError(err, rollbackErr)
			}
			attemptUpdated = hadOriginalAttempt
		}
		if response.Reply == approval.ReplyReject {
			updatedPlan, err = updateLatestPlanStepInStore(s.Plans, rec.SessionID, step)
			if err != nil {
				rollbackErr := error(nil)
				if attemptUpdated {
					rollbackErr = restoreAttemptRecord(s.Attempts, originalAttempt)
				}
				rollbackErr = joinRollbackError(rollbackErr, restoreApprovalRecord(s.Approvals, originalApproval))
				return approval.Record{}, session.State{}, joinRollbackError(err, rollbackErr)
			}
			planUpdated = true
			if _, updatedTaskErr = updateTaskForTerminalInStore(s.Tasks, st); updatedTaskErr != nil {
				rollbackErr := error(nil)
				if planUpdated {
					rollbackErr = restorePlanStepState(s.Plans, rec.SessionID, originalStep)
				}
				if attemptUpdated {
					rollbackErr = joinRollbackError(rollbackErr, restoreAttemptRecord(s.Attempts, originalAttempt))
				}
				rollbackErr = joinRollbackError(rollbackErr, restoreApprovalRecord(s.Approvals, originalApproval))
				return approval.Record{}, session.State{}, joinRollbackError(updatedTaskErr, rollbackErr)
			}
			updatedTaskUpdated = true
			taskUpdated = hadOriginalTask
		}
		if err := s.Sessions.Update(st); err != nil {
			rollbackErr := error(nil)
			if taskUpdated {
				rollbackErr = restoreTaskRecord(s.Tasks, originalTask)
			}
			if planUpdated {
				rollbackErr = joinRollbackError(rollbackErr, restorePlanStepState(s.Plans, rec.SessionID, originalStep))
			}
			if attemptUpdated {
				rollbackErr = joinRollbackError(rollbackErr, restoreAttemptRecord(s.Attempts, originalAttempt))
			}
			if approvalUpdated {
				rollbackErr = joinRollbackError(rollbackErr, restoreApprovalRecord(s.Approvals, originalApproval))
			}
			return approval.Record{}, session.State{}, joinRollbackError(err, rollbackErr)
		}
		s.emitEventsBestEffort(context.Background(), events)
	}
	if updatedTaskUpdated && updatedTaskErr != nil {
		return approval.Record{}, session.State{}, updatedTaskErr
	}
	if err == nil {
		s.exportApprovalResponseObservability(context.Background(), st, rec)
	}
	_ = updatedPlan
	return rec, st, err
}

func (s *Service) ResumePendingApproval(ctx context.Context, sessionID string) (StepRunOutput, error) {
	return s.resumePendingApprovalWithLease(ctx, sessionID, "")
}

func (s *Service) ResumeClaimedApproval(ctx context.Context, sessionID, leaseID string) (StepRunOutput, error) {
	return s.resumePendingApprovalWithLease(ctx, sessionID, leaseID)
}

func (s *Service) resumePendingApprovalWithLease(ctx context.Context, sessionID, leaseID string) (StepRunOutput, error) {
	st, err := s.ensureSessionLease(sessionID, leaseID)
	if err != nil {
		return StepRunOutput{}, err
	}
	if st.PendingApprovalID == "" {
		return StepRunOutput{}, ErrNoPendingApproval
	}
	rec, err := s.GetApproval(st.PendingApprovalID)
	if err != nil {
		return StepRunOutput{}, err
	}
	if rec.Status != approval.StatusApproved {
		return StepRunOutput{}, ErrApprovalNotResolved
	}
	if isSessionApprovalRecord(rec) {
		return s.resumeSessionApprovalGateWithLease(ctx, st, leaseID, rec)
	}
	decision, ok := s.ResumePolicy.Resolve(rec, rec.Step)
	if !ok {
		return StepRunOutput{}, ErrApprovalNotResolved
	}
	return s.runStepWithDecision(ctx, sessionID, leaseID, rec.Step, &decision, &rec)
}

func (s *Service) resolvePendingApprovalForSession(ctx context.Context, sessionID, leaseID string) (*StepRunOutput, bool, error) {
	st, err := s.GetSession(sessionID)
	if err != nil {
		return nil, false, err
	}
	if st.PendingApprovalID == "" {
		return nil, false, nil
	}
	rec, err := s.GetApproval(st.PendingApprovalID)
	if err != nil {
		return nil, false, err
	}
	switch rec.Status {
	case approval.StatusPending:
		return nil, true, nil
	case approval.StatusApproved:
		resumed, err := s.resumePendingApprovalWithLease(ctx, sessionID, leaseID)
		if err != nil {
			return nil, true, err
		}
		if isSessionApprovalRecord(rec) {
			return nil, false, nil
		}
		return &resumed, true, nil
	default:
		return nil, true, ErrApprovalNotResolved
	}
}

func (s *Service) findReusableApprovalDecision(ctx context.Context, state session.State, step plan.StepSpec, decision permission.Decision) (*permission.Decision, *approval.Record) {
	scope := s.buildApprovalReuseScope(ctx, state, step, decision)
	records, err := s.ListApprovals(state.SessionID)
	if err != nil {
		return nil, nil
	}
	for _, rec := range records {
		if rec.Status != approval.StatusApproved || rec.Reply != approval.ReplyAlways {
			continue
		}
		if !approvalReuseScopeMatches(rec.Metadata, scope) {
			continue
		}
		copyRec := rec
		allow := permission.Decision{
			Action:      permission.Allow,
			Reason:      "approval previously granted",
			MatchedRule: "approval/always",
		}
		return &allow, &copyRec
	}
	return nil, nil
}

const (
	approvalRequestScopeKey  = "request_scope"
	approvalResumeContextKey = "resume_context"
	approvalResponseKey      = "response"
)

type approvalReuseScope struct {
	ToolName               string `json:"tool_name"`
	RequestedToolVersion   string `json:"requested_tool_version,omitempty"`
	ResolvedToolVersion    string `json:"resolved_tool_version,omitempty"`
	MatchedRule            string `json:"matched_rule,omitempty"`
	ArgsJSON               string `json:"args_json,omitempty"`
	CapabilityType         string `json:"capability_type,omitempty"`
	RiskLevel              string `json:"risk_level,omitempty"`
	DefinitionMetadataJSON string `json:"definition_metadata_json,omitempty"`
}

type approvalResumeContext struct {
	Kind      string `json:"kind"`
	AttemptID string `json:"attempt_id,omitempty"`
	StepID    string `json:"step_id,omitempty"`
	CycleID   string `json:"cycle_id,omitempty"`
}

const (
	approvalResumeContextStepAttempt = "step_attempt"
	approvalResumeContextSessionGate = "session_entry"
)

func (s *Service) buildApprovalReuseScope(ctx context.Context, state session.State, step plan.StepSpec, decision permission.Decision) approvalReuseScope {
	scope := approvalReuseScope{
		ToolName:             step.Action.ToolName,
		RequestedToolVersion: step.Action.ToolVersion,
		MatchedRule:          decision.MatchedRule,
		ArgsJSON:             marshalApprovalScopeValue(step.Action.Args),
	}
	resolution, err := s.ResolveCapability(ctx, capability.Request{
		SessionID: state.SessionID,
		TaskID:    state.TaskID,
		StepID:    step.StepID,
		Action:    step.Action,
	})
	if err != nil {
		return scope
	}
	scope.ResolvedToolVersion = resolution.Definition.Version
	scope.CapabilityType = resolution.Definition.CapabilityType
	scope.RiskLevel = string(resolution.Definition.RiskLevel)
	scope.DefinitionMetadataJSON = marshalApprovalScopeValue(resolution.Definition.Metadata)
	return scope
}

func approvalReuseScopeMatches(metadata map[string]any, current approvalReuseScope) bool {
	stored, ok := approvalScopeFromMetadata(metadata)
	if !ok {
		return false
	}
	return stored == current
}

func approvalScopeFromMetadata(metadata map[string]any) (approvalReuseScope, bool) {
	raw, ok := metadata[approvalRequestScopeKey]
	if !ok {
		return approvalReuseScope{}, false
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return approvalReuseScope{}, false
	}
	var scope approvalReuseScope
	if err := json.Unmarshal(b, &scope); err != nil {
		return approvalReuseScope{}, false
	}
	if scope.ToolName == "" {
		return approvalReuseScope{}, false
	}
	return scope, true
}

func approvalMetadataForRequest(scope approvalReuseScope, resume approvalResumeContext) map[string]any {
	metadata := map[string]any{
		approvalRequestScopeKey: scope,
	}
	if resume.Kind != "" {
		metadata[approvalResumeContextKey] = resume
	}
	return metadata
}

func mergeApprovalResponseMetadata(existing map[string]any, response approval.Response) map[string]any {
	merged := cloneAnyMap(existing)
	if response.Metadata == nil {
		return merged
	}
	merged[approvalResponseKey] = cloneAnyMap(response.Metadata)
	return merged
}

func cloneAnyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func marshalApprovalScopeValue(value any) string {
	if value == nil {
		return ""
	}
	b, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(b)
}

func approvalResumeContextFromMetadata(metadata map[string]any) (approvalResumeContext, bool) {
	raw, ok := metadata[approvalResumeContextKey]
	if !ok {
		return approvalResumeContext{}, false
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return approvalResumeContext{}, false
	}
	var ctx approvalResumeContext
	if err := json.Unmarshal(b, &ctx); err != nil {
		return approvalResumeContext{}, false
	}
	if ctx.Kind == "" {
		return approvalResumeContext{}, false
	}
	return ctx, true
}

func approvalResumeContextRequiresBlockedAttempt(rec approval.Record) bool {
	ctx, ok := approvalResumeContextFromMetadata(rec.Metadata)
	return ok && ctx.Kind == approvalResumeContextStepAttempt && ctx.AttemptID != ""
}

func findBlockedAttemptForApprovalInStore(store execution.AttemptStore, sessionID string, rec approval.Record) (execution.Attempt, bool, error) {
	if store == nil || rec.ApprovalID == "" {
		return execution.Attempt{}, false, nil
	}
	if resumeContext, ok := approvalResumeContextFromMetadata(rec.Metadata); ok && resumeContext.AttemptID != "" {
		attempt, exactMatch, err := findBlockedAttemptByIDInStore(store, sessionID, rec.ApprovalID, resumeContext.AttemptID)
		if err != nil {
			return execution.Attempt{}, false, err
		}
		if exactMatch {
			return attempt, true, nil
		}
		if resumeContext.Kind == approvalResumeContextStepAttempt {
			return execution.Attempt{}, false, ErrApprovalResumeContextMissing
		}
	}
	return findLatestBlockedAttemptInStore(store, sessionID, rec.ApprovalID)
}

func findBlockedAttemptForApprovalProjectionInStore(store execution.AttemptStore, sessionID string, rec approval.Record) (execution.Attempt, bool, error) {
	if store == nil || rec.ApprovalID == "" {
		return execution.Attempt{}, false, nil
	}
	if resumeContext, ok := approvalResumeContextFromMetadata(rec.Metadata); ok && resumeContext.AttemptID != "" {
		attempt, exactMatch, err := findBlockedAttemptByIDInStore(store, sessionID, rec.ApprovalID, resumeContext.AttemptID)
		if err != nil {
			return execution.Attempt{}, false, err
		}
		if exactMatch {
			return attempt, true, nil
		}
		if resumeContext.Kind == approvalResumeContextStepAttempt {
			return execution.Attempt{}, false, nil
		}
	}
	return findLatestBlockedAttemptInStore(store, sessionID, rec.ApprovalID)
}

func findBlockedAttemptByIDInStore(store execution.AttemptStore, sessionID, approvalID, attemptID string) (execution.Attempt, bool, error) {
	if store == nil || approvalID == "" || attemptID == "" {
		return execution.Attempt{}, false, nil
	}
	attempts, err := store.List(sessionID)
	if err != nil {
		return execution.Attempt{}, false, err
	}
	for _, attempt := range attempts {
		if attempt.AttemptID != attemptID || attempt.ApprovalID != approvalID || attempt.Status != execution.AttemptBlocked {
			continue
		}
		return attempt, true, nil
	}
	return execution.Attempt{}, false, nil
}

func finalizeBlockedAttemptInStore(store execution.AttemptStore, sessionID, approvalID string, status execution.AttemptStatus, step plan.StepSpec, reply string, now int64) error {
	attempt, ok, err := findLatestBlockedAttemptInStore(store, sessionID, approvalID)
	if err != nil || !ok {
		return err
	}
	attempt.Status = status
	attempt.Step = step
	if attempt.Metadata == nil {
		attempt.Metadata = map[string]any{}
	}
	attempt.Metadata["approval_reply"] = reply
	if attempt.FinishedAt == 0 {
		attempt.FinishedAt = now
	}
	return store.Update(attempt)
}

func finalizeBlockedAttemptForApprovalInStore(store execution.AttemptStore, sessionID string, rec approval.Record, status execution.AttemptStatus, step plan.StepSpec, reply string, now int64) error {
	attempt, ok, err := findBlockedAttemptForApprovalInStore(store, sessionID, rec)
	if err != nil || !ok {
		return err
	}
	attempt.Status = status
	attempt.Step = step
	if attempt.Metadata == nil {
		attempt.Metadata = map[string]any{}
	}
	attempt.Metadata["approval_reply"] = reply
	if attempt.FinishedAt == 0 {
		attempt.FinishedAt = now
	}
	return store.Update(attempt)
}

func findLatestBlockedAttemptInStore(store execution.AttemptStore, sessionID, approvalID string) (execution.Attempt, bool, error) {
	if store == nil || approvalID == "" {
		return execution.Attempt{}, false, nil
	}
	attempts, err := store.List(sessionID)
	if err != nil {
		return execution.Attempt{}, false, err
	}
	for i := len(attempts) - 1; i >= 0; i-- {
		if attempts[i].ApprovalID != approvalID || attempts[i].Status != execution.AttemptBlocked {
			continue
		}
		return attempts[i], true, nil
	}
	return execution.Attempt{}, false, nil
}
