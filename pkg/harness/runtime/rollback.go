package runtime

import (
	"errors"

	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

func joinRollbackError(cause error, rollbackErr error) error {
	if rollbackErr == nil {
		return cause
	}
	if cause == nil {
		return rollbackErr
	}
	return errors.Join(cause, rollbackErr)
}

func rollbackSessionState(store session.Store, before, persisted session.State, leaseID string) error {
	if store == nil || before.SessionID == "" || persisted.SessionID == "" {
		return nil
	}
	rollback := before
	rollback.Version = persisted.Version
	_, err := persistSessionUpdate(store, rollback, leaseID)
	return err
}

func terminalizeApprovalForRollback(store approval.Store, current approval.Record, reason string, now int64) error {
	if store == nil || current.ApprovalID == "" {
		return nil
	}
	next := cloneApprovalRecord(current)
	next.Status = approval.StatusRejected
	next.Reply = approval.ReplyReject
	next.RespondedAt = now
	next.Version++
	next.Metadata = cloneAnyMap(next.Metadata)
	next.Metadata["terminal_reason"] = reason
	return store.Update(next)
}

func restoreApprovalRecord(store approval.Store, before approval.Record) error {
	if store == nil || before.ApprovalID == "" {
		return nil
	}
	current, err := store.Get(before.ApprovalID)
	if err != nil {
		return err
	}
	restore := cloneApprovalRecord(before)
	restore.Version = current.Version + 1
	return store.Update(restore)
}

func restoreAttemptRecord(store execution.AttemptStore, before execution.Attempt) error {
	if store == nil || before.AttemptID == "" {
		return nil
	}
	return store.Update(cloneAttemptRecord(before))
}

func restoreBlockedRuntimeRecord(store execution.BlockedRuntimeStore, before execution.BlockedRuntimeRecord) error {
	if store == nil || before.BlockedRuntimeID == "" {
		return nil
	}
	return store.Update(cloneBlockedRuntimeRecord(before))
}

func restoreTaskRecord(store task.Store, before task.Record) error {
	if store == nil || before.TaskID == "" {
		return nil
	}
	return store.Update(before)
}

func restorePlanStepState(store plan.Store, sessionID string, before plan.StepSpec) error {
	if store == nil || sessionID == "" || before.StepID == "" {
		return nil
	}
	_, err := updateLatestPlanStepInStore(store, sessionID, before)
	return err
}

func restoreRuntimeHandles(store execution.RuntimeHandleStore, before []execution.RuntimeHandle) error {
	if store == nil || len(before) == 0 {
		return nil
	}
	var rollbackErr error
	for _, handle := range before {
		current, err := store.Get(handle.HandleID)
		if err != nil {
			rollbackErr = joinRollbackError(rollbackErr, err)
			continue
		}
		restore := handle
		restore.Version = current.Version + 1
		if err := store.Update(restore); err != nil {
			rollbackErr = joinRollbackError(rollbackErr, err)
		}
	}
	return rollbackErr
}

func cloneApprovalRecord(in approval.Record) approval.Record {
	out := in
	out.Metadata = cloneAnyMap(in.Metadata)
	out.Step = cloneStepSpec(in.Step)
	return out
}

func cloneAttemptRecord(in execution.Attempt) execution.Attempt {
	out := in
	out.Metadata = cloneAnyMap(in.Metadata)
	out.Step = cloneStepSpec(in.Step)
	return out
}

func cloneStepSpec(in plan.StepSpec) plan.StepSpec {
	out := in
	if len(in.Metadata) > 0 {
		out.Metadata = cloneAnyMap(in.Metadata)
	}
	return out
}
