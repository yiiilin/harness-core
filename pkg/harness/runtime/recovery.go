package runtime

import (
	"context"
	"time"

	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/persistence"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

func (s *Service) MarkSessionInFlight(ctx context.Context, sessionID, stepID string) (session.State, error) {
	return s.markSessionInFlight(ctx, sessionID, "", stepID)
}

func (s *Service) MarkClaimedSessionInFlight(ctx context.Context, sessionID, leaseID, stepID string) (session.State, error) {
	return s.markSessionInFlight(ctx, sessionID, leaseID, stepID)
}

func (s *Service) markSessionInFlight(ctx context.Context, sessionID, leaseID, stepID string) (session.State, error) {
	now := time.Now().UnixMilli()
	return s.updateRecoveryState(ctx, sessionID, leaseID, func(st session.State) session.State {
		st.ExecutionState = session.ExecutionInFlight
		st.InFlightStepID = stepID
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
	now := time.Now().UnixMilli()
	return s.updateRecoveryState(ctx, sessionID, leaseID, func(st session.State) session.State {
		st.ExecutionState = session.ExecutionInterrupted
		st.InterruptedAt = now
		st.Phase = session.PhaseRecover
		return st
	})
}

func (s *Service) ListRecoverableSessions() ([]session.State, error) {
	items, err := s.Sessions.List()
	if err != nil {
		return nil, err
	}
	out := make([]session.State, 0)
	for _, st := range items {
		if st.ExecutionState == session.ExecutionInFlight || st.ExecutionState == session.ExecutionInterrupted {
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
	if latest, ok, err := s.latestPlanForSession(sessionID); err != nil {
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
			out.Executions = append(out.Executions, resumed)
			out.Session = resumed.Session
			if resumed.UpdatedPlan != nil {
				out.Plan = resumed.UpdatedPlan
			}
			if isTerminalPhase(resumed.Session.Phase) || resumed.Session.PendingApprovalID != "" {
				return out, nil
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
	if _, _, err := s.CompactSessionContext(ctx, sessionID, CompactionTriggerRecover); err != nil {
		return SessionRunOutput{}, err
	}
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
	if err := requireSessionLease(st, leaseID, time.Now().UnixMilli()); err != nil {
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

func (s *Service) normalizeSessionForRecovery(ctx context.Context, sessionID string) (session.State, error) {
	current, err := s.GetSession(sessionID)
	if err != nil {
		return session.State{}, err
	}
	shouldReconcileHandles := current.ExecutionState == session.ExecutionInFlight ||
		current.ExecutionState == session.ExecutionInterrupted ||
		current.Phase == session.PhaseRecover
	var updated session.State
	normalize := func(sessStore session.Store, handleStore execution.RuntimeHandleStore) error {
		st, err := sessStore.Get(sessionID)
		if err != nil {
			return err
		}
		next := st
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
			if err := reconcileActiveRuntimeHandlesInStore(handleStore, sessionID, "session recovered"); err != nil {
				return err
			}
		}
		if next.ExecutionState == st.ExecutionState &&
			next.Phase == st.Phase &&
			next.CurrentStepID == st.CurrentStepID {
			updated = st
			return nil
		}
		next.Version++
		if err := sessStore.Update(next); err != nil {
			return err
		}
		updated = next
		return nil
	}

	if s.Runner != nil {
		if err := s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
			repoSet := s.repositoriesWithFallback(repos)
			return normalize(repoSet.Sessions, repoSet.RuntimeHandles)
		}); err != nil {
			return session.State{}, err
		}
		return updated, nil
	}

	if err := normalize(s.Sessions, s.RuntimeHandles); err != nil {
		return session.State{}, err
	}
	return updated, nil
}

func (s *Service) updateRecoveryState(ctx context.Context, sessionID, leaseID string, mutate func(session.State) session.State) (session.State, error) {
	var updated session.State
	update := func(store session.Store) error {
		st, err := store.Get(sessionID)
		if err != nil {
			return err
		}
		if err := requireSessionLease(st, leaseID, time.Now().UnixMilli()); err != nil {
			return err
		}
		updated = mutate(st)
		updated.Version++
		return store.Update(updated)
	}

	if s.Runner != nil {
		if err := s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
			store := s.Sessions
			if repos.Sessions != nil {
				store = repos.Sessions
			}
			return update(store)
		}); err != nil {
			return session.State{}, err
		}
		return updated, nil
	}

	if err := update(s.Sessions); err != nil {
		return session.State{}, err
	}
	return updated, nil
}
