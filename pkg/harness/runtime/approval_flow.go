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
				if err := finalizeBlockedAttemptInStore(repoSet.Attempts, rec.SessionID, rec.ApprovalID, execution.AttemptFailed, step, string(response.Reply), now); err != nil {
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
		if err := s.Approvals.Update(rec); err != nil {
			return approval.Record{}, session.State{}, err
		}
		if response.Reply == approval.ReplyReject {
			if err := finalizeBlockedAttemptInStore(s.Attempts, rec.SessionID, rec.ApprovalID, execution.AttemptFailed, step, string(response.Reply), now); err != nil {
				return approval.Record{}, session.State{}, err
			}
		}
		if response.Reply == approval.ReplyReject {
			updatedPlan, _ = updateLatestPlanStepInStore(s.Plans, rec.SessionID, step)
			if _, updatedTaskErr = updateTaskForTerminalInStore(s.Tasks, st); updatedTaskErr != nil {
				return approval.Record{}, session.State{}, updatedTaskErr
			}
			updatedTaskUpdated = true
		}
		if err := s.Sessions.Update(st); err != nil {
			return approval.Record{}, session.State{}, err
		}
		_ = s.emitEvents(context.Background(), events)
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
	approvalRequestScopeKey = "request_scope"
	approvalResponseKey     = "response"
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

func approvalMetadataForRequest(scope approvalReuseScope) map[string]any {
	return map[string]any{
		approvalRequestScopeKey: scope,
	}
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
