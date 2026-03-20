package runtime

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
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

	now := time.Now().UnixMilli()
	rec.Reply = response.Reply
	rec.Metadata = response.Metadata
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
	default:
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

func (s *Service) findReusableApprovalDecision(sessionID string, step plan.StepSpec) (*permission.Decision, *approval.Record) {
	for _, rec := range s.ListApprovals(sessionID) {
		decision, ok := s.ResumePolicy.Resolve(rec, step)
		if !ok || decision.Action != "allow" || rec.Reply != approval.ReplyAlways {
			continue
		}
		copyRec := rec
		return &decision, &copyRec
	}
	return nil, nil
}
