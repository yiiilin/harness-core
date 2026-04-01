package runtime

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/capability"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/persistence"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type fanoutRound struct {
	AggregateID    string
	AnchorStepID   string
	AllowSingle    bool
	MaxConcurrency int
	Steps          []plan.StepSpec
}

type fanoutPreparedStep struct {
	Original         plan.StepSpec
	Step             plan.StepSpec
	Decision         permission.Decision
	Attempt          execution.Attempt
	ConcurrencyGroup string
	ConcurrencyLimit int
}

type fanoutStepOutcome struct {
	Step               plan.StepSpec
	Execution          ExecutionResult
	Attempt            execution.Attempt
	ActionRecord       *execution.ActionRecord
	VerificationRecord *execution.VerificationRecord
	Artifacts          []execution.Artifact
	RuntimeHandles     []execution.RuntimeHandle
	CapabilitySnapshot *capability.Snapshot
	Events             []audit.Event
	Verified           bool
	PolicyDenied       bool
	ActionFailed       bool
	VerifyFailed       bool
	DurationMS         int64
}

type fanoutRoundOutput struct {
	Session     session.State
	Executions  []StepRunOutput
	UpdatedPlan *plan.Spec
	UpdatedTask *task.Record
}

func fanoutRoundForSelection(spec plan.Spec, selected plan.StepSpec, budgets LoopBudgets) (fanoutRound, bool) {
	aggregateID, scope, ok := execution.AggregateRefFromMetadata(selected.Metadata)
	if !ok || aggregateID == "" || scope != execution.AggregateScopeTargetFanout {
		return fanoutRound{}, false
	}
	steps := make([]plan.StepSpec, 0)
	for _, step := range spec.Steps {
		id, otherScope, ok := execution.AggregateRefFromMetadata(step.Metadata)
		if !ok || id != aggregateID || otherScope != execution.AggregateScopeTargetFanout {
			continue
		}
		if !fanoutStepRunnable(step, budgets) {
			continue
		}
		steps = append(steps, step)
	}
	if len(steps) <= 1 {
		return fanoutRound{}, false
	}
	maxConcurrency := fanoutRoundMaxConcurrency(selected, len(steps))
	if maxConcurrency <= 1 {
		return fanoutRound{}, false
	}
	return fanoutRound{
		AggregateID:    aggregateID,
		AnchorStepID:   selected.StepID,
		AllowSingle:    false,
		MaxConcurrency: maxConcurrency,
		Steps:          steps,
	}, true
}

func fanoutStepRunnable(step plan.StepSpec, budgets LoopBudgets) bool {
	switch step.Status {
	case "", plan.StepPending:
		return true
	case plan.StepFailed:
		if step.Attempt >= allowedAttempts(step, budgets) {
			return false
		}
		return !backoffActive(step, systemClock{}.NowMilli())
	default:
		return false
	}
}

func (s *Service) tryRunFanoutRound(ctx context.Context, sessionID, leaseID string, state session.State, latest plan.Spec, selected plan.StepSpec) (fanoutRoundOutput, bool, error) {
	round, ok := fanoutRoundForSelection(latest, selected, s.LoopBudgets)
	if !ok {
		return fanoutRoundOutput{}, false, nil
	}
	out, ok, err := s.runFanoutRound(ctx, sessionID, leaseID, state, latest, round)
	if err != nil {
		return fanoutRoundOutput{}, false, err
	}
	if !ok {
		return fanoutRoundOutput{}, false, nil
	}
	return out, true, nil
}

func (s *Service) runFanoutRound(ctx context.Context, sessionID, leaseID string, state session.State, latest plan.Spec, round fanoutRound) (fanoutRoundOutput, bool, error) {
	if state.Phase == session.PhaseComplete || state.Phase == session.PhaseFailed || state.Phase == session.PhaseAborted {
		return fanoutRoundOutput{}, false, ErrSessionTerminal
	}
	if state.ExecutionState == session.ExecutionBlocked {
		return fanoutRoundOutput{}, false, ErrSessionBlocked
	}
	if state.PendingApprovalID != "" {
		return fanoutRoundOutput{}, false, ErrSessionAwaitingApproval
	}

	now := s.nowMilli()
	if err := ensureRuntimeBudget(state, s.LoopBudgets, now); err != nil {
		return fanoutRoundOutput{}, false, err
	}

	preparationState, _, _ := s.advanceStateToExecuteForFanout(state, round.AnchorStepID)
	prepared, ok, err := s.prepareFanoutRound(ctx, sessionID, preparationState, round)
	if err != nil {
		return fanoutRoundOutput{}, false, err
	}
	if !ok {
		return fanoutRoundOutput{}, false, nil
	}
	anchorStepID := fanoutEffectiveAnchorStepID(round, prepared)
	workingState, initialTransitions, initialEvents := s.advanceStateToExecuteForFanout(state, anchorStepID)

	if _, err := s.markSessionInFlight(ctx, sessionID, leaseID, anchorStepID); err != nil {
		return fanoutRoundOutput{}, false, err
	}
	currentState, err := s.GetSession(sessionID)
	if err != nil {
		return fanoutRoundOutput{}, false, err
	}
	workingState = rebaseFanoutWorkingState(workingState, currentState)

	outcomes := s.executeFanoutPreparedSteps(ctx, workingState, prepared, round.MaxConcurrency)
	finalState, updatedPlan, updatedTask, allEvents, outputs, err := s.persistFanoutRound(ctx, sessionID, leaseID, latest, workingState, round, prepared, outcomes, initialTransitions, initialEvents)
	if err != nil {
		return fanoutRoundOutput{}, false, err
	}

	for i := range outcomes {
		s.Metrics.Record("step.run", map[string]any{
			"success":       outcomes[i].Verified,
			"policy_denied": outcomes[i].PolicyDenied,
			"verify_failed": outcomes[i].VerifyFailed,
			"action_failed": outcomes[i].ActionFailed,
			"duration_ms":   outcomes[i].DurationMS,
		})
		s.exportStepMetricSample(ctx, finalState, outcomes[i].Step, outcomes[i].Attempt, outcomes[i].ActionRecord, outcomes[i].VerificationRecord, outcomes[i].Verified, outcomes[i].PolicyDenied, outcomes[i].VerifyFailed, outcomes[i].ActionFailed, outcomes[i].DurationMS)
		s.exportTraceSpans(ctx, finalState, outcomes[i].Step, outcomes[i].Attempt, outcomes[i].ActionRecord, outcomes[i].VerificationRecord)
	}

	_ = allEvents
	return fanoutRoundOutput{
		Session:     finalState,
		Executions:  outputs,
		UpdatedPlan: updatedPlan,
		UpdatedTask: updatedTask,
	}, true, nil
}

