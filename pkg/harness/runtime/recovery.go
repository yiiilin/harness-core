package runtime

import (
	"context"
	"time"

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

func (s *Service) updateRecoveryState(ctx context.Context, mutate func(session.State) session.State, sessionID string) (session.State, error) {
	var updated session.State
	update := func(store session.Store) error {
		st, err := store.Get(sessionID)
		if err != nil {
			return err
		}
		updated = mutate(st)
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
