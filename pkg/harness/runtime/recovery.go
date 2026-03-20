package runtime

import (
	"context"
	"time"

	"github.com/yiiilin/harness-core/pkg/harness/session"
)

func (s *Service) MarkSessionInFlight(ctx context.Context, sessionID, stepID string) (session.State, error) {
	st, err := s.Sessions.Get(sessionID)
	if err != nil {
		return session.State{}, err
	}
	now := time.Now().UnixMilli()
	st.ExecutionState = session.ExecutionInFlight
	st.InFlightStepID = stepID
	st.LastHeartbeatAt = now
	if err := s.Sessions.Update(st); err != nil {
		return session.State{}, err
	}
	_ = ctx
	return st, nil
}

func (s *Service) MarkSessionInterrupted(ctx context.Context, sessionID string) (session.State, error) {
	st, err := s.Sessions.Get(sessionID)
	if err != nil {
		return session.State{}, err
	}
	now := time.Now().UnixMilli()
	st.ExecutionState = session.ExecutionInterrupted
	st.InterruptedAt = now
	st.Phase = session.PhaseRecover
	if err := s.Sessions.Update(st); err != nil {
		return session.State{}, err
	}
	_ = ctx
	return st, nil
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