func (s *Service) advanceStateToExecuteForFanout(state session.State, stepID string) (session.State, []TransitionDecision, []audit.Event) {
	working := state
	transitions := []TransitionDecision{}
	events := []audit.Event{}
	appendEvent := func(next TransitionDecision) {
		events = append(events, audit.Event{
			EventID:   "evt_" + uuid.NewString(),
			Type:      audit.EventStateChanged,
			SessionID: state.SessionID,
			TaskID:    state.TaskID,
			StepID:    stepID,
			Payload: map[string]any{
				"from":   next.From,
				"to":     next.To,
				"reason": next.Reason,
			},
			CreatedAt: s.nowMilli(),
		})
	}
	for working.Phase != session.PhaseExecute && !isTerminalPhase(working.Phase) {
		next := DecideNextTransition(working, stepID, permission.Decision{Action: permission.Allow, Reason: "state advancement"}, false)
		transitions = append(transitions, next)
		appendEvent(next)
		working = ApplyTransition(working, next)
	}
	return working, transitions, events
}

func (s *Service) prepareFanoutRound(ctx context.Context, sessionID string, workingState session.State, round fanoutRound) ([]fanoutPreparedStep, bool, error) {
	prepared := make([]fanoutPreparedStep, 0, len(round.Steps))
	for _, original := range round.Steps {
		step := annotateStepFromSession(workingState, original)
		step = cloneStepForExecution(step)
		if _, hasProgram := execution.ProgramFromStep(step); hasProgram {
			return nil, false, ErrProgramStepNotCompiled
		}
		if err := ensureStepRetryBudget(step, s.LoopBudgets); err != nil {
			return nil, false, err
		}
		if backoffActive(step, s.nowMilli()) {
			continue
		}
		resolved, err := s.resolveProgramBindings(ctx, sessionID, step)
		if err != nil {
			return nil, false, err
		}
		stepState := setSessionPlanRef(workingState, step)
		decision, err := s.EvaluatePolicy(ctx, stepState, resolved)
		if err != nil {
			return nil, false, err
		}
		if decision.Action == permission.Ask {
			reusableDecision, _ := s.findReusableApprovalDecision(ctx, stepState, resolved, decision)
			if reusableDecision == nil {
				if round.AllowSingle {
					continue
				}
				return nil, false, nil
			}
			decision = *reusableDecision
		}
		attempt := execution.Attempt{
			AttemptID: "att_" + uuid.NewString(),
			SessionID: sessionID,
			TaskID:    workingState.TaskID,
			StepID:    resolved.StepID,
			TraceID:   "trc_" + uuid.NewString(),
			Step:      resolved,
			StartedAt: s.nowMilli(),
		}
		attempt.CycleID = ensureExecutionCycleID(&resolved, attempt.CycleID)
		applyExecutionFactMetadata(&attempt.Metadata, resolved.Metadata)
		prepared = append(prepared, fanoutPreparedStep{
			Original:         original,
			Step:             resolved,
			Decision:         decision,
			Attempt:          attempt,
			ConcurrencyGroup: fanoutPreparedStepConcurrencyGroup(resolved),
			ConcurrencyLimit: fanoutPreparedStepConcurrencyLimit(resolved),
		})
	}
	minPrepared := 2
	if round.AllowSingle {
		minPrepared = 1
	}
	if len(prepared) < minPrepared {
		return nil, false, nil
	}
	return prepared, true, nil
}

