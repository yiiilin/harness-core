package runtime

import (
	"context"
	"encoding/json"
	"time"

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

	now := time.Now().UnixMilli()
	rec.Reply = response.Reply
	rec.Metadata = mergeApprovalResponseMetadata(rec.Metadata, response)
	rec.RespondedAt = now

	events := []audit.Event{}
	appendEvent := func(eventType string, payload map[string]any) {
		events = append(events, audit.Event{
			EventID:   "evt_" + uuid.NewString(),
			Type:      eventType,
			SessionID: rec.SessionID,
			StepID:    rec.StepID,
			Payload:   payload,
			CreatedAt: time.Now().UnixMilli(),
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

	if s.Runner != nil {
		err = s.Runner.Within(context.Background(), func(repos persistence.RepositorySet) error {
			if repos.Approvals == nil {
				return approval.ErrApprovalNotFound
			}
			if err := repos.Approvals.Update(rec); err != nil {
				return err
			}
			if response.Reply == approval.ReplyReject {
				if err := finalizeBlockedAttemptInStore(repos.Attempts, rec.SessionID, rec.ApprovalID, execution.AttemptFailed, step, string(response.Reply)); err != nil {
					return err
				}
			} else if response.Reply == approval.ReplyOnce || response.Reply == approval.ReplyAlways {
				if err := finalizeBlockedAttemptInStore(repos.Attempts, rec.SessionID, rec.ApprovalID, execution.AttemptCompleted, rec.Step, string(response.Reply)); err != nil {
					return err
				}
			}
			if response.Reply == approval.ReplyReject {
				pl, err := updateLatestPlanStepInStore(repos.Plans, rec.SessionID, step)
				if err != nil {
					return err
				}
				updatedPlan = pl
				if _, updatedTaskErr = updateTaskForTerminalInStore(repos.Tasks, st); updatedTaskErr != nil {
					return updatedTaskErr
				}
				updatedTaskUpdated = true
			}
			if err := repos.Sessions.Update(st); err != nil {
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
			if err := finalizeBlockedAttemptInStore(s.Attempts, rec.SessionID, rec.ApprovalID, execution.AttemptFailed, step, string(response.Reply)); err != nil {
				return approval.Record{}, session.State{}, err
			}
		} else if response.Reply == approval.ReplyOnce || response.Reply == approval.ReplyAlways {
			if err := finalizeBlockedAttemptInStore(s.Attempts, rec.SessionID, rec.ApprovalID, execution.AttemptCompleted, rec.Step, string(response.Reply)); err != nil {
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
		if err := s.emitEvents(context.Background(), events); err != nil {
			return approval.Record{}, session.State{}, err
		}
	}
	if updatedTaskUpdated && updatedTaskErr != nil {
		return approval.Record{}, session.State{}, updatedTaskErr
	}
	_ = updatedPlan
	return rec, st, err
}

func (s *Service) ResumePendingApproval(ctx context.Context, sessionID string) (StepRunOutput, error) {
	st, err := s.GetSession(sessionID)
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
	return s.runStepWithDecision(ctx, sessionID, rec.Step, &decision, &rec)
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

func finalizeBlockedAttemptInStore(store execution.AttemptStore, sessionID, approvalID string, status execution.AttemptStatus, step plan.StepSpec, reply string) error {
	if store == nil || approvalID == "" {
		return nil
	}
	attempts, err := store.List(sessionID)
	if err != nil {
		return err
	}
	for i := len(attempts) - 1; i >= 0; i-- {
		if attempts[i].ApprovalID != approvalID || attempts[i].Status != execution.AttemptBlocked {
			continue
		}
		attempts[i].Status = status
		attempts[i].Step = step
		if attempts[i].Metadata == nil {
			attempts[i].Metadata = map[string]any{}
		}
		attempts[i].Metadata["approval_reply"] = reply
		if attempts[i].FinishedAt == 0 {
			attempts[i].FinishedAt = time.Now().UnixMilli()
		}
		return store.Update(attempts[i])
	}
	return nil
}
