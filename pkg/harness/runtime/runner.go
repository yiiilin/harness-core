package runtime

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/yiiilin/harness-core/pkg/harness/action"
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

type StepRunOutput struct {
	Session     session.State        `json:"session"`
	Execution   ExecutionResult      `json:"execution"`
	Transitions []TransitionDecision `json:"transitions"`
	Events      []audit.Event        `json:"events"`
	UpdatedPlan *plan.Spec           `json:"updated_plan,omitempty"`
	UpdatedTask *task.Record         `json:"updated_task,omitempty"`
}

func (s *Service) runStep(ctx context.Context, sessionID string, step plan.StepSpec) (StepRunOutput, error) {
	return s.runStepWithDecision(ctx, sessionID, step, nil, nil)
}

func (s *Service) runStepWithDecision(ctx context.Context, sessionID string, step plan.StepSpec, forcedDecision *permission.Decision, activeApproval *approval.Record) (StepRunOutput, error) {
	state, err := s.GetSession(sessionID)
	if err != nil {
		return StepRunOutput{}, err
	}
	if state.Phase == session.PhaseComplete || state.Phase == session.PhaseFailed || state.Phase == session.PhaseAborted {
		return StepRunOutput{}, ErrSessionTerminal
	}
	if state.PendingApprovalID != "" && (activeApproval == nil || state.PendingApprovalID != activeApproval.ApprovalID) {
		return StepRunOutput{}, ErrSessionAwaitingApproval
	}
	if err := ensureRuntimeBudget(state, s.LoopBudgets); err != nil {
		return StepRunOutput{}, err
	}
	if err := ensureStepRetryBudget(step, s.LoopBudgets); err != nil {
		return StepRunOutput{}, err
	}

	now := time.Now().UnixMilli()
	attemptRecord := execution.Attempt{
		AttemptID: "att_" + uuid.NewString(),
		SessionID: sessionID,
		TaskID:    state.TaskID,
		StepID:    step.StepID,
		TraceID:   "trc_" + uuid.NewString(),
		Step:      step,
		StartedAt: now,
	}
	var actionRecord *execution.ActionRecord
	var verificationRecord *execution.VerificationRecord
	artifactRecords := []execution.Artifact{}
	var capabilitySnapshot *capability.Snapshot

	transitions := []TransitionDecision{}
	events := []audit.Event{}
	appendEvent := func(eventType string, stepID string, payload map[string]any, actionID string, causationID string) {
		events = append(events, audit.Event{
			EventID:     "evt_" + uuid.NewString(),
			Type:        eventType,
			SessionID:   sessionID,
			TaskID:      state.TaskID,
			StepID:      stepID,
			AttemptID:   attemptRecord.AttemptID,
			ActionID:    actionID,
			TraceID:     attemptRecord.TraceID,
			CausationID: causationID,
			Payload:     payload,
			CreatedAt:   time.Now().UnixMilli(),
		})
	}

	for state.Phase != session.PhaseExecute && state.Phase != session.PhaseComplete && state.Phase != session.PhaseFailed && state.Phase != session.PhaseAborted {
		next := DecideNextTransition(state, step.StepID, permission.Decision{Action: permission.Allow, Reason: "state advancement"}, false)
		transitions = append(transitions, next)
		appendEvent(audit.EventStateChanged, step.StepID, map[string]any{"from": state.Phase, "to": next.To, "reason": next.Reason}, "", attemptRecord.AttemptID)
		state = ApplyTransition(state, next)
	}

	var decision permission.Decision
	if forcedDecision != nil {
		decision = *forcedDecision
	} else {
		decision, err = s.EvaluatePolicy(ctx, state, step)
		if err != nil {
			return StepRunOutput{}, err
		}
		if decision.Action == permission.Ask {
			reusableDecision, reusableApproval := s.findReusableApprovalDecision(ctx, state, step, decision)
			if reusableDecision != nil {
				decision = *reusableDecision
				activeApproval = reusableApproval
			}
		}
	}
	execResult := ExecutionResult{
		Step:   step,
		Policy: PolicyDecision{Decision: decision},
	}

	if decision.Action == permission.Ask && forcedDecision == nil {
		requestScope := s.buildApprovalReuseScope(ctx, state, step, decision)
		step.Status = plan.StepBlocked
		state.ExecutionState = session.ExecutionAwaitingApproval
		state.PendingApprovalID = ""
		state.CurrentStepID = step.StepID
		request := approval.Request{
			SessionID:   sessionID,
			TaskID:      state.TaskID,
			StepID:      step.StepID,
			ToolName:    step.Action.ToolName,
			Reason:      decision.Reason,
			MatchedRule: decision.MatchedRule,
			Step:        step,
			Metadata:    approvalMetadataForRequest(requestScope),
		}
		appendEvent(audit.EventApprovalRequested, step.StepID, map[string]any{
			"tool_name":    step.Action.ToolName,
			"reason":       decision.Reason,
			"matched_rule": decision.MatchedRule,
		}, "", attemptRecord.AttemptID)
		var updatedPlan *plan.Spec
		var updatedTask *task.Record
		var pendingApproval *approval.Record
		attemptRecord.Status = execution.AttemptBlocked
		attemptRecord.Step = step
		attemptRecord.FinishedAt = time.Now().UnixMilli()
		if s.Runner != nil {
			if err := s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
				if repos.Approvals == nil {
					return approval.ErrApprovalNotFound
				}
				rec := repos.Approvals.CreatePending(request)
				pendingApproval = &rec
				attemptRecord.ApprovalID = rec.ApprovalID
				state.PendingApprovalID = rec.ApprovalID
				events[len(events)-1].Payload["approval_id"] = rec.ApprovalID
				pl, err := updateLatestPlanStepInStore(repos.Plans, sessionID, step)
				if err != nil {
					return err
				}
				updatedPlan = pl
				taskRec, err := updateTaskForTerminalInStore(repos.Tasks, state)
				if err != nil {
					return err
				}
				updatedTask = taskRec
				if err := repos.Sessions.Update(state); err != nil {
					return err
				}
				persistExecutionFactsInRepos(repos, attemptRecord, nil, nil, nil)
				if err := s.emitEventsWithSink(ctx, s.eventSinkForRepos(repos), events); err != nil {
					return err
				}
				return nil
			}); err != nil {
				return StepRunOutput{}, err
			}
		} else {
			if s.Approvals == nil {
				return StepRunOutput{}, approval.ErrApprovalNotFound
			}
			rec := s.Approvals.CreatePending(request)
			pendingApproval = &rec
			attemptRecord.ApprovalID = rec.ApprovalID
			state.PendingApprovalID = rec.ApprovalID
			events[len(events)-1].Payload["approval_id"] = rec.ApprovalID
			updatedPlan, _ = updateLatestPlanStepInStore(s.Plans, sessionID, step)
			updatedTask, _ = updateTaskForTerminalInStore(s.Tasks, state)
			if err := s.Sessions.Update(state); err != nil {
				return StepRunOutput{}, err
			}
			s.persistExecutionFacts(attemptRecord, nil, nil, nil)
		}
		if pendingApproval != nil {
			execResult.PendingApproval = pendingApproval
		}
		if s.Runner == nil {
			if err := s.emitEvents(ctx, events); err != nil {
				return StepRunOutput{}, err
			}
		}
		return StepRunOutput{
			Session:     state,
			Execution:   execResult,
			Transitions: transitions,
			Events:      events,
			UpdatedPlan: updatedPlan,
			UpdatedTask: updatedTask,
		}, nil
	}

	step.Attempt++
	if step.Status == "" || step.Status == plan.StepPending || step.Status == plan.StepBlocked {
		step.StartedAt = now
	}
	step.Status = plan.StepRunning
	execResult.Step = step
	appendEvent(audit.EventStepStarted, step.StepID, map[string]any{"title": step.Title}, "", attemptRecord.AttemptID)

	if _, err := s.MarkSessionInFlight(ctx, sessionID, step.StepID); err != nil {
		return StepRunOutput{}, err
	}
	state, _ = s.GetSession(sessionID)

	if decision.Action == permission.Deny {
		s.Metrics.Record("step.run", map[string]any{"success": false, "policy_denied": true, "verify_failed": false, "action_failed": false, "duration_ms": int64(0)})
		step.Status = plan.StepFailed
		step.FinishedAt = time.Now().UnixMilli()
		attemptRecord.Status = execution.AttemptFailed
		attemptRecord.Step = step
		attemptRecord.FinishedAt = step.FinishedAt
		if activeApproval != nil {
			attemptRecord.ApprovalID = activeApproval.ApprovalID
		}
		next := TransitionDecision{From: state.Phase, To: TransitionFailed, StepID: step.StepID, Reason: "policy denied action"}
		transitions = append(transitions, next)
		appendEvent(audit.EventPolicyDenied, step.StepID, map[string]any{"reason": decision.Reason, "matched_rule": decision.MatchedRule}, "", attemptRecord.AttemptID)
		appendEvent(audit.EventStateChanged, step.StepID, map[string]any{"from": state.Phase, "to": next.To, "reason": next.Reason}, "", attemptRecord.AttemptID)
		state = ApplyTransition(state, next)
		state.ExecutionState = session.ExecutionIdle
		state.InFlightStepID = ""
		var updatedPlan *plan.Spec
		var updatedTask *task.Record
		if s.Runner != nil {
			if err := s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
				pl, err := updateLatestPlanStepInStore(repos.Plans, sessionID, step)
				if err != nil {
					return err
				}
				updatedPlan = pl
				taskRec, err := updateTaskForTerminalInStore(repos.Tasks, state)
				if err != nil {
					return err
				}
				updatedTask = taskRec
				if err := repos.Sessions.Update(state); err != nil {
					return err
				}
				persistExecutionFactsInRepos(repos, attemptRecord, nil, nil, nil)
				if err := s.emitEventsWithSink(ctx, s.eventSinkForRepos(repos), events); err != nil {
					return err
				}
				return nil
			}); err != nil {
				return StepRunOutput{}, err
			}
		} else {
			updatedPlan, _ = updateLatestPlanStepInStore(s.Plans, sessionID, step)
			updatedTask, _ = updateTaskForTerminalInStore(s.Tasks, state)
			if err := s.Sessions.Update(state); err != nil {
				return StepRunOutput{}, err
			}
			s.persistExecutionFacts(attemptRecord, nil, nil, nil)
		}
		if s.Runner == nil {
			if err := s.emitEvents(ctx, events); err != nil {
				return StepRunOutput{}, err
			}
		}
		return StepRunOutput{Session: state, Execution: execResult, Transitions: transitions, Events: events, UpdatedPlan: updatedPlan, UpdatedTask: updatedTask}, nil
	}

	actionRecord = &execution.ActionRecord{
		ActionID:    "act_" + uuid.NewString(),
		AttemptID:   attemptRecord.AttemptID,
		SessionID:   sessionID,
		TaskID:      state.TaskID,
		StepID:      step.StepID,
		ToolName:    step.Action.ToolName,
		TraceID:     attemptRecord.TraceID,
		CausationID: attemptRecord.AttemptID,
		StartedAt:   time.Now().UnixMilli(),
	}
	resolution, actResult, actErr := s.resolveCapabilityAndInvoke(ctx, state, step)
	if resolution != nil {
		snapshot := resolution.Snapshot
		capabilitySnapshot = &snapshot
		if actionRecord.Metadata == nil {
			actionRecord.Metadata = map[string]any{}
		}
		actionRecord.Metadata["capability_snapshot_id"] = snapshot.SnapshotID
		actionRecord.Metadata["tool_version"] = resolution.Definition.Version
		actionRecord.Metadata["capability_type"] = resolution.Definition.CapabilityType
		actionRecord.Metadata["risk_level"] = string(resolution.Definition.RiskLevel)
		actionRecord.ToolName = resolution.Definition.ToolName
	}
	var toolVersion any
	var snapshotID any
	if actionRecord.Metadata != nil {
		toolVersion = actionRecord.Metadata["tool_version"]
		snapshotID = actionRecord.Metadata["capability_snapshot_id"]
	}
	appendEvent(audit.EventToolCalled, step.StepID, map[string]any{
		"tool_name":              actionRecord.ToolName,
		"tool_version":           toolVersion,
		"capability_snapshot_id": snapshotID,
	}, actionRecord.ActionID, attemptRecord.AttemptID)
	actResult = trimActionResultToBudget(actResult, s.LoopBudgets.MaxToolOutputChars)
	execResult.Action = actResult
	actionRecord.Result = actResult
	actionRecord.FinishedAt = time.Now().UnixMilli()
	if actErr != nil {
		actionRecord.Status = execution.ActionFailed
		appendEvent(audit.EventToolFailed, step.StepID, map[string]any{"tool_name": actionRecord.ToolName, "error": actErr.Error()}, actionRecord.ActionID, actionRecord.ActionID)
	} else if actResult.OK {
		actionRecord.Status = execution.ActionCompleted
		appendEvent(audit.EventToolCompleted, step.StepID, map[string]any{"tool_name": actionRecord.ToolName}, actionRecord.ActionID, actionRecord.ActionID)
	} else {
		actionRecord.Status = execution.ActionFailed
		appendEvent(audit.EventToolFailed, step.StepID, map[string]any{"tool_name": actionRecord.ToolName, "error": actionErrorMessage(actResult)}, actionRecord.ActionID, actionRecord.ActionID)
	}
	if len(actResult.Data) > 0 || len(actResult.Meta) > 0 || actResult.Error != nil {
		artifactRecords = append(artifactRecords, execution.Artifact{
			ArtifactID: "art_" + uuid.NewString(),
			SessionID:  sessionID,
			TaskID:     state.TaskID,
			StepID:     step.StepID,
			AttemptID:  attemptRecord.AttemptID,
			ActionID:   actionRecord.ActionID,
			TraceID:    attemptRecord.TraceID,
			Name:       "action.result",
			Kind:       "action_result",
			Payload: map[string]any{
				"data":  actResult.Data,
				"meta":  actResult.Meta,
				"error": actResult.Error,
			},
			CreatedAt: time.Now().UnixMilli(),
		})
	}

	state.Phase = session.PhaseVerify
	verificationRecord = &execution.VerificationRecord{
		VerificationID: "ver_" + uuid.NewString(),
		AttemptID:      attemptRecord.AttemptID,
		SessionID:      sessionID,
		TaskID:         state.TaskID,
		StepID:         step.StepID,
		ActionID:       actionRecord.ActionID,
		TraceID:        attemptRecord.TraceID,
		CausationID:    actionRecord.ActionID,
		Spec:           step.Verify,
		StartedAt:      time.Now().UnixMilli(),
	}
	verifyResult, verifyErr := s.EvaluateVerify(ctx, step.Verify, actResult, state)
	execResult.Verify = verifyResult
	verificationRecord.Result = verifyResult
	verificationRecord.FinishedAt = time.Now().UnixMilli()
	appendEvent(audit.EventVerifyCompleted, step.StepID, map[string]any{"success": verifyResult.Success, "reason": verifyResult.Reason}, actionRecord.ActionID, actionRecord.ActionID)
	verified := verifyErr == nil && verifyResult.Success
	if verified {
		verificationRecord.Status = execution.VerificationCompleted
	} else {
		verificationRecord.Status = execution.VerificationFailed
	}

	next := nextTransitionAfterVerification(state, step, decision, verified, s.LoopBudgets)
	if verified && latestPlanHasRemainingSteps(s.Plans, sessionID, step.StepID) {
		next = TransitionDecision{From: state.Phase, To: TransitionPlan, StepID: step.StepID, Reason: "step completed, continue plan"}
	}
	transitions = append(transitions, next)
	appendEvent(audit.EventStateChanged, step.StepID, map[string]any{"from": state.Phase, "to": next.To, "reason": next.Reason}, "", attemptRecord.AttemptID)
	state = ApplyTransition(state, next)
	state.ExecutionState = session.ExecutionIdle
	state.InFlightStepID = ""
	if activeApproval != nil && state.PendingApprovalID == activeApproval.ApprovalID {
		state.PendingApprovalID = ""
	}

	if verified {
		step.Status = plan.StepCompleted
	} else {
		step.Status = plan.StepFailed
		state.RetryCount++
	}
	step.FinishedAt = time.Now().UnixMilli()
	execResult.Step = step
	attemptRecord.Step = step
	attemptRecord.FinishedAt = step.FinishedAt
	if verified {
		attemptRecord.Status = execution.AttemptCompleted
	} else {
		attemptRecord.Status = execution.AttemptFailed
	}
	if activeApproval != nil {
		attemptRecord.ApprovalID = activeApproval.ApprovalID
	}

	var updatedPlan *plan.Spec
	var updatedTask *task.Record
	var finalizedApproval *approval.Record
	if activeApproval != nil {
		nextApproval := *activeApproval
		switch nextApproval.Reply {
		case approval.ReplyOnce:
			nextApproval.Status = approval.StatusConsumed
			nextApproval.ConsumedAt = time.Now().UnixMilli()
			finalizedApproval = &nextApproval
		case approval.ReplyAlways:
			nextApproval.Status = approval.StatusApproved
			finalizedApproval = &nextApproval
		}
	}
	if s.Runner != nil {
		if err := s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
			pl, err := updateLatestPlanStepInStore(repos.Plans, sessionID, step)
			if err != nil {
				return err
			}
			updatedPlan = pl
			taskRec, err := updateTaskForTerminalInStore(repos.Tasks, state)
			if err != nil {
				return err
			}
			updatedTask = taskRec
			if err := repos.Sessions.Update(state); err != nil {
				return err
			}
			persistExecutionFactsInRepos(repos, attemptRecord, actionRecord, verificationRecord, artifactRecords)
			persistCapabilitySnapshotInRepos(repos, capabilitySnapshot)
			if finalizedApproval != nil && repos.Approvals != nil {
				if err := repos.Approvals.Update(*finalizedApproval); err != nil {
					return err
				}
			}
			if err := s.emitEventsWithSink(ctx, s.eventSinkForRepos(repos), events); err != nil {
				return err
			}
			return nil
		}); err != nil {
			return StepRunOutput{}, err
		}
	} else {
		updatedPlan, _ = updateLatestPlanStepInStore(s.Plans, sessionID, step)
		updatedTask, _ = updateTaskForTerminalInStore(s.Tasks, state)
		if err := s.Sessions.Update(state); err != nil {
			return StepRunOutput{}, err
		}
		s.persistExecutionFacts(attemptRecord, actionRecord, verificationRecord, artifactRecords)
		s.persistCapabilitySnapshot(capabilitySnapshot)
		if finalizedApproval != nil && s.Approvals != nil {
			if err := s.Approvals.Update(*finalizedApproval); err != nil {
				return StepRunOutput{}, err
			}
		}
	}
	if s.Runner == nil {
		if err := s.emitEvents(ctx, events); err != nil {
			return StepRunOutput{}, err
		}
	}
	s.Metrics.Record("step.run", map[string]any{
		"success":       verified,
		"policy_denied": false,
		"verify_failed": !verified,
		"action_failed": !actResult.OK,
		"duration_ms":   time.Now().UnixMilli() - now,
	})

	return StepRunOutput{
		Session:     state,
		Execution:   execResult,
		Transitions: transitions,
		Events:      events,
		UpdatedPlan: updatedPlan,
		UpdatedTask: updatedTask,
	}, nil
}