func (s *Service) executeFanoutPreparedSteps(ctx context.Context, workingState session.State, prepared []fanoutPreparedStep, maxConcurrency int) []fanoutStepOutcome {
	outcomes := make([]fanoutStepOutcome, len(prepared))
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxConcurrency)
	groupSems := make(map[string]chan struct{}, len(prepared))
	for i := range prepared {
		group := prepared[i].ConcurrencyGroup
		limit := prepared[i].ConcurrencyLimit
		if group == "" || limit <= 0 {
			continue
		}
		if _, ok := groupSems[group]; !ok {
			groupSems[group] = make(chan struct{}, limit)
		}
	}

	for i := range prepared {
		preparedStep := prepared[i]
		groupSem := groupSems[preparedStep.ConcurrencyGroup]
		wg.Add(1)
		go func(index int, item fanoutPreparedStep, groupSem chan struct{}) {
			defer wg.Done()
			if groupSem != nil {
				groupSem <- struct{}{}
				defer func() { <-groupSem }()
			}
			sem <- struct{}{}
			defer func() { <-sem }()
			outcomes[index] = s.executeFanoutPreparedStep(ctx, workingState, item)
		}(i, preparedStep, groupSem)
	}

	wg.Wait()
	return outcomes
}

func fanoutPreparedStepConcurrencyGroup(step plan.StepSpec) string {
	if aggregateID, scope, ok := execution.AggregateRefFromMetadata(step.Metadata); ok && scope == execution.AggregateScopeTargetFanout && aggregateID != "" {
		return aggregateID
	}
	return step.StepID
}

func fanoutEffectiveAnchorStepID(round fanoutRound, prepared []fanoutPreparedStep) string {
	if round.AnchorStepID != "" {
		for _, item := range prepared {
			if item.Original.StepID == round.AnchorStepID {
				return round.AnchorStepID
			}
		}
	}
	if len(prepared) == 0 {
		return round.AnchorStepID
	}
	return prepared[0].Original.StepID
}

