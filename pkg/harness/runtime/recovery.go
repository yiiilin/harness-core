package runtime

import (
	"context"
	"time"

	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/persistence"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

func (s *Service) MarkSessionInFlight(ctx context.Context, sessionID, stepID string) (session.State, error) {
	now := time.Now().UnixMilli()
	return s.updateRecoveryState(ctx, func(st session.State) session.State {
		st.ExecutionState = session.ExecutionInFlight
		st.InFlightStepID = stepID
		st.LastHeartbeatAt = now
		return st
	}, sessionID)
}

func (s *Service) MarkSessionInterrupted(ctx context.Context, sessionID string) (session.State, error) {
	now := time.Now().UnixMilli()
	return s.updateRecoveryState(ctx, func(st session.State) session.State {
		st.ExecutionState = session.ExecutionInterrupted
		st.InterruptedAt = now
		st.Phase = session.PhaseRecover
		return st
	}, sessionID)
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

func (s *Service) recoverSession(ctx context.Context, sessionID string) (SessionRunOutput, error) {
	out := SessionRunOutput{}
	state, err := s.GetSession(sessionID)
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
			resumed, err := s.ResumePendingApproval(ctx, sessionID)
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
	next, err := s.runSession(ctx, sessionID)
	if err != nil {
		return SessionRunOutput{}, err
	}
	return mergeSessionRunOutputs(out, next), nil
}

func (s *Service) normalizeSessionForRecovery(ctx context.Context, sessionID string) (session.State, error) {
	current, err := s.GetSession(sessionID)
	if err != nil {
		return session.State{}, err
	}
	normalized := current
	if current.ExecutionState == session.ExecutionInFlight || current.ExecutionState == session.ExecutionInterrupted {
		normalized.ExecutionState = session.ExecutionIdle
		if !isTerminalPhase(current.Phase) {
			normalized.Phase = session.PhaseRecover
		}
		if normalized.CurrentStepID == "" && normalized.InFlightStepID != "" {
			normalized.CurrentStepID = normalized.InFlightStepID
		}
	}
	if normalized.ExecutionState == current.ExecutionState &&
		normalized.Phase == current.Phase &&
		normalized.CurrentStepID == current.CurrentStepID {
		return current, nil
	}
	return s.updateRecoveryState(ctx, func(session.State) session.State {
		return normalized
	}, sessionID)
}

func (s *Service) updateRecoveryState(ctx context.Context, mutate func(session.State) session.State, sessionID string) (session.State, error) {
	var updated session.State
	update := func(store session.Store) error {
		st, err := store.Get(sessionID)
		if err != nil {
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
