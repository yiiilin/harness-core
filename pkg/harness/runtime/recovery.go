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
)

func (s *Service) MarkSessionInFlight(ctx context.Context, sessionID, stepID string) (session.State, error) {
	return s.markSessionInFlight(ctx, sessionID, "", stepID)
}

func (s *Service) MarkClaimedSessionInFlight(ctx context.Context, sessionID, leaseID, stepID string) (session.State, error) {
	return s.markSessionInFlight(ctx, sessionID, leaseID, stepID)
}

func (s *Service) markSessionInFlight(ctx context.Context, sessionID, leaseID, stepID string) (session.State, error) {
	now := s.nowMilli()
	state, err := s.ensureSessionLease(sessionID, leaseID)
	if err != nil {
		return session.State{}, err
	}
	if err := ensureRuntimeBudget(state, s.LoopBudgets, now); err != nil {
		return session.State{}, err
	}
	stepRef, hasStepRef, err := s.stepRefForSession(ctx, state, stepID)
	if err != nil {
		return session.State{}, err
	}
	return s.updateRecoveryState(ctx, sessionID, leaseID, "in_flight", func(st session.State) session.State {
		if st.RuntimeStartedAt == 0 {
			st.RuntimeStartedAt = now
		}
		st.ExecutionState = session.ExecutionInFlight
		st.InFlightStepID = stepID
		if hasStepRef {
			st = setSessionPlanRef(st, stepRef)
		}
		st.LastHeartbeatAt = now
		return st
	})
}

func (s *Service) MarkSessionInterrupted(ctx context.Context, sessionID string) (session.State, error) {
	return s.markSessionInterrupted(ctx, sessionID, "")
}

func (s *Service) MarkClaimedSessionInterrupted(ctx context.Context, sessionID, leaseID string) (session.State, error) {
	return s.markSessionInterrupted(ctx, sessionID, leaseID)
}

func (s *Service) markSessionInterrupted(ctx context.Context, sessionID, leaseID string) (session.State, error) {
	now := s.nowMilli()
	return s.updateRecoveryState(ctx, sessionID, leaseID, "interrupted", func(st session.State) session.State {
		st.ExecutionState = session.ExecutionInterrupted
		st.InterruptedAt = now
		st.Phase = session.PhaseRecover
		return st
	})
}

func (s *Service) ListRecoverableSessions() ([]session.State, error) {
	items, err := s.listSessionRecords(context.Background())
	if err != nil {
		return nil, err
	}
	out := make([]session.State, 0)
	for _, st := range items {
		if session.IsRecoverableState(st) {
			out = append(out, st)
		}
	}
	return out, nil
}

func (s *Service) recoverSession(ctx context.Context, sessionID, leaseID string) (SessionRunOutput, error) {
	out := SessionRunOutput{}
	state, err := s.ensureSessionLease(sessionID, leaseID)
	if err != nil {
		return SessionRunOutput{}, err
	}
	out.Session = state
	if latest, ok, err := s.latestPlanForSession(ctx, sessionID); err != nil {
		return SessionRunOutput{}, err
	} else if ok {
		out.Plan = planPointer(latest)
	}
	if isTerminalPhase(state.Phase) {
		return out, nil
	}
	if state.PendingApprovalID != "" {
		rec, err := s.GetApproval(state.PendingApprovalID)
		if err != nil {
			return SessionRunOutput{}, err
		}
		switch rec.Status {
		case approval.StatusPending:
			return out, nil
		case approval.StatusApproved:
			resumed, err := s.resumePendingApprovalWithLease(ctx, sessionID, leaseID)
			if err != nil {
				return SessionRunOutput{}, err
			}
			if !isSessionApprovalRecord(rec) {
				out.Executions = append(out.Executions, resumed)
				out.Session = resumed.Session
				if resumed.UpdatedPlan != nil {
					out.Plan = resumed.UpdatedPlan
				}
				if isTerminalPhase(resumed.Session.Phase) || resumed.Session.PendingApprovalID != "" {
					return out, nil
				}
			}
		default:
			return SessionRunOutput{}, ErrApprovalNotResolved
		}
	}
	normalized, err := s.normalizeSessionForRecovery(ctx, sessionID)
	if err != nil {
		return SessionRunOutput{}, err
	}
	out.Session = normalized
	if recovered, handled, err := s.failInterruptedConcurrentRound(ctx, sessionID, leaseID, normalized, state.InFlightStepID); err != nil {
		return SessionRunOutput{}, err
	} else if handled {
		return mergeSessionRunOutputs(out, recovered), nil
	}
	s.compactSessionContextBestEffort(ctx, sessionID, CompactionTriggerRecover)
	next, err := s.runSession(ctx, sessionID, leaseID)
	if err != nil {
		return SessionRunOutput{}, err
	}
	return mergeSessionRunOutputs(out, next), nil
}