func (s *Service) executeFanoutPreparedStep(ctx context.Context, workingState session.State, prepared fanoutPreparedStep) fanoutStepOutcome {
	now := s.nowMilli()
	step := prepared.Step
	out := fanoutStepOutcome{
		Step: step,
		Execution: ExecutionResult{
			Step:   step,
			Policy: PolicyDecision{Decision: prepared.Decision},
		},
		Attempt: prepared.Attempt,
		Events:  []audit.Event{},
	}
	appendEvent := func(eventType string, payload map[string]any, actionID string, causationID string) {
		out.Events = append(out.Events, audit.Event{
			EventID:     "evt_" + uuid.NewString(),
			Type:        eventType,
			SessionID:   prepared.Attempt.SessionID,
			TaskID:      prepared.Attempt.TaskID,
			StepID:      step.StepID,
			AttemptID:   prepared.Attempt.AttemptID,
			ActionID:    actionID,
			CycleID:     prepared.Attempt.CycleID,
			TraceID:     prepared.Attempt.TraceID,
			CausationID: causationID,
			Payload:     payload,
			CreatedAt:   s.nowMilli(),
		})
	}

	step.Attempt++
	if step.Status == "" || step.Status == plan.StepPending || step.Status == plan.StepBlocked {
		step.StartedAt = now
	}
	step.Status = plan.StepRunning
	out.Execution.Step = step
	appendEvent(audit.EventStepStarted, map[string]any{"title": step.Title}, "", prepared.Attempt.AttemptID)

	if prepared.Decision.Action == permission.Deny {
		step.Status = plan.StepFailed
		step.FinishedAt = s.nowMilli()
		out.Step = step
		out.Execution.Step = step
		out.PolicyDenied = true
		out.VerifyFailed = false
		out.ActionFailed = false
		out.DurationMS = step.FinishedAt - now
		out.Attempt.Step = step
		out.Attempt.Status = execution.AttemptFailed
		out.Attempt.FinishedAt = step.FinishedAt
		appendEvent(audit.EventPolicyDenied, map[string]any{"reason": prepared.Decision.Reason, "matched_rule": prepared.Decision.MatchedRule}, "", prepared.Attempt.AttemptID)
		return out
	}

	actionRecord := &execution.ActionRecord{
		ActionID:    "act_" + uuid.NewString(),
		AttemptID:   prepared.Attempt.AttemptID,
		SessionID:   prepared.Attempt.SessionID,
		TaskID:      prepared.Attempt.TaskID,
		StepID:      step.StepID,
		CycleID:     prepared.Attempt.CycleID,
		ToolName:    step.Action.ToolName,
		TraceID:     prepared.Attempt.TraceID,
		CausationID: prepared.Attempt.AttemptID,
		StartedAt:   s.nowMilli(),
	}
	applyExecutionFactMetadata(&actionRecord.Metadata, step.Metadata)
	resolution, actResult, actErr := s.resolveCapabilityAndInvoke(ctx, workingState, step, prepared.Attempt, actionRecord)
	if resolution != nil {
		snapshot := resolution.Snapshot
		snapshot.Scope = capability.SnapshotScopeAction
		snapshot.ViewID = capabilityViewIDFromStep(step)
		out.CapabilitySnapshot = &snapshot
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
	appendEvent(audit.EventToolCalled, map[string]any{
		"tool_name":              actionRecord.ToolName,
		"tool_version":           toolVersion,
		"capability_snapshot_id": snapshotID,
	}, actionRecord.ActionID, prepared.Attempt.AttemptID)

	actResult = inlineActionResultWithRaw(actResult, s.LoopBudgets.MaxToolOutputChars)
	rawResult := rawPreferredActionResult(actResult)
	actionRecord.Result = actResult
	actionRecord.FinishedAt = s.nowMilli()
	if actErr != nil {
		actionRecord.Status = execution.ActionFailed
		appendEvent(audit.EventToolFailed, map[string]any{"tool_name": actionRecord.ToolName, "error": actErr.Error()}, actionRecord.ActionID, actionRecord.ActionID)
	} else if actResult.OK {
		actionRecord.Status = execution.ActionCompleted
		appendEvent(audit.EventToolCompleted, map[string]any{"tool_name": actionRecord.ToolName}, actionRecord.ActionID, actionRecord.ActionID)
	} else {
		actionRecord.Status = execution.ActionFailed
		appendEvent(audit.EventToolFailed, map[string]any{"tool_name": actionRecord.ToolName, "error": actionErrorMessage(actResult)}, actionRecord.ActionID, actionRecord.ActionID)
	}

	artifacts := []execution.Artifact{}
	if len(actResult.Data) > 0 || len(actResult.Meta) > 0 || actResult.Error != nil {
		artifacts = append(artifacts, execution.Artifact{
			ArtifactID: "art_" + uuid.NewString(),
			SessionID:  prepared.Attempt.SessionID,
			TaskID:     prepared.Attempt.TaskID,
			StepID:     step.StepID,
			AttemptID:  prepared.Attempt.AttemptID,
			ActionID:   actionRecord.ActionID,
			CycleID:    prepared.Attempt.CycleID,
			TraceID:    prepared.Attempt.TraceID,
			Name:       "action.result",
			Kind:       "action_result",
			Payload:    actionResultPayloadForArtifact(actResult),
			Metadata:   executionFactMetadata(step.Metadata),
			CreatedAt:  s.nowMilli(),
		})
	}
	runtimeHandles := extractRuntimeHandles(rawResult, prepared.Attempt, actionRecord, s.nowMilli())
	applyExecutionFactMetadataToHandles(runtimeHandles, step.Metadata)

	out.Step = step
	out.Execution.Step = step
	out.Execution.Action = actResult
	out.ActionRecord = actionRecord
	out.Artifacts = artifacts
	out.RuntimeHandles = runtimeHandles
	out.ActionFailed = actErr != nil || !actResult.OK
	return out
}

func (s *Service) persistFanoutRound(ctx context.Context, sessionID, leaseID string, latest plan.Spec, workingState session.State, round fanoutRound, prepared []fanoutPreparedStep, outcomes []fanoutStepOutcome, initialTransitions []TransitionDecision, initialEvents []audit.Event) (session.State, *plan.Spec, *task.Record, []audit.Event, []StepRunOutput, error) {
	verifyState := workingState
	verifyState.Phase = session.PhaseVerify

	aggregateFinalIndex := map[string]int{}
	aggregateFinalOrder := make([]string, 0)
	for i := range outcomes {
		if outcomes[i].ActionRecord == nil {
			outcomes[i] = finalizeFanoutDenyOutcome(outcomes[i], s.nowMilli())
			continue
		}
		if programVerifyScopeFromStep(outcomes[i].Step) == execution.VerificationScopeAggregate {
			aggregateID, _, _ := execution.AggregateRefFromMetadata(outcomes[i].Step.Metadata)
			if aggregateID != "" {
				if _, ok := aggregateFinalIndex[aggregateID]; !ok {
					aggregateFinalOrder = append(aggregateFinalOrder, aggregateID)
				}
				aggregateFinalIndex[aggregateID] = i
			}
			outcomes[i] = s.finalizeFanoutAggregatePendingOutcome(outcomes[i], s.nowMilli())
			continue
		}
		outcomes[i] = s.finalizeFanoutVerifiedOutcome(ctx, verifyState, outcomes[i], rawPreferredActionResult(outcomes[i].Execution.Action), s.nowMilli())
	}

	if len(aggregateFinalOrder) > 0 {
		provisional := replacePlanSteps(latest, fanoutOutcomeSteps(outcomes))
		for _, aggregateID := range aggregateFinalOrder {
			index := aggregateFinalIndex[aggregateID]
			resolveAt := s.nowMilli()
			outcomes[index] = s.finalizeFanoutAggregateOutcome(ctx, verifyState, provisional, outcomes[index], resolveAt)
			if outcomes[index].Verified {
				outcomes = finalizeSuccessfulAggregateSiblingOutcomes(outcomes, aggregateID, outcomes[index], resolveAt)
			}
			provisional = replacePlanSteps(provisional, fanoutOutcomeSteps(outcomes))
		}
	}

	retryFailures := fanoutRetryFailureCount(outcomes)
	finalPlanSpec := replacePlanSteps(latest, fanoutOutcomeSteps(outcomes))
	outcome := planExecutionOutcomeForSpec(finalPlanSpec, s.LoopBudgets)
	transitionState := verifyState
	transitionState.RetryCount += retryFailures
	transitionIndex := fanoutTransitionOutcomeIndex(latest, outcomes, s.LoopBudgets)
	transitionStep := outcomes[transitionIndex].Step
	next := transitionForPlanOutcome(transitionState, transitionStep, outcome)
	finalState := ApplyTransition(transitionState, next)
	finalState.ExecutionState = session.ExecutionIdle
	finalState.InFlightStepID = ""
	finalEvent := audit.Event{
		EventID:     "evt_" + uuid.NewString(),
		Type:        audit.EventStateChanged,
		SessionID:   sessionID,
		TaskID:      finalState.TaskID,
		StepID:      transitionStep.StepID,
		AttemptID:   outcomes[transitionIndex].Attempt.AttemptID,
		CycleID:     outcomes[transitionIndex].Attempt.CycleID,
		TraceID:     outcomes[transitionIndex].Attempt.TraceID,
		CausationID: outcomes[transitionIndex].Attempt.AttemptID,
		Payload: map[string]any{
			"from":   next.From,
			"to":     next.To,
			"reason": next.Reason,
		},
		CreatedAt: s.nowMilli(),
	}
	outcomes[transitionIndex].Events = append(outcomes[transitionIndex].Events, finalEvent)

	var updatedPlan *plan.Spec
	var updatedTask *task.Record
	if s.Runner != nil {
		if err := s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
			repoSet := s.repositoriesWithFallback(repos)
			pl, err := updateLatestPlanStepsInStoreWithBudgets(repoSet.Plans, sessionID, fanoutOutcomeSteps(outcomes), s.LoopBudgets)
			if err != nil {
				return err
			}
			updatedPlan = pl
			taskRec, err := updateTaskForTerminalInStore(repoSet.Tasks, finalState)
			if err != nil {
				return err
			}
			updatedTask = taskRec
			updatedState, err := persistSessionUpdate(repoSet.Sessions, finalState, leaseID)
			if err != nil {
				return err
			}
			finalState = updatedState
			for i := range outcomes {
				if err := persistExecutionFactsInRepos(repoSet, outcomes[i].Attempt, false, outcomes[i].ActionRecord, outcomes[i].VerificationRecord, outcomes[i].Artifacts); err != nil {
					return err
				}
				if err := persistCapabilitySnapshotInRepos(repoSet, outcomes[i].CapabilitySnapshot); err != nil {
					return err
				}
				if err := persistRuntimeHandlesInRepos(repoSet, outcomes[i].RuntimeHandles); err != nil {
					return err
				}
			}
			return s.emitEventsWithSink(ctx, s.eventSinkForRepos(repos), fanoutAllEvents(initialEvents, outcomes))
		}); err != nil {
			return session.State{}, nil, nil, nil, nil, err
		}
	} else {
		pl, err := updateLatestPlanStepsInStoreWithBudgets(s.Plans, sessionID, fanoutOutcomeSteps(outcomes), s.LoopBudgets)
		if err != nil {
			return session.State{}, nil, nil, nil, nil, err
		}
		updatedPlan = pl
		taskRec, err := updateTaskForTerminalInStore(s.Tasks, finalState)
		if err != nil {
			return session.State{}, nil, nil, nil, nil, err
		}
		updatedTask = taskRec
		updatedState, err := persistSessionUpdate(s.Sessions, finalState, leaseID)
		if err != nil {
			return session.State{}, nil, nil, nil, nil, err
		}
		finalState = updatedState
		for i := range outcomes {
			if err := s.persistExecutionFacts(outcomes[i].Attempt, false, outcomes[i].ActionRecord, outcomes[i].VerificationRecord, outcomes[i].Artifacts); err != nil {
				return session.State{}, nil, nil, nil, nil, err
			}
			if err := s.persistCapabilitySnapshot(outcomes[i].CapabilitySnapshot); err != nil {
				return session.State{}, nil, nil, nil, nil, err
			}
			if err := s.persistRuntimeHandles(outcomes[i].RuntimeHandles); err != nil {
				return session.State{}, nil, nil, nil, nil, err
			}
		}
		s.emitEventsBestEffort(ctx, fanoutAllEvents(initialEvents, outcomes))
	}

	outputs := fanoutStepRunOutputs(outcomes, finalState, updatedPlan, updatedTask, initialTransitions, next, transitionIndex)
	return finalState, updatedPlan, updatedTask, fanoutAllEvents(initialEvents, outcomes), outputs, nil
}

