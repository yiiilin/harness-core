package runtime

import (
	"context"

	"github.com/google/uuid"
	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/persistence"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

type AbortRequest struct {
	Code     string         `json:"code,omitempty"`
	Reason   string         `json:"reason,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type AbortOutput struct {
	Session     session.State `json:"session"`
	UpdatedTask *task.Record  `json:"updated_task,omitempty"`
	Events      []audit.Event `json:"events,omitempty"`
}

func (s *Service) AbortSession(ctx context.Context, sessionID string, request AbortRequest) (AbortOutput, error) {
	current, err := s.GetSession(sessionID)
	if err != nil {
		return AbortOutput{}, err
	}
	if current.Phase == session.PhaseAborted {
		return AbortOutput{Session: current}, nil
	}
	if isTerminalPhase(current.Phase) {
		return AbortOutput{}, ErrSessionTerminal
	}

	reason := request.Reason
	if reason == "" {
		reason = "abort requested"
	}
	stepID := abortStateStepID(current)
	currentPlanID, _, _ := planRefFromSession(current)
	aborted := ApplyTransition(current, TransitionDecision{
		From:   current.Phase,
		To:     TransitionAborted,
		StepID: stepID,
		Reason: reason,
	})
	aborted.ExecutionState = session.ExecutionIdle
	aborted.InFlightStepID = ""
	aborted.PendingApprovalID = ""
	aborted = setCurrentBlockedRuntime(aborted, "")
	aborted.LeaseID = ""
	aborted.LeaseClaimedAt = 0
	aborted.LeaseExpiresAt = 0
	aborted.Version++

	payload := map[string]any{
		"code":   request.Code,
		"reason": reason,
	}
	if len(request.Metadata) > 0 {
		payload["metadata"] = cloneAnyMap(request.Metadata)
	}
	now := s.nowMilli()
	events := []audit.Event{
		{
			EventID:   "evt_" + uuid.NewString(),
			Type:      audit.EventStateChanged,
			SessionID: current.SessionID,
			TaskID:    current.TaskID,
			StepID:    stepID,
			Payload:   map[string]any{"from": current.Phase, "to": TransitionAborted, "reason": reason},
			CreatedAt: now,
		},
		{
			EventID:   "evt_" + uuid.NewString(),
			Type:      audit.EventSessionAborted,
			SessionID: current.SessionID,
			TaskID:    current.TaskID,
			StepID:    stepID,
			Payload:   payload,
			CreatedAt: now,
		},
	}

	var updatedTask *task.Record
	persist := func(sessStore session.Store, taskStore task.Store, planStore plan.Store, approvalStore approval.Store, blockedRuntimeStore execution.BlockedRuntimeStore, attemptStore execution.AttemptStore, handleStore execution.RuntimeHandleStore) error {
		if current.PendingApprovalID != "" {
			approvalEvents, err := abortPendingApprovalInStore(approvalStore, planStore, attemptStore, current.SessionID, current.PendingApprovalID, now)
			if err != nil {
				return err
			}
			events = append(events, approvalEvents...)
		} else if blockedRuntimeID := currentBlockedRuntimeID(current); blockedRuntimeID != "" {
			blockedRuntimeEvents, err := abortCurrentBlockedRuntimeInStore(blockedRuntimeStore, current.SessionID, blockedRuntimeID, reason, now)
			if err != nil {
				return err
			}
			events = append(events, blockedRuntimeEvents...)
		} else {
			if err := abortActiveStepInLatestPlanStore(planStore, current.SessionID, currentPlanID, stepID, now); err != nil {
				return err
			}
		}
		taskRec, err := updateTaskForTerminalInStore(taskStore, aborted)
		if err != nil {
			return err
		}
		updatedTask = taskRec
		if updatedTask != nil {
			events = append(events, audit.Event{
				EventID:   "evt_" + uuid.NewString(),
				Type:      audit.EventTaskAborted,
				SessionID: aborted.SessionID,
				TaskID:    updatedTask.TaskID,
				StepID:    aborted.CurrentStepID,
				Payload: map[string]any{
					"task_id": updatedTask.TaskID,
					"status":  updatedTask.Status,
				},
				CreatedAt: now,
			})
		}
		handles, err := reconcileActiveRuntimeHandlesInStore(handleStore, current.SessionID, "session aborted", now)
		if err != nil {
			return err
		}
		events = append(events, runtimeHandleAuditEvents(now, audit.EventRuntimeHandleInvalidated, handles)...)
		if err := sessStore.Update(aborted); err != nil {
			return err
		}
		return nil
	}

	if s.Runner != nil {
		if err := s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
			repoSet := s.repositoriesWithFallback(repos)
			if err := persist(repoSet.Sessions, repoSet.Tasks, repoSet.Plans, repoSet.Approvals, repoSet.BlockedRuntimes, repoSet.Attempts, repoSet.RuntimeHandles); err != nil {
				return err
			}
			return s.emitEventsWithSink(ctx, s.eventSinkForRepos(repos), events)
		}); err != nil {
			return AbortOutput{}, err
		}
		s.exportAbortObservability(ctx, aborted, request, now, s.nowMilli())
		return AbortOutput{Session: aborted, UpdatedTask: updatedTask, Events: events}, nil
	}

	var (
		originalApproval    approval.Record
		hadOriginalApproval bool
		originalBlocked     execution.BlockedRuntimeRecord
		hadOriginalBlocked  bool
		originalAttempt     execution.Attempt
		hadOriginalAttempt  bool
		originalPlanStep    plan.StepSpec
		hadOriginalPlanStep bool
		originalTask        task.Record
		hadOriginalTask     bool
		originalHandles     []execution.RuntimeHandle
	)
	if current.PendingApprovalID != "" {
		if s.Approvals == nil {
			return AbortOutput{}, approval.ErrApprovalNotFound
		}
		originalApproval, err = s.Approvals.Get(current.PendingApprovalID)
		if err != nil {
			return AbortOutput{}, err
		}
		originalApproval = cloneApprovalRecord(originalApproval)
		hadOriginalApproval = true
		originalAttempt, hadOriginalAttempt, err = findLatestBlockedAttemptInStore(s.Attempts, current.SessionID, current.PendingApprovalID)
		if err != nil {
			return AbortOutput{}, err
		}
		if hadOriginalAttempt {
			originalAttempt = cloneAttemptRecord(originalAttempt)
		}
		originalPlanStep = cloneStepSpec(originalApproval.Step)
		hadOriginalPlanStep = originalPlanStep.StepID != ""
	} else if blockedRuntimeID := currentBlockedRuntimeID(current); blockedRuntimeID != "" {
		originalBlocked, err = blockedRuntimeRecordOrErr(s.BlockedRuntimes, blockedRuntimeID)
		if err != nil {
			return AbortOutput{}, err
		}
		originalBlocked = cloneBlockedRuntimeRecord(originalBlocked)
		hadOriginalBlocked = true
	} else {
		originalPlanStep, hadOriginalPlanStep, err = latestPlanStepByID(s.Plans, current.SessionID, currentPlanID, stepID)
		if err != nil {
			return AbortOutput{}, err
		}
	}
	if current.TaskID != "" && s.Tasks != nil {
		originalTask, err = s.Tasks.Get(current.TaskID)
		if err != nil {
			return AbortOutput{}, err
		}
		hadOriginalTask = true
	}
	originalHandles, err = snapshotActiveRuntimeHandles(s.RuntimeHandles, current.SessionID)
	if err != nil {
		return AbortOutput{}, err
	}

	approvalChanged := false
	blockedChanged := false
	attemptChanged := false
	planChanged := false
	taskChanged := false
	handlesChanged := false
	if current.PendingApprovalID != "" {
		approvalEvents, err := abortPendingApprovalInStore(s.Approvals, s.Plans, s.Attempts, current.SessionID, current.PendingApprovalID, now)
		if err != nil {
			return AbortOutput{}, err
		}
		approvalChanged = hadOriginalApproval
		attemptChanged = hadOriginalAttempt
		planChanged = hadOriginalPlanStep
		events = append(events, approvalEvents...)
	} else if blockedRuntimeID := currentBlockedRuntimeID(current); blockedRuntimeID != "" {
		blockedRuntimeEvents, err := abortCurrentBlockedRuntimeInStore(s.BlockedRuntimes, current.SessionID, blockedRuntimeID, reason, now)
		if err != nil {
			return AbortOutput{}, err
		}
		blockedChanged = hadOriginalBlocked
		events = append(events, blockedRuntimeEvents...)
	} else {
		if err := abortActiveStepInLatestPlanStore(s.Plans, current.SessionID, currentPlanID, stepID, now); err != nil {
			return AbortOutput{}, err
		}
		planChanged = hadOriginalPlanStep
	}

	taskRec, err := updateTaskForTerminalInStore(s.Tasks, aborted)
	if err != nil {
		rollbackErr := rollbackNoRunnerAbortState(s, current.SessionID, originalApproval, hadOriginalApproval && approvalChanged, originalBlocked, hadOriginalBlocked && blockedChanged, originalAttempt, hadOriginalAttempt && attemptChanged, originalPlanStep, planChanged, originalTask, hadOriginalTask && taskChanged, originalHandles, handlesChanged)
		return AbortOutput{}, joinRollbackError(err, rollbackErr)
	}
	updatedTask = taskRec
	if updatedTask != nil {
		taskChanged = hadOriginalTask
		events = append(events, audit.Event{
			EventID:   "evt_" + uuid.NewString(),
			Type:      audit.EventTaskAborted,
			SessionID: aborted.SessionID,
			TaskID:    updatedTask.TaskID,
			StepID:    aborted.CurrentStepID,
			Payload: map[string]any{
				"task_id": updatedTask.TaskID,
				"status":  updatedTask.Status,
			},
			CreatedAt: now,
		})
	}

	handles, err := reconcileActiveRuntimeHandlesInStore(s.RuntimeHandles, current.SessionID, "session aborted", now)
	if err != nil {
		rollbackErr := rollbackNoRunnerAbortState(s, current.SessionID, originalApproval, hadOriginalApproval && approvalChanged, originalBlocked, hadOriginalBlocked && blockedChanged, originalAttempt, hadOriginalAttempt && attemptChanged, originalPlanStep, planChanged, originalTask, hadOriginalTask && taskChanged, originalHandles, handlesChanged)
		return AbortOutput{}, joinRollbackError(err, rollbackErr)
	}
	if len(handles) > 0 {
		handlesChanged = len(originalHandles) > 0
		events = append(events, runtimeHandleAuditEvents(now, audit.EventRuntimeHandleInvalidated, handles)...)
	}

	if err := s.Sessions.Update(aborted); err != nil {
		rollbackErr := rollbackNoRunnerAbortState(s, current.SessionID, originalApproval, hadOriginalApproval && approvalChanged, originalBlocked, hadOriginalBlocked && blockedChanged, originalAttempt, hadOriginalAttempt && attemptChanged, originalPlanStep, planChanged, originalTask, hadOriginalTask && taskChanged, originalHandles, handlesChanged)
		return AbortOutput{}, joinRollbackError(err, rollbackErr)
	}
	s.emitEventsBestEffortWithSink(ctx, s.EventSink, events)
	s.exportAbortObservability(ctx, aborted, request, now, s.nowMilli())
	return AbortOutput{Session: aborted, UpdatedTask: updatedTask, Events: events}, nil
}

func rollbackNoRunnerAbortState(s *Service, sessionID string, originalApproval approval.Record, restoreApproval bool, originalBlocked execution.BlockedRuntimeRecord, restoreBlocked bool, originalAttempt execution.Attempt, restoreAttempt bool, originalPlanStep plan.StepSpec, restorePlan bool, originalTask task.Record, restoreTask bool, originalHandles []execution.RuntimeHandle, restoreHandles bool) error {
	rollbackErr := error(nil)
	if restoreHandles {
		rollbackErr = restoreRuntimeHandles(s.RuntimeHandles, originalHandles)
	}
	if restoreTask {
		rollbackErr = joinRollbackError(rollbackErr, restoreTaskRecord(s.Tasks, originalTask))
	}
	if restorePlan {
		rollbackErr = joinRollbackError(rollbackErr, restorePlanStepState(s.Plans, sessionID, originalPlanStep))
	}
	if restoreAttempt {
		rollbackErr = joinRollbackError(rollbackErr, restoreAttemptRecord(s.Attempts, originalAttempt))
	}
	if restoreBlocked {
		rollbackErr = joinRollbackError(rollbackErr, restoreBlockedRuntimeRecord(s.BlockedRuntimes, originalBlocked))
	}
	if restoreApproval {
		rollbackErr = joinRollbackError(rollbackErr, restoreApprovalRecord(s.Approvals, originalApproval))
	}
	return rollbackErr
}

func abortCurrentBlockedRuntimeInStore(store execution.BlockedRuntimeStore, sessionID, blockedRuntimeID, reason string, now int64) ([]audit.Event, error) {
	rec, err := blockedRuntimeRecordOrErr(store, blockedRuntimeID)
	if err != nil {
		return nil, err
	}
	if rec.SessionID != "" && rec.SessionID != sessionID {
		return nil, execution.ErrBlockedRuntimeNotFound
	}
	if rec.Status == execution.BlockedRuntimeAborted {
		return nil, nil
	}
	rec.Status = execution.BlockedRuntimeAborted
	rec.UpdatedAt = now
	rec.ResolvedAt = now
	rec.Metadata = mergeBlockedRuntimeTerminalMetadata(rec.Metadata, reason, nil, execution.BlockedRuntimeAborted)
	if err := store.Update(rec); err != nil {
		return nil, err
	}
	return []audit.Event{
		blockedRuntimeAuditEvent(now, audit.EventBlockedRuntimeAborted, rec, rec.TaskID, map[string]any{
			"status": string(rec.Status),
			"reason": reason,
		}),
	}, nil
}

func latestPlanStepByID(store plan.Store, sessionID, planID, stepID string) (plan.StepSpec, bool, error) {
	if store == nil || sessionID == "" || stepID == "" {
		return plan.StepSpec{}, false, nil
	}
	target, ok, err := planForStepInStore(store, sessionID, plan.StepSpec{PlanID: planID, StepID: stepID})
	if err != nil || !ok {
		return plan.StepSpec{}, false, err
	}
	for _, step := range target.Steps {
		if step.StepID == stepID {
			return cloneStepSpec(step), true, nil
		}
	}
	return plan.StepSpec{}, false, nil
}

func snapshotActiveRuntimeHandles(store execution.RuntimeHandleStore, sessionID string) ([]execution.RuntimeHandle, error) {
	if store == nil || sessionID == "" {
		return nil, nil
	}
	handles, err := store.List(sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]execution.RuntimeHandle, 0)
	for _, handle := range handles {
		if !isRuntimeHandleActive(handle) {
			continue
		}
		out = append(out, handle)
	}
	return out, nil
}

func abortPendingApprovalInStore(approvalStore approval.Store, planStore plan.Store, attemptStore execution.AttemptStore, sessionID, approvalID string, now int64) ([]audit.Event, error) {
	if approvalID == "" {
		return nil, nil
	}
	if approvalStore == nil {
		return nil, approval.ErrApprovalNotFound
	}

	rec, err := approvalStore.Get(approvalID)
	if err != nil {
		return nil, err
	}
	step := rec.Step
	step.Status = plan.StepFailed
	step.Reason = "session aborted"
	if step.FinishedAt == 0 {
		step.FinishedAt = now
	}

	emitRejectedEvent := false
	if rec.Status == approval.StatusPending {
		rec.Status = approval.StatusRejected
		rec.Reply = approval.ReplyReject
		rec.RespondedAt = now
		emitRejectedEvent = true
	} else if rec.Status == approval.StatusApproved {
		rec.Status = approval.StatusConsumed
		if rec.ConsumedAt == 0 {
			rec.ConsumedAt = now
		}
	}
	if rec.Metadata == nil {
		rec.Metadata = map[string]any{}
	}
	rec.Metadata["terminal_reason"] = "session_aborted"
	rec.Step = step
	rec.Version++
	if err := approvalStore.Update(rec); err != nil {
		return nil, err
	}
	if planStore != nil {
		if _, err := updateLatestPlanStepInStore(planStore, sessionID, step); err != nil {
			return nil, err
		}
	}
	if err := finalizeBlockedAttemptForAbortInStore(attemptStore, sessionID, approvalID, step, string(rec.Reply), now); err != nil {
		return nil, err
	}
	if !emitRejectedEvent {
		return nil, nil
	}
	return []audit.Event{{
		EventID:     "evt_" + uuid.NewString(),
		Type:        audit.EventApprovalRejected,
		SessionID:   rec.SessionID,
		TaskID:      rec.TaskID,
		ApprovalID:  rec.ApprovalID,
		StepID:      rec.StepID,
		CycleID:     executionCycleIDFromStep(step),
		TraceID:     rec.ApprovalID,
		CausationID: rec.ApprovalID,
		Payload: map[string]any{
			"approval_id": approvalID,
			"tool_name":   rec.ToolName,
			"reason":      "session aborted",
		},
		CreatedAt: now,
	}}, nil
}

func finalizeBlockedAttemptForAbortInStore(store execution.AttemptStore, sessionID, approvalID string, step plan.StepSpec, reply string, now int64) error {
	attempt, ok, err := findLatestBlockedAttemptInStore(store, sessionID, approvalID)
	if err != nil || !ok {
		return err
	}
	attempt.Status = execution.AttemptFailed
	attempt.Step = step
	if attempt.Metadata == nil {
		attempt.Metadata = map[string]any{}
	}
	attempt.Metadata["terminal_reason"] = "session_aborted"
	if reply != "" {
		attempt.Metadata["approval_reply"] = reply
	}
	if attempt.FinishedAt == 0 {
		attempt.FinishedAt = now
	}
	return store.Update(attempt)
}

func abortActiveStepInLatestPlanStore(store plan.Store, sessionID, planID, stepID string, now int64) error {
	if store == nil || sessionID == "" || stepID == "" {
		return nil
	}
	latest, ok, err := planForStepInStore(store, sessionID, plan.StepSpec{PlanID: planID, StepID: stepID})
	if err != nil || !ok {
		return err
	}
	changed := false
	for i := range latest.Steps {
		if latest.Steps[i].StepID != stepID {
			continue
		}
		latest.Steps[i].Status = plan.StepFailed
		latest.Steps[i].Reason = "session aborted"
		if latest.Steps[i].FinishedAt == 0 {
			latest.Steps[i].FinishedAt = now
		}
		changed = true
		break
	}
	if !changed {
		return nil
	}
	return store.Update(latest)
}

func abortStateStepID(st session.State) string {
	if st.CurrentStepID != "" {
		return st.CurrentStepID
	}
	return st.InFlightStepID
}