func (s *Service) ensureSessionLease(sessionID, leaseID string) (session.State, error) {
	st, err := s.GetSession(sessionID)
	if err != nil {
		return session.State{}, err
	}
	if err := requireSessionLease(st, leaseID, s.nowMilli()); err != nil {
		return session.State{}, err
	}
	return st, nil
}

func requireSessionLease(st session.State, leaseID string, now int64) error {
	if leaseID == "" {
		if st.LeaseID != "" && st.LeaseExpiresAt > now {
			return session.ErrSessionLeaseNotHeld
		}
		return nil
	}
	if st.LeaseID != leaseID || st.LeaseExpiresAt <= now {
		return session.ErrSessionLeaseNotHeld
	}
	return nil
}

func (s *Service) stepRefForSession(ctx context.Context, st session.State, stepID string) (plan.StepSpec, bool, error) {
	if stepID == "" {
		return plan.StepSpec{}, false, nil
	}
	if pinnedPlan, ok, err := s.pinnedPlanForSession(ctx, st); err != nil {
		return plan.StepSpec{}, false, err
	} else if ok {
		if step, found := findPlanStepByID(pinnedPlan, stepID); found {
			return step, true, nil
		}
	}
	latest, ok, err := s.latestPlanForSession(ctx, st.SessionID)
	if err != nil || !ok {
		return plan.StepSpec{}, false, err
	}
	step, found := findPlanStepByID(latest, stepID)
	if !found {
		return plan.StepSpec{}, false, nil
	}
	return step, true, nil
}

func (s *Service) normalizeSessionForRecovery(ctx context.Context, sessionID string) (session.State, error) {
	current, err := s.GetSession(sessionID)
	if err != nil {
		return session.State{}, err
	}
	shouldReconcileHandles := current.ExecutionState == session.ExecutionInFlight ||
		current.ExecutionState == session.ExecutionInterrupted ||
		current.Phase == session.PhaseRecover
	var updated session.State
	normalize := func(sessStore session.Store, handleStore execution.RuntimeHandleStore, sink EventSink) error {
		st, err := sessStore.Get(sessionID)
		if err != nil {
			return err
		}
		next := st
		events := make([]audit.Event, 0)
		if st.ExecutionState == session.ExecutionInFlight || st.ExecutionState == session.ExecutionInterrupted {
			next.ExecutionState = session.ExecutionIdle
			if !isTerminalPhase(st.Phase) {
				next.Phase = session.PhaseRecover
			}
			if next.CurrentStepID == "" && next.InFlightStepID != "" {
				next.CurrentStepID = next.InFlightStepID
			}
		}
		if shouldReconcileHandles {
			handles, err := reconcileActiveRuntimeHandlesInStore(handleStore, sessionID, "session recovered", s.nowMilli())
			if err != nil {
				return err
			}
			events = append(events, runtimeHandleAuditEvents(s.nowMilli(), audit.EventRuntimeHandleInvalidated, handles)...)
		}
		if next.ExecutionState == st.ExecutionState &&
			next.Phase == st.Phase &&
			next.CurrentStepID == st.CurrentStepID {
			updated = st
		} else {
			updatedState, err := persistSessionUpdate(sessStore, next, st.LeaseID)
			if err != nil {
				return err
			}
			updated = updatedState
			events = append(events, newAuditEventAt(s.nowMilli(), audit.EventRecoveryStateChanged, updated.SessionID, updated.TaskID, recoveryStepID(updated), recoveryStatePayload(st, updated, "recovered")))
		}
		if len(events) == 0 {
			return nil
		}
		if sink != nil {
			return s.emitEventsWithSink(ctx, sink, events)
		}
		s.emitEventsBestEffort(ctx, events)
		return nil
	}

	if s.Runner != nil {
		if err := s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
			repoSet := s.repositoriesWithFallback(repos)
			return normalize(repoSet.Sessions, repoSet.RuntimeHandles, s.eventSinkForRepos(repos))
		}); err != nil {
			return session.State{}, err
		}
		return updated, nil
	}

	if err := normalize(s.Sessions, s.RuntimeHandles, nil); err != nil {
		return session.State{}, err
	}
	return updated, nil
}

