package runtime

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/yiiilin/harness-core/pkg/harness/persistence"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

var ErrInvalidLeaseTTL = errors.New("lease ttl must be positive")

func (s *Service) ClaimRunnableSession(ctx context.Context, leaseTTL time.Duration) (session.State, bool, error) {
	return s.claimSession(ctx, session.ClaimModeRunnable, leaseTTL)
}

func (s *Service) ClaimRecoverableSession(ctx context.Context, leaseTTL time.Duration) (session.State, bool, error) {
	return s.claimSession(ctx, session.ClaimModeRecoverable, leaseTTL)
}

func (s *Service) RenewSessionLease(ctx context.Context, sessionID, leaseID string, leaseTTL time.Duration) (session.State, error) {
	if leaseTTL <= 0 {
		return session.State{}, ErrInvalidLeaseTTL
	}
	now := time.Now().UnixMilli()
	expiresAt := now + leaseTTL.Milliseconds()

	var updated session.State
	renew := func(store session.Store) error {
		st, err := store.RenewLease(sessionID, leaseID, now, expiresAt)
		if err != nil {
			return err
		}
		updated = st
		return nil
	}

	if s.Runner != nil {
		if err := s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
			store := s.Sessions
			if repos.Sessions != nil {
				store = repos.Sessions
			}
			return renew(store)
		}); err != nil {
			return session.State{}, err
		}
		return updated, nil
	}

	if err := renew(s.Sessions); err != nil {
		return session.State{}, err
	}
	return updated, nil
}

func (s *Service) ReleaseSessionLease(ctx context.Context, sessionID, leaseID string) (session.State, error) {
	var updated session.State
	release := func(store session.Store) error {
		st, err := store.ReleaseLease(sessionID, leaseID)
		if err != nil {
			return err
		}
		updated = st
		return nil
	}

	if s.Runner != nil {
		if err := s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
			store := s.Sessions
			if repos.Sessions != nil {
				store = repos.Sessions
			}
			return release(store)
		}); err != nil {
			return session.State{}, err
		}
		return updated, nil
	}

	if err := release(s.Sessions); err != nil {
		return session.State{}, err
	}
	return updated, nil
}

func (s *Service) claimSession(ctx context.Context, mode session.ClaimMode, leaseTTL time.Duration) (session.State, bool, error) {
	if leaseTTL <= 0 {
		return session.State{}, false, ErrInvalidLeaseTTL
	}
	now := time.Now().UnixMilli()
	expiresAt := now + leaseTTL.Milliseconds()
	leaseID := "lse_" + uuid.NewString()

	var claimed session.State
	var ok bool
	claim := func(store session.Store) error {
		st, found, err := store.ClaimNext(mode, leaseID, now, expiresAt)
		if err != nil {
			return err
		}
		if !found {
			return nil
		}
		claimed = st
		ok = true
		return nil
	}

	if s.Runner != nil {
		if err := s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
			store := s.Sessions
			if repos.Sessions != nil {
				store = repos.Sessions
			}
			return claim(store)
		}); err != nil {
			return session.State{}, false, err
		}
		return claimed, ok, nil
	}

	if err := claim(s.Sessions); err != nil {
		return session.State{}, false, err
	}
	return claimed, ok, nil
}