func finalizeFanoutDenyOutcome(out fanoutStepOutcome, now int64) fanoutStepOutcome {
	out.Step.FinishedAt = now
	out.Attempt.Step = out.Step
	out.Attempt.Status = execution.AttemptFailed
	out.Attempt.FinishedAt = now
	out.Execution.Step = out.Step
	out.VerificationRecord = nil
	out.Verified = false
	out.DurationMS = now - out.Attempt.StartedAt
	return out
}

func (s *Service) finalizeFanoutVerifiedOutcome(ctx context.Context, verifyState session.State, out fanoutStepOutcome, verifyInput action.Result, now int64) fanoutStepOutcome {
	verifyScope := programVerifyScopeFromStep(out.Step)
	verificationRecord := &execution.VerificationRecord{
		VerificationID: "ver_" + uuid.NewString(),
		AttemptID:      out.Attempt.AttemptID,
		SessionID:      out.Attempt.SessionID,
		TaskID:         out.Attempt.TaskID,
		StepID:         out.Step.StepID,
		ActionID:       out.ActionRecord.ActionID,
		CycleID:        out.Attempt.CycleID,
		TraceID:        out.Attempt.TraceID,
		CausationID:    out.ActionRecord.ActionID,
		Spec:           out.Step.Verify,
		StartedAt:      s.nowMilli(),
	}
	applyExecutionFactMetadata(&verificationRecord.Metadata, out.Step.Metadata)
	if verificationRecord.Metadata == nil {
		verificationRecord.Metadata = map[string]any{}
	}
	verificationRecord.Metadata[verificationScopeMetadataKey] = string(verifyScope)

	verifyResult, verifyErr := s.EvaluateVerify(ctx, out.Step.Verify, verifyInput, verifyState)
	verificationRecord.Result = verifyResult
	verificationRecord.FinishedAt = now
	verifyEvent := audit.Event{
		EventID:        "evt_" + uuid.NewString(),
		Type:           audit.EventVerifyCompleted,
		SessionID:      out.Attempt.SessionID,
		TaskID:         out.Attempt.TaskID,
		StepID:         out.Step.StepID,
		AttemptID:      out.Attempt.AttemptID,
		ActionID:       out.ActionRecord.ActionID,
		VerificationID: verificationRecord.VerificationID,
		CycleID:        out.Attempt.CycleID,
		TraceID:        out.Attempt.TraceID,
		CausationID:    out.ActionRecord.ActionID,
		Payload: map[string]any{
			"success": verifyResult.Success,
			"reason":  verifyResult.Reason,
		},
		CreatedAt: s.nowMilli(),
	}
	out.Events = append(out.Events, verifyEvent)

	verified := verifyErr == nil && verifyResult.Success
	if out.ActionFailed {
		verified = false
		if verificationRecord.Result.Success {
			verificationRecord.Result.Success = false
		}
		if verificationRecord.Result.Reason == "" {
			verificationRecord.Result.Reason = actionErrorMessage(out.Execution.Action)
		}
		verifyResult = verificationRecord.Result
		verifyEvent.Payload["success"] = false
		verifyEvent.Payload["reason"] = verifyResult.Reason
	}
	if verified {
		verificationRecord.Status = execution.VerificationCompleted
		out.Step.Status = plan.StepCompleted
	} else {
		verificationRecord.Status = execution.VerificationFailed
		out.Step.Status = plan.StepFailed
	}
	stepNext := nextTransitionAfterVerification(verifyState, out.Step, out.Execution.Policy.Decision, verified, s.LoopBudgets)
	applyStepRetryBackoff(&out.Step, stepNext, now)
	out.Step.FinishedAt = now
	out.Execution.Step = out.Step
	out.Execution.Verify = verifyResult
	out.Attempt.Step = out.Step
	out.Attempt.FinishedAt = now
	if verified {
		out.Attempt.Status = execution.AttemptCompleted
	} else {
		out.Attempt.Status = execution.AttemptFailed
	}
	out.VerificationRecord = verificationRecord
	out.Verified = verified
	out.VerifyFailed = !verified
	out.DurationMS = now - out.Attempt.StartedAt
	return out
}