func (s *Service) failInterruptedConcurrentRound(ctx context.Context, sessionID, leaseID string, state session.State, inFlightStepID string) (SessionRunOutput, bool, error) {
	if inFlightStepID == "" || isTerminalPhase(state.Phase) {
		return SessionRunOutput{}, false, nil
	}
	latest, ok, err := s.latestPlanForSession(ctx, sessionID)
	if err != nil {
		return SessionRunOutput{}, false, err
	}
	if !ok {
		return SessionRunOutput{}, false, nil
	}
	selected, ok := findPlanStepByID(latest, inFlightStepID)
	if !ok {
		return SessionRunOutput{}, false, nil
	}
	round, ok := interruptedConcurrentRoundForSelection(latest, selected, s.LoopBudgets)
	if !ok {
		return SessionRunOutput{}, false, nil
	}
	workingState, _, _ := s.advanceStateToExecuteForFanout(state, round.AnchorStepID)
	prepared, ok, err := s.prepareFanoutRound(ctx, sessionID, workingState, round)
	if err != nil {
		return SessionRunOutput{}, false, err
	}
	if !ok || len(prepared) == 0 {
		return SessionRunOutput{}, false, nil
	}

	now := s.nowMilli()
	reason := "interrupted concurrent ready round"
	failedSteps := make([]plan.StepSpec, 0, len(prepared))
	for _, item := range prepared {
		step := item.Original
		step.Status = plan.StepFailed
		step.Reason = reason
		if step.Attempt < allowedAttempts(step, s.LoopBudgets) {
			step.Attempt = allowedAttempts(step, s.LoopBudgets)
		}
		if step.StartedAt == 0 {
			step.StartedAt = now
		}
		step.FinishedAt = now
		failedSteps = append(failedSteps, step)
	}

	updatedPlan := replacePlanSteps(latest, failedSteps)
	updatedPlan.Status = plan.StatusFailed
	annotatedPlan := annotatePlanIdentity(updatedPlan)

	attributedStepID := prepared[0].Original.StepID
	next := setSessionPlanRef(state, plan.StepSpec{PlanID: latest.PlanID, PlanRevision: latest.Revision})
	transition := TransitionDecision{
		From:   next.Phase,
		To:     TransitionFailed,
		StepID: attributedStepID,
		Reason: reason,
	}
	next = ApplyTransition(next, transition)
	next.ExecutionState = session.ExecutionIdle
	next.InFlightStepID = ""

	event := audit.Event{
		EventID:   "evt_" + uuid.NewString(),
		Type:      audit.EventStateChanged,
		SessionID: sessionID,
		TaskID:    next.TaskID,
		StepID:    attributedStepID,
		Payload: map[string]any{
			"from":   transition.From,
			"to":     transition.To,
			"reason": transition.Reason,
		},
		CreatedAt: now,
	}

	persist := func(repos persistence.RepositorySet) error {
		if repos.Sessions == nil {
			return session.ErrSessionNotFound
		}
		if repos.Plans != nil {
			if err := repos.Plans.Update(updatedPlan); err != nil {
				return err
			}
		}
		persisted, err := persistSessionUpdate(repos.Sessions, next, leaseID)
		if err != nil {
			return err
		}
		next = persisted
		_, err = updateTaskForTerminalInStore(repos.Tasks, next)
		return err
	}

	if s.Runner != nil {
		if err := s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
			repoSet := s.repositoriesWithFallback(repos)
			if err := persist(repoSet); err != nil {
				return err
			}
			return s.emitEventsWithSink(ctx, s.eventSinkForRepos(repos), []audit.Event{event})
		}); err != nil {
			return SessionRunOutput{}, false, err
		}
	} else {
		if err := persist(s.repositoriesWithFallback(persistence.RepositorySet{})); err != nil {
			return SessionRunOutput{}, false, err
		}
		s.emitEventsBestEffort(ctx, []audit.Event{event})
	}

	return SessionRunOutput{
		Session:    next,
		Plan:       &annotatedPlan,
		Aggregates: execution.AggregateResultsFromPlan(updatedPlan),
	}, true, nil
}

