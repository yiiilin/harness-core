package runtime

import (
	"context"
	"reflect"
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
	attemptRecord := execution.Attempt{}
	reuseBlockedAttempt := false
	if activeApproval != nil && state.PendingApprovalID != "" && state.PendingApprovalID == activeApproval.ApprovalID {
		existingAttempt, ok, err := findLatestBlockedAttemptInStore(s.Attempts, sessionID, activeApproval.ApprovalID)
		if err != nil {
			return StepRunOutput{}, err
		}
		if ok {
			attemptRecord = existingAttempt
			reuseBlockedAttempt = true
		}
	}
	if !reuseBlockedAttempt {
		attemptRecord = execution.Attempt{
			AttemptID: "att_" + uuid.NewString(),
			SessionID: sessionID,
			TaskID:    state.TaskID,
			StepID:    step.StepID,
			TraceID:   "trc_" + uuid.NewString(),
			Step:      step,
			StartedAt: now,
		}
	}
	if attemptRecord.AttemptID == "" {
		attemptRecord.AttemptID = "att_" + uuid.NewString()
	}
	if attemptRecord.TraceID == "" {
		attemptRecord.TraceID = "trc_" + uuid.NewString()
	}
	if attemptRecord.StartedAt == 0 {
		attemptRecord.StartedAt = now
	}
	attemptRecord.SessionID = sessionID
	attemptRecord.TaskID = state.TaskID
	attemptRecord.StepID = step.StepID
	var actionRecord *execution.ActionRecord
	var verificationRecord *execution.VerificationRecord
	artifactRecords := []execution.Artifact{}
	runtimeHandles := []execution.RuntimeHandle{}
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
		attemptRecord.FinishedAt = 0
		if s.Runner != nil {
			if err := s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
				repoSet := s.repositoriesWithFallback(repos)
				if repoSet.Approvals == nil {
					return approval.ErrApprovalNotFound
				}
				rec, err := repoSet.Approvals.CreatePending(request)
				if err != nil {
					return err
				}
				pendingApproval = &rec
				attemptRecord.ApprovalID = rec.ApprovalID
				state.PendingApprovalID = rec.ApprovalID
				events[len(events)-1].Payload["approval_id"] = rec.ApprovalID
				pl, err := updateLatestPlanStepInStore(repoSet.Plans, sessionID, step)
				if err != nil {
					return err
				}
				updatedPlan = pl
				taskRec, err := updateTaskForTerminalInStore(repoSet.Tasks, state)
				if err != nil {
					return err
				}
				updatedTask = taskRec
				state.Version++
				if err := repoSet.Sessions.Update(state); err != nil {
					return err
				}
				if err := persistExecutionFactsInRepos(repoSet, attemptRecord, false, nil, nil, nil); err != nil {
					return err
				}
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
			rec, err := s.Approvals.CreatePending(request)
			if err != nil {
				return StepRunOutput{}, err
			}
			pendingApproval = &rec
			attemptRecord.ApprovalID = rec.ApprovalID
			state.PendingApprovalID = rec.ApprovalID
			events[len(events)-1].Payload["approval_id"] = rec.ApprovalID
			updatedPlan, _ = updateLatestPlanStepInStore(s.Plans, sessionID, step)
			updatedTask, _ = updateTaskForTerminalInStore(s.Tasks, state)
			state.Version++
			if err := s.Sessions.Update(state); err != nil {
				return StepRunOutput{}, err
			}
			if err := s.persistExecutionFacts(attemptRecord, false, nil, nil, nil); err != nil {
				return StepRunOutput{}, err
			}
		}
		if pendingApproval != nil {
			execResult.PendingApproval = pendingApproval
			s.exportApprovalRequestObservability(ctx, state, attemptRecord, *pendingApproval)
		}
		if s.Runner == nil {
			_ = s.emitEvents(ctx, events)
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
		s.exportStepMetricSample(ctx, state, step, attemptRecord, nil, nil, false, true, false, false, 0)
		step.Status = plan.StepFailed
		step.FinishedAt = time.Now().UnixMilli()
		attemptRecord.Status = execution.AttemptFailed
		attemptRecord.Step = step
		attemptRecord.FinishedAt = step.FinishedAt
		if activeApproval != nil {
			attemptRecord.ApprovalID = activeApproval.ApprovalID
			if attemptRecord.Metadata == nil {
				attemptRecord.Metadata = map[string]any{}
			}
			attemptRecord.Metadata["approval_reply"] = string(activeApproval.Reply)
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
				repoSet := s.repositoriesWithFallback(repos)
				pl, err := updateLatestPlanStepInStore(repoSet.Plans, sessionID, step)
				if err != nil {
					return err
				}
				updatedPlan = pl
				taskRec, err := updateTaskForTerminalInStore(repoSet.Tasks, state)
				if err != nil {
					return err
				}
				updatedTask = taskRec
				state.Version++
				if err := repoSet.Sessions.Update(state); err != nil {
					return err
				}
				if err := persistExecutionFactsInRepos(repoSet, attemptRecord, reuseBlockedAttempt, nil, nil, nil); err != nil {
					return err
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
			state.Version++
			if err := s.Sessions.Update(state); err != nil {
				return StepRunOutput{}, err
			}
			if err := s.persistExecutionFacts(attemptRecord, reuseBlockedAttempt, nil, nil, nil); err != nil {
				return StepRunOutput{}, err
			}
		}
		if s.Runner == nil {
			_ = s.emitEvents(ctx, events)
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
		snapshot.Scope = capability.SnapshotScopeAction
		snapshot.ViewID = capabilityViewIDFromStep(step)
		capabilitySnapshot = &snapshot
		if actionRecord.Metadata == nil {
			actionRecord.Metadata = map[string]any{}
		}
		actionRecord.Metadata["capability_snapshot_id"] = snapshot.SnapshotID
		if snapshot.ViewID != "" {
			actionRecord.Metadata["capability_view_id"] = snapshot.ViewID
		}
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
	runtimeHandles = extractRuntimeHandles(actResult, attemptRecord, actionRecord)

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
	verifyEventIndex := len(events)
	appendEvent(audit.EventVerifyCompleted, step.StepID, map[string]any{"success": verifyResult.Success, "reason": verifyResult.Reason}, actionRecord.ActionID, actionRecord.ActionID)
	events[verifyEventIndex].VerificationID = verificationRecord.VerificationID
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
		if attemptRecord.Metadata == nil {
			attemptRecord.Metadata = map[string]any{}
		}
		attemptRecord.Metadata["approval_reply"] = string(activeApproval.Reply)
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
			repoSet := s.repositoriesWithFallback(repos)
			pl, err := updateLatestPlanStepInStore(repoSet.Plans, sessionID, step)
			if err != nil {
				return err
			}
			updatedPlan = pl
			taskRec, err := updateTaskForTerminalInStore(repoSet.Tasks, state)
			if err != nil {
				return err
			}
			updatedTask = taskRec
			state.Version++
			if err := repoSet.Sessions.Update(state); err != nil {
				return err
			}
			if err := persistExecutionFactsInRepos(repoSet, attemptRecord, reuseBlockedAttempt, actionRecord, verificationRecord, artifactRecords); err != nil {
				return err
			}
			if err := persistCapabilitySnapshotInRepos(repoSet, capabilitySnapshot); err != nil {
				return err
			}
			if err := persistRuntimeHandlesInRepos(repoSet, runtimeHandles); err != nil {
				return err
			}
			if finalizedApproval != nil && repoSet.Approvals != nil {
				finalizedApproval.Version++
				if err := repoSet.Approvals.Update(*finalizedApproval); err != nil {
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
		state.Version++
		if err := s.Sessions.Update(state); err != nil {
			return StepRunOutput{}, err
		}
		if err := s.persistExecutionFacts(attemptRecord, reuseBlockedAttempt, actionRecord, verificationRecord, artifactRecords); err != nil {
			return StepRunOutput{}, err
		}
		if err := s.persistCapabilitySnapshot(capabilitySnapshot); err != nil {
			return StepRunOutput{}, err
		}
		if err := s.persistRuntimeHandles(runtimeHandles); err != nil {
			return StepRunOutput{}, err
		}
		if finalizedApproval != nil && s.Approvals != nil {
			finalizedApproval.Version++
			if err := s.Approvals.Update(*finalizedApproval); err != nil {
				return StepRunOutput{}, err
			}
		}
	}
	if s.Runner == nil {
		_ = s.emitEvents(ctx, events)
	}
	s.Metrics.Record("step.run", map[string]any{
		"success":       verified,
		"policy_denied": false,
		"verify_failed": !verified,
		"action_failed": !actResult.OK,
		"duration_ms":   time.Now().UnixMilli() - now,
	})
	s.exportStepMetricSample(ctx, state, step, attemptRecord, actionRecord, verificationRecord, verified, false, !verified, !actResult.OK, time.Now().UnixMilli()-now)
	s.exportTraceSpans(ctx, state, step, attemptRecord, actionRecord, verificationRecord)

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
	latest, ok, err := store.LatestBySession(sessionID)
	if err != nil {
		return nil, err
	}
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
	latest, ok, err := store.LatestBySession(sessionID)
	if err != nil || !ok {
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

func persistExecutionFactsInRepos(repos persistence.RepositorySet, attempt execution.Attempt, updateAttempt bool, actionRecord *execution.ActionRecord, verificationRecord *execution.VerificationRecord, artifacts []execution.Artifact) error {
	if repos.Attempts != nil {
		if updateAttempt {
			if err := repos.Attempts.Update(attempt); err != nil {
				return err
			}
		} else {
			if _, err := repos.Attempts.Create(attempt); err != nil {
				return err
			}
		}
	}
	if actionRecord != nil && repos.Actions != nil {
		if _, err := repos.Actions.Create(*actionRecord); err != nil {
			return err
		}
	}
	if verificationRecord != nil && repos.Verifications != nil {
		if _, err := repos.Verifications.Create(*verificationRecord); err != nil {
			return err
		}
	}
	if repos.Artifacts != nil {
		for _, artifact := range artifacts {
			if _, err := repos.Artifacts.Create(artifact); err != nil {
				return err
			}
		}
	}
	return nil
}

func persistCapabilitySnapshotInRepos(repos persistence.RepositorySet, snapshot *capability.Snapshot) error {
	if snapshot == nil || repos.CapabilitySnapshots == nil {
		return nil
	}
	_, err := repos.CapabilitySnapshots.Create(*snapshot)
	return err
}

func persistRuntimeHandlesInRepos(repos persistence.RepositorySet, handles []execution.RuntimeHandle) error {
	if repos.RuntimeHandles == nil {
		return nil
	}
	for _, handle := range handles {
		if err := upsertRuntimeHandle(repos.RuntimeHandles, handle); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) persistExecutionFacts(attempt execution.Attempt, updateAttempt bool, actionRecord *execution.ActionRecord, verificationRecord *execution.VerificationRecord, artifacts []execution.Artifact) error {
	if s.Attempts != nil {
		if updateAttempt {
			if err := s.Attempts.Update(attempt); err != nil {
				return err
			}
		} else {
			if _, err := s.Attempts.Create(attempt); err != nil {
				return err
			}
		}
	}
	if actionRecord != nil && s.Actions != nil {
		if _, err := s.Actions.Create(*actionRecord); err != nil {
			return err
		}
	}
	if verificationRecord != nil && s.Verifications != nil {
		if _, err := s.Verifications.Create(*verificationRecord); err != nil {
			return err
		}
	}
	if s.Artifacts != nil {
		for _, artifact := range artifacts {
			if _, err := s.Artifacts.Create(artifact); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) persistCapabilitySnapshot(snapshot *capability.Snapshot) error {
	if snapshot == nil || s.CapabilitySnapshots == nil {
		return nil
	}
	_, err := s.CapabilitySnapshots.Create(*snapshot)
	return err
}

func (s *Service) persistRuntimeHandles(handles []execution.RuntimeHandle) error {
	if s.RuntimeHandles == nil {
		return nil
	}
	for _, handle := range handles {
		if err := upsertRuntimeHandle(s.RuntimeHandles, handle); err != nil {
			return err
		}
	}
	return nil
}

func upsertRuntimeHandle(store execution.RuntimeHandleStore, handle execution.RuntimeHandle) error {
	if store == nil {
		return nil
	}
	if handle.Status == "" {
		handle.Status = execution.RuntimeHandleActive
	}
	if _, err := store.Create(handle); err != nil {
		if _, getErr := store.Get(handle.HandleID); getErr == nil {
			return store.Update(handle)
		}
		return err
	}
	return nil
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

func (s *Service) repositoriesWithFallback(repos persistence.RepositorySet) persistence.RepositorySet {
	if repos.Sessions == nil {
		repos.Sessions = s.Sessions
	}
	if repos.Tasks == nil {
		repos.Tasks = s.Tasks
	}
	if repos.Plans == nil {
		repos.Plans = s.Plans
	}
	if repos.Audits == nil {
		repos.Audits = s.Audit
	}
	if repos.Attempts == nil {
		repos.Attempts = s.Attempts
	}
	if repos.Actions == nil {
		repos.Actions = s.Actions
	}
	if repos.Verifications == nil {
		repos.Verifications = s.Verifications
	}
	if repos.Artifacts == nil {
		repos.Artifacts = s.Artifacts
	}
	if repos.RuntimeHandles == nil {
		repos.RuntimeHandles = s.RuntimeHandles
	}
	if repos.Approvals == nil {
		repos.Approvals = s.Approvals
	}
	if repos.CapabilitySnapshots == nil {
		repos.CapabilitySnapshots = s.CapabilitySnapshots
	}
	if repos.PlanningRecords == nil {
		repos.PlanningRecords = s.PlanningRecords
	}
	return repos
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

func extractRuntimeHandles(result action.Result, attempt execution.Attempt, actionRecord *execution.ActionRecord) []execution.RuntimeHandle {
	out := []execution.RuntimeHandle{}
	seen := map[string]struct{}{}
	appendHandle := func(raw any) {
		handle, ok := runtimeHandleFromValue(raw, attempt, actionRecord)
		if !ok {
			return
		}
		if _, exists := seen[handle.HandleID]; exists {
			return
		}
		seen[handle.HandleID] = struct{}{}
		out = append(out, handle)
	}
	collect := func(container map[string]any) {
		if container == nil {
			return
		}
		if raw, ok := container["runtime_handle"]; ok {
			appendHandle(raw)
		}
		if raw, ok := container["runtime_handles"]; ok {
			appendRuntimeHandleSlice(raw, appendHandle)
		}
	}
	collect(result.Data)
	collect(result.Meta)
	return out
}

func appendRuntimeHandleSlice(raw any, appendHandle func(any)) {
	switch items := raw.(type) {
	case []any:
		for _, item := range items {
			appendHandle(item)
		}
	case []map[string]any:
		for _, item := range items {
			appendHandle(item)
		}
	default:
		value := reflect.ValueOf(raw)
		if !value.IsValid() || value.Kind() != reflect.Slice {
			return
		}
		for i := 0; i < value.Len(); i++ {
			appendHandle(value.Index(i).Interface())
		}
	}
}

func runtimeHandleFromValue(raw any, attempt execution.Attempt, actionRecord *execution.ActionRecord) (execution.RuntimeHandle, bool) {
	var handle execution.RuntimeHandle
	switch item := raw.(type) {
	case execution.RuntimeHandle:
		handle = item
	case *execution.RuntimeHandle:
		if item == nil {
			return execution.RuntimeHandle{}, false
		}
		handle = *item
	default:
		mapItem, ok := raw.(map[string]any)
		if !ok {
			return execution.RuntimeHandle{}, false
		}

		now := time.Now().UnixMilli()
		handle = execution.RuntimeHandle{
			SessionID: attempt.SessionID,
			TaskID:    attempt.TaskID,
			AttemptID: attempt.AttemptID,
			TraceID:   attempt.TraceID,
			Status:    execution.RuntimeHandleActive,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if actionRecord != nil {
			if handle.Metadata == nil {
				handle.Metadata = map[string]any{}
			}
			handle.Metadata["action_id"] = actionRecord.ActionID
		}
		if v, _ := mapItem["handle_id"].(string); v != "" {
			handle.HandleID = v
		} else {
			handle.HandleID = "hdl_" + uuid.NewString()
		}
		if v, _ := mapItem["session_id"].(string); v != "" {
			handle.SessionID = v
		}
		if v, _ := mapItem["task_id"].(string); v != "" {
			handle.TaskID = v
		}
		if v, _ := mapItem["attempt_id"].(string); v != "" {
			handle.AttemptID = v
		}
		if v, _ := mapItem["trace_id"].(string); v != "" {
			handle.TraceID = v
		}
		if v, _ := mapItem["kind"].(string); v != "" {
			handle.Kind = v
		}
		if v, _ := mapItem["value"].(string); v != "" {
			handle.Value = v
		}
		if v, _ := mapItem["status"].(string); v != "" {
			handle.Status = execution.RuntimeHandleStatus(v)
		}
		if v, _ := mapItem["status_reason"].(string); v != "" {
			handle.StatusReason = v
		}
		if metadata, ok := mapItem["metadata"].(map[string]any); ok {
			handle.Metadata = mergeMaps(handle.Metadata, metadata)
		}
		if createdAt, ok := asInt64(mapItem["created_at"]); ok && createdAt > 0 {
			handle.CreatedAt = createdAt
		}
		if updatedAt, ok := asInt64(mapItem["updated_at"]); ok && updatedAt > 0 {
			handle.UpdatedAt = updatedAt
		} else {
			handle.UpdatedAt = handle.CreatedAt
		}
		if closedAt, ok := asInt64(mapItem["closed_at"]); ok && closedAt > 0 {
			handle.ClosedAt = closedAt
		}
		if invalidatedAt, ok := asInt64(mapItem["invalidated_at"]); ok && invalidatedAt > 0 {
			handle.InvalidatedAt = invalidatedAt
		}
		if handle.Kind == "" && handle.Value == "" {
			return execution.RuntimeHandle{}, false
		}
		return handle, true
	}

	now := time.Now().UnixMilli()
	if handle.HandleID == "" {
		handle.HandleID = "hdl_" + uuid.NewString()
	}
	if handle.SessionID == "" {
		handle.SessionID = attempt.SessionID
	}
	if handle.TaskID == "" {
		handle.TaskID = attempt.TaskID
	}
	if handle.AttemptID == "" {
		handle.AttemptID = attempt.AttemptID
	}
	if handle.TraceID == "" {
		handle.TraceID = attempt.TraceID
	}
	if handle.CreatedAt == 0 {
		handle.CreatedAt = now
	}
	if handle.UpdatedAt == 0 {
		handle.UpdatedAt = handle.CreatedAt
	}
	if handle.Status == "" {
		handle.Status = execution.RuntimeHandleActive
	}
	if actionRecord != nil {
		if handle.Metadata == nil {
			handle.Metadata = map[string]any{}
		}
		handle.Metadata["action_id"] = actionRecord.ActionID
	}
	if handle.Kind == "" && handle.Value == "" {
		return execution.RuntimeHandle{}, false
	}
	return handle, true
}

func mergeMaps(base map[string]any, extra map[string]any) map[string]any {
	if base == nil && len(extra) == 0 {
		return nil
	}
	out := map[string]any{}
	for key, value := range base {
		out[key] = value
	}
	for key, value := range extra {
		out[key] = value
	}
	return out
}

func asInt64(value any) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int32:
		return int64(v), true
	case int64:
		return v, true
	case float32:
		return int64(v), true
	case float64:
		return int64(v), true
	default:
		return 0, false
	}
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