func (s *Service) finalizeFanoutAggregatePendingOutcome(out fanoutStepOutcome, now int64) fanoutStepOutcome {
	rawSuccess := !out.ActionFailed
	verifyResult := verify.Result{Success: rawSuccess, Reason: "aggregate pending"}
	if !rawSuccess {
		verifyResult.Success = false
		verifyResult.Reason = actionErrorMessage(out.Execution.Action)
	}
	verificationRecord := &execution.VerificationRecord{
		VerificationID: "ver_" + uuid.NewString(),
		AttemptID:      out.Attempt.AttemptID,
		SessionID:      out.Attempt.SessionID,
		TaskID:         out.Attempt.TaskID,
		StepID:         out.Step.StepID,
		ActionID:       out.ActionRecord.ActionID,
		CycleID:        out.Attempt.CycleID,
		TraceID:        out.Attempt.TraceID,
		CausationID:    out.ActionRecord.ActionID,
		Spec:           out.Step.Verify,
		Result:         verifyResult,
		StartedAt:      s.nowMilli(),
		FinishedAt:     now,
	}
	applyExecutionFactMetadata(&verificationRecord.Metadata, out.Step.Metadata)
	if verificationRecord.Metadata == nil {
		verificationRecord.Metadata = map[string]any{}
	}
	verificationRecord.Metadata[verificationScopeMetadataKey] = string(execution.VerificationScopeAggregate)
	verifyEvent := audit.Event{
		EventID:        "evt_" + uuid.NewString(),
		Type:           audit.EventVerifyCompleted,
		SessionID:      out.Attempt.SessionID,
		TaskID:         out.Attempt.TaskID,
		StepID:         out.Step.StepID,
		AttemptID:      out.Attempt.AttemptID,
		ActionID:       out.ActionRecord.ActionID,
		VerificationID: verificationRecord.VerificationID,
		CycleID:        out.Attempt.CycleID,
		TraceID:        out.Attempt.TraceID,
		CausationID:    out.ActionRecord.ActionID,
		Payload: map[string]any{
			"success": verifyResult.Success,
			"reason":  verifyResult.Reason,
		},
		CreatedAt: s.nowMilli(),
	}
	out.Events = append(out.Events, verifyEvent)

	if verifyResult.Success {
		verificationRecord.Status = execution.VerificationCompleted
		out.Step.Status = plan.StepCompleted
	} else {
		verificationRecord.Status = execution.VerificationFailed
		out.Step.Status = plan.StepFailed
	}
	targetReason := ""
	if !rawSuccess {
		targetReason = verifyResult.Reason
	}
	out.Step.Metadata = execution.ApplyAggregateTargetOutcomeMetadata(out.Step.Metadata, out.Step.Status, targetReason)
	stepNext := nextTransitionAfterVerification(session.State{Phase: session.PhaseVerify}, out.Step, out.Execution.Policy.Decision, verifyResult.Success, s.LoopBudgets)
	applyStepRetryBackoff(&out.Step, stepNext, now)
	out.Step.FinishedAt = now
	out.Execution.Step = out.Step
	out.Execution.Verify = verifyResult
	out.Attempt.Step = out.Step
	out.Attempt.FinishedAt = now
	if verifyResult.Success {
		out.Attempt.Status = execution.AttemptCompleted
		out.Verified = true
	} else {
		out.Attempt.Status = execution.AttemptFailed
		out.Verified = false
		out.VerifyFailed = true
	}
	out.VerificationRecord = verificationRecord
	out.DurationMS = now - out.Attempt.StartedAt
	return out
}