func interruptedConcurrentRoundForSelection(latest plan.Spec, selected plan.StepSpec, budgets LoopBudgets) (fanoutRound, bool) {
	if round, ok := programReadyRoundForSelection(latest, selected, budgets); ok {
		return fanoutRound{
			AnchorStepID:   round.AnchorStepID,
			AllowSingle:    true,
			MaxConcurrency: round.MaxConcurrency,
			Steps:          append([]plan.StepSpec(nil), round.Steps...),
		}, true
	}
	if round, ok := fanoutRoundForSelection(latest, selected, budgets); ok {
		round.Steps = append([]plan.StepSpec(nil), round.Steps...)
		return round, true
	}
	return fanoutRound{}, false
}

func (s *Service) updateRecoveryState(ctx context.Context, sessionID, leaseID, mutation string, mutate func(session.State) session.State) (session.State, error) {
	var updated session.State
	update := func(store session.Store, sink EventSink) error {
		st, err := store.Get(sessionID)
		if err != nil {
			return err
		}
		if err := requireSessionLease(st, leaseID, s.nowMilli()); err != nil {
			return err
		}
		updated = mutate(st)
		updatedState, err := persistSessionUpdateAt(store, updated, leaseID, s.nowMilli())
		if err != nil {
			return err
		}
		updated = updatedState
		event := newAuditEventAt(s.nowMilli(), audit.EventRecoveryStateChanged, updated.SessionID, updated.TaskID, recoveryStepID(updated), recoveryStatePayload(st, updated, mutation))
		if sink != nil {
			return s.emitEventsWithSink(ctx, sink, []audit.Event{event})
		}
		s.emitEventsBestEffort(ctx, []audit.Event{event})
		return nil
	}

	if s.Runner != nil {
		if err := s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
			store := s.Sessions
			if repos.Sessions != nil {
				store = repos.Sessions
			}
			return update(store, s.eventSinkForRepos(repos))
		}); err != nil {
			return session.State{}, err
		}
		return updated, nil
	}

	if err := update(s.Sessions, nil); err != nil {
		return session.State{}, err
	}
	return updated, nil
}

func recoveryStatePayload(before, after session.State, mutation string) map[string]any {
	return map[string]any{
		"mutation":               mutation,
		"from_phase":             string(before.Phase),
		"to_phase":               string(after.Phase),
		"from_execution_state":   string(before.ExecutionState),
		"to_execution_state":     string(after.ExecutionState),
		"from_current_step_id":   before.CurrentStepID,
		"to_current_step_id":     after.CurrentStepID,
		"from_in_flight_step_id": before.InFlightStepID,
		"to_in_flight_step_id":   after.InFlightStepID,
		"pending_approval_id":    after.PendingApprovalID,
		"runtime_started_at":     after.RuntimeStartedAt,
		"last_heartbeat_at":      after.LastHeartbeatAt,
		"interrupted_at":         after.InterruptedAt,
	}
}

func recoveryStepID(st session.State) string {
	if st.InFlightStepID != "" {
		return st.InFlightStepID
	}
	return st.CurrentStepID
}