func updateLatestPlanStepInStore(store plan.Store, sessionID string, step plan.StepSpec) (*plan.Spec, error) {
	latest, ok := store.LatestBySession(sessionID)
	if !ok {
		return nil, nil
	}
	changed := false
	for i := range latest.Steps {
		if latest.Steps[i].StepID == step.StepID {
			latest.Steps[i] = step
			changed = true
			break
		}
	}
	if !changed {
		return &latest, nil
	}
	if step.Status == plan.StepCompleted {
		allDone := true
		for _, st := range latest.Steps {
			if st.Status != plan.StepCompleted {
				allDone = false
				break
			}
		}
		if allDone {
			latest.Status = plan.StatusCompleted
		}
	}
	if step.Status == plan.StepFailed {
		latest.Status = plan.StatusActive
	}
	if err := store.Update(latest); err != nil {
		return nil, err
	}
	return &latest, nil
}

func updateTaskForTerminalInStore(store task.Store, state session.State) (*task.Record, error) {
	if state.TaskID == "" {
		return nil, nil
	}
	rec, err := store.Get(state.TaskID)
	if err != nil {
		return nil, err
	}
	switch state.Phase {
	case session.PhaseComplete:
		rec.Status = task.StatusCompleted
	case session.PhaseFailed:
		rec.Status = task.StatusFailed
	case session.PhaseAborted:
		rec.Status = task.StatusAborted
	default:
		return &rec, nil
	}
	if err := store.Update(rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

func latestPlanHasRemainingSteps(store plan.Store, sessionID, completedStepID string) bool {
	if store == nil {
		return false
	}
	latest, ok := store.LatestBySession(sessionID)
	if !ok {
		return false
	}
	for _, st := range latest.Steps {
		status := st.Status
		if st.StepID == completedStepID {
			status = plan.StepCompleted
		}
		if status != plan.StepCompleted {
			return true
		}
	}
	return false
}

func actionErrorMessage(result action.Result) string {
	if result.Error != nil && result.Error.Message != "" {
		return result.Error.Message
	}
	return "tool failed"
}

func persistExecutionFactsInRepos(repos persistence.RepositorySet, attempt execution.Attempt, actionRecord *execution.ActionRecord, verificationRecord *execution.VerificationRecord, artifacts []execution.Artifact) {
	if repos.Attempts != nil {
		repos.Attempts.Create(attempt)
	}
	if actionRecord != nil && repos.Actions != nil {
		repos.Actions.Create(*actionRecord)
	}
	if verificationRecord != nil && repos.Verifications != nil {
		repos.Verifications.Create(*verificationRecord)
	}
	if repos.Artifacts != nil {
		for _, artifact := range artifacts {
			repos.Artifacts.Create(artifact)
		}
	}
}

func persistCapabilitySnapshotInRepos(repos persistence.RepositorySet, snapshot *capability.Snapshot) {
	if snapshot == nil || repos.CapabilitySnapshots == nil {
		return
	}
	repos.CapabilitySnapshots.Create(*snapshot)
}

func (s *Service) persistExecutionFacts(attempt execution.Attempt, actionRecord *execution.ActionRecord, verificationRecord *execution.VerificationRecord, artifacts []execution.Artifact) {
	if s.Attempts != nil {
		s.Attempts.Create(attempt)
	}
	if actionRecord != nil && s.Actions != nil {
		s.Actions.Create(*actionRecord)
	}
	if verificationRecord != nil && s.Verifications != nil {
		s.Verifications.Create(*verificationRecord)
	}
	if s.Artifacts != nil {
		for _, artifact := range artifacts {
			s.Artifacts.Create(artifact)
		}
	}
}

func (s *Service) persistCapabilitySnapshot(snapshot *capability.Snapshot) {
	if snapshot == nil || s.CapabilitySnapshots == nil {
		return
	}
	s.CapabilitySnapshots.Create(*snapshot)
}

func (s *Service) emitEvents(ctx context.Context, events []audit.Event) error {
	return s.emitEventsWithSink(ctx, s.EventSink, events)
}

func (s *Service) emitEventsWithSink(ctx context.Context, sink EventSink, events []audit.Event) error {
	if sink == nil {
		return nil
	}
	for _, event := range events {
		if err := sink.Emit(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) eventSinkForRepos(repos persistence.RepositorySet) EventSink {
	if repos.Audits == nil {
		return s.EventSink
	}
	if s.EventSink == nil {
		return AuditStoreSink{Store: repos.Audits}
	}
	if aware, ok := s.EventSink.(auditStoreAwareSink); ok {
		return aware.WithAuditStore(repos.Audits)
	}
	return FanoutEventSink{Sinks: []EventSink{s.EventSink, AuditStoreSink{Store: repos.Audits}}}
}

func (s *Service) resolveCapabilityAndInvoke(ctx context.Context, state session.State, step plan.StepSpec) (*capability.Resolution, action.Result, error) {
	resolution, err := s.ResolveCapability(ctx, capability.Request{
		SessionID: state.SessionID,
		TaskID:    state.TaskID,
		StepID:    step.StepID,
		Action:    step.Action,
	})
	if err != nil {
		return nil, capabilityErrorResult(step.Action, err), err
	}
	if resolution.Handler == nil {
		return &resolution, action.Result{OK: false, Error: &action.Error{Code: "TOOL_NOT_IMPLEMENTED", Message: step.Action.ToolName}}, nil
	}
	result, invokeErr := resolution.Handler.Invoke(ctx, step.Action.Args)
	return &resolution, result, invokeErr
}

func trimActionResultToBudget(result action.Result, limit int) action.Result {
	if limit <= 0 {
		return result
	}
	result.Data = trimMapStrings(result.Data, limit)
	result.Meta = trimMapStrings(result.Meta, limit)
	if result.Error != nil && len(result.Error.Message) > limit {
		result.Error = &action.Error{Code: result.Error.Code, Message: result.Error.Message[:limit]}
	}
	return result
}

func trimMapStrings(in map[string]any, limit int) map[string]any {
	if len(in) == 0 || limit <= 0 {
		return in
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		switch v := value.(type) {
		case string:
			if len(v) > limit {
				out[key] = v[:limit]
			} else {
				out[key] = v
			}
		default:
			out[key] = value
		}
	}
	return out
}