func (s *Service) finalizeFanoutAggregateOutcome(ctx context.Context, verifyState session.State, provisional plan.Spec, out fanoutStepOutcome, now int64) fanoutStepOutcome {
	aggregateID, _, ok := execution.AggregateRefFromMetadata(out.Step.Metadata)
	if !ok || aggregateID == "" {
		return out
	}
	spec, ok := programAggregateVerifySpecFromStep(out.Step)
	if !ok || spec == nil {
		return out
	}
	if !aggregateVerificationReady(provisional, aggregateID, s.LoopBudgets) {
		return out
	}
	var aggregate execution.AggregateResult
	for _, item := range execution.AggregateResultsFromPlan(provisional) {
		if item.AggregateID == aggregateID {
			aggregate = item
			break
		}
	}
	if aggregate.AggregateID == "" {
		return out
	}
	out.Step.Verify = *spec
	out.VerificationRecord.Spec = *spec
	verifyInput := actionResultFromAggregate(aggregate)
	verifyResult, verifyErr := s.EvaluateVerify(ctx, *spec, verifyInput, verifyState)
	verified := verifyErr == nil && verifyResult.Success
	out.VerificationRecord.Result = verifyResult
	if verified {
		out.VerificationRecord.Status = execution.VerificationCompleted
		out.Step.Status = plan.StepCompleted
	} else {
		out.VerificationRecord.Status = execution.VerificationFailed
		out.Step.Status = plan.StepFailed
	}
	stepNext := nextTransitionAfterVerification(verifyState, out.Step, out.Execution.Policy.Decision, verified, s.LoopBudgets)
	applyStepRetryBackoff(&out.Step, stepNext, now)
	out.Step.FinishedAt = now
	out.Execution.Step = out.Step
	out.Execution.Verify = verifyResult
	out.Attempt.Step = out.Step
	out.Attempt.FinishedAt = now
	if verified {
		out.Attempt.Status = execution.AttemptCompleted
		out.Verified = true
		out.VerifyFailed = false
	} else {
		out.Attempt.Status = execution.AttemptFailed
		out.Verified = false
		out.VerifyFailed = true
	}
	if len(out.Events) > 0 {
		last := &out.Events[len(out.Events)-1]
		last.Payload["success"] = verifyResult.Success
		last.Payload["reason"] = verifyResult.Reason
	}
	out.DurationMS = now - out.Attempt.StartedAt
	return out
}

func finalizeSuccessfulAggregateSiblingOutcomes(outcomes []fanoutStepOutcome, aggregateID string, resolved fanoutStepOutcome, now int64) []fanoutStepOutcome {
	for i := range outcomes {
		if !fanoutAggregateOutcomeMatches(outcomes[i], aggregateID) {
			continue
		}
		outcomes[i] = finalizeSuccessfulAggregateSiblingOutcome(outcomes[i], resolved, now)
	}
	return outcomes
}

func fanoutAggregateOutcomeMatches(out fanoutStepOutcome, aggregateID string) bool {
	if programVerifyScopeFromStep(out.Step) != execution.VerificationScopeAggregate {
		return false
	}
	id, _, ok := execution.AggregateRefFromMetadata(out.Step.Metadata)
	return ok && id == aggregateID
}

func finalizeSuccessfulAggregateSiblingOutcome(out fanoutStepOutcome, resolved fanoutStepOutcome, now int64) fanoutStepOutcome {
	out.Step.Verify = resolved.Step.Verify
	out.Step.Status = plan.StepCompleted
	out.Step.Reason = ""
	out.Step.FinishedAt = now
	out.Execution.Step = out.Step
	out.Execution.Verify = resolved.Execution.Verify
	if out.ActionFailed || out.VerifyFailed || out.Attempt.Status == execution.AttemptFailed {
		out.Attempt.Step = out.Step
		out.Attempt.Status = execution.AttemptCompleted
		out.Attempt.FinishedAt = now
		if out.VerificationRecord != nil {
			out.VerificationRecord.Spec = resolved.Step.Verify
			out.VerificationRecord.Status = execution.VerificationCompleted
			out.VerificationRecord.Result = resolved.Execution.Verify
			out.VerificationRecord.FinishedAt = now
		}
	}
	out.Verified = true
	out.VerifyFailed = false
	return out
}

func fanoutOutcomeSteps(outcomes []fanoutStepOutcome) []plan.StepSpec {
	steps := make([]plan.StepSpec, 0, len(outcomes))
	for _, outcome := range outcomes {
		steps = append(steps, outcome.Step)
	}
	return steps
}

func replacePlanSteps(spec plan.Spec, steps []plan.StepSpec) plan.Spec {
	out := spec
	if len(out.Steps) == 0 || len(steps) == 0 {
		return out
	}
	index := make(map[string]plan.StepSpec, len(steps))
	for _, step := range steps {
		index[step.StepID] = step
	}
	for i := range out.Steps {
		if replacement, ok := index[out.Steps[i].StepID]; ok {
			out.Steps[i] = replacement
		}
	}
	return out
}

func updateLatestPlanStepsInStoreWithBudgets(store plan.Store, sessionID string, steps []plan.StepSpec, budgets LoopBudgets) (*plan.Spec, error) {
	if len(steps) == 0 {
		return nil, nil
	}
	target, ok, err := planForStepInStore(store, sessionID, steps[0])
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	changed := false
	index := make(map[string]plan.StepSpec, len(steps))
	for _, step := range steps {
		index[step.StepID] = annotateStepIdentity(step, target.PlanID, target.Revision)
	}
	for i := range target.Steps {
		if replacement, ok := index[target.Steps[i].StepID]; ok {
			target.Steps[i] = replacement
			changed = true
		}
	}
	if !changed {
		annotated := annotatePlanIdentity(target)
		return &annotated, nil
	}
	target.Status = planStatusForSpec(target, budgets)
	if err := store.Update(target); err != nil {
		return nil, err
	}
	annotated := annotatePlanIdentity(target)
	return &annotated, nil
}

func fanoutAllEvents(initial []audit.Event, outcomes []fanoutStepOutcome) []audit.Event {
	total := len(initial)
	for _, outcome := range outcomes {
		total += len(outcome.Events)
	}
	events := make([]audit.Event, 0, total)
	events = append(events, initial...)
	for _, outcome := range outcomes {
		events = append(events, outcome.Events...)
	}
	return events
}

func fanoutRetryFailureCount(outcomes []fanoutStepOutcome) int {
	count := 0
	for _, outcome := range outcomes {
		if outcome.ActionRecord == nil {
			continue
		}
		if outcome.Step.Status == plan.StepFailed {
			count++
		}
	}
	return count
}

func fanoutTransitionOutcomeIndex(latest plan.Spec, outcomes []fanoutStepOutcome, budgets LoopBudgets) int {
	if len(outcomes) == 0 {
		return 0
	}
	finalPlan := replacePlanSteps(latest, fanoutOutcomeSteps(outcomes))
	finalOutcome := planExecutionOutcomeForSpec(finalPlan, budgets)
	if finalOutcome.Fail && finalOutcome.Reason == "fan-out aggregate failed" {
		exhausted := exhaustedFailedAggregateIDs(finalPlan, budgets)
		for i := len(outcomes) - 1; i >= 0; i-- {
			aggregateID, _, ok := execution.AggregateRefFromMetadata(outcomes[i].Step.Metadata)
			if !ok {
				continue
			}
			if _, ok := exhausted[aggregateID]; ok {
				return i
			}
		}
	}
	provisional := latest
	for i := range outcomes {
		provisional = replacePlanSteps(provisional, []plan.StepSpec{outcomes[i].Step})
		if planExecutionOutcomeForSpec(provisional, budgets).Fail {
			return i
		}
	}
	return len(outcomes) - 1
}

func exhaustedFailedAggregateIDs(spec plan.Spec, budgets LoopBudgets) map[string]struct{} {
	aggregateIDs := map[string]struct{}{}
	for _, aggregate := range execution.AggregateResultsFromPlan(spec) {
		if aggregate.Status != execution.AggregateStatusFailed {
			continue
		}
		if !aggregateExhaustedAsFailed(spec, aggregate.AggregateID, budgets) {
			continue
		}
		aggregateIDs[aggregate.AggregateID] = struct{}{}
	}
	return aggregateIDs
}

func fanoutStepRunOutputs(outcomes []fanoutStepOutcome, finalState session.State, updatedPlan *plan.Spec, updatedTask *task.Record, initialTransitions []TransitionDecision, finalTransition TransitionDecision, finalTransitionIndex int) []StepRunOutput {
	outputs := make([]StepRunOutput, 0, len(outcomes))
	for i := range outcomes {
		transitions := []TransitionDecision{}
		if i == 0 {
			transitions = append(transitions, initialTransitions...)
		}
		if i == finalTransitionIndex {
			transitions = append(transitions, finalTransition)
		}
		stepOut := StepRunOutput{
			Session:     finalState,
			Execution:   outcomes[i].Execution,
			Transitions: transitions,
			Events:      outcomes[i].Events,
		}
		if i == finalTransitionIndex {
			stepOut.UpdatedPlan = updatedPlan
			stepOut.UpdatedTask = updatedTask
		}
		outputs = append(outputs, stepOut)
	}
	return outputs
}

func rebaseFanoutWorkingState(working, current session.State) session.State {
	working.Version = current.Version
	working.CreatedAt = current.CreatedAt
	working.UpdatedAt = current.UpdatedAt
	working.RuntimeStartedAt = current.RuntimeStartedAt
	working.LeaseID = current.LeaseID
	working.LeaseClaimedAt = current.LeaseClaimedAt
	working.LeaseExpiresAt = current.LeaseExpiresAt
	working.LastHeartbeatAt = current.LastHeartbeatAt
	return working
}
