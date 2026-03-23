package runtime

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
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
	startedAt := s.nowMilli()
	now := s.nowMilli()
	expiresAt := now + leaseTTL.Milliseconds()

	var updated session.State
	renew := func(store session.Store, sink EventSink) error {
		st, err := store.RenewLease(sessionID, leaseID, now, expiresAt)
		if err != nil {
			return err
		}
		updated = st
		event := newAuditEventAt(s.nowMilli(), audit.EventLeaseRenewed, updated.SessionID, updated.TaskID, updated.InFlightStepID, map[string]any{
			"lease_id":         leaseID,
			"lease_claimed_at": updated.LeaseClaimedAt,
			"lease_expires_at": updated.LeaseExpiresAt,
			"ttl_ms":           leaseTTL.Milliseconds(),
		})
		if sink != nil {
			return s.emitEventsWithSink(ctx, sink, []audit.Event{event})
		}
		_ = s.emitEvents(ctx, []audit.Event{event})
		return nil
	}

	if s.Runner != nil {
		if err := s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
			store := s.Sessions
			if repos.Sessions != nil {
				store = repos.Sessions
			}
			return renew(store, s.eventSinkForRepos(repos))
		}); err != nil {
			return session.State{}, err
		}
		s.exportLeaseObservability(ctx, "lease.renew", updated, leaseID, startedAt, s.nowMilli(), map[string]any{"ttl_ms": leaseTTL.Milliseconds()})
		return updated, nil
	}

	if err := renew(s.Sessions, nil); err != nil {
		return session.State{}, err
	}
	s.exportLeaseObservability(ctx, "lease.renew", updated, leaseID, startedAt, s.nowMilli(), map[string]any{"ttl_ms": leaseTTL.Milliseconds()})
	return updated, nil
}

func (s *Service) ReleaseSessionLease(ctx context.Context, sessionID, leaseID string) (session.State, error) {
	startedAt := s.nowMilli()
	now := s.nowMilli()
	var updated session.State
	release := func(store session.Store, sink EventSink) error {
		st, err := store.ReleaseLease(sessionID, leaseID, now)
		if err != nil {
			return err
		}
		updated = st
		event := newAuditEventAt(s.nowMilli(), audit.EventLeaseReleased, sessionID, updated.TaskID, updated.InFlightStepID, map[string]any{
			"lease_id":         leaseID,
			"released_at":      now,
			"lease_claimed_at": updated.LeaseClaimedAt,
			"lease_expires_at": updated.LeaseExpiresAt,
		})
		if sink != nil {
			return s.emitEventsWithSink(ctx, sink, []audit.Event{event})
		}
		_ = s.emitEvents(ctx, []audit.Event{event})
		return nil
	}

	if s.Runner != nil {
		if err := s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
			store := s.Sessions
			if repos.Sessions != nil {
				store = repos.Sessions
			}
			return release(store, s.eventSinkForRepos(repos))
		}); err != nil {
			return session.State{}, err
		}
		s.exportLeaseObservability(ctx, "lease.release", updated, leaseID, startedAt, s.nowMilli(), map[string]any{})
		return updated, nil
	}

	if err := release(s.Sessions, nil); err != nil {
		return session.State{}, err
	}
	s.exportLeaseObservability(ctx, "lease.release", updated, leaseID, startedAt, s.nowMilli(), map[string]any{})
	return updated, nil
}

func (s *Service) claimSession(ctx context.Context, mode session.ClaimMode, leaseTTL time.Duration) (session.State, bool, error) {
	if leaseTTL <= 0 {
		return session.State{}, false, ErrInvalidLeaseTTL
	}
	startedAt := s.nowMilli()
	now := s.nowMilli()
	expiresAt := now + leaseTTL.Milliseconds()
	leaseID := "lse_" + uuid.NewString()

	var claimed session.State
	var ok bool
	claim := func(store session.Store, sink EventSink) error {
		st, found, err := store.ClaimNext(mode, leaseID, now, expiresAt)
		if err != nil {
			return err
		}
		if !found {
			return nil
		}
		claimed = st
		ok = true
		event := newAuditEventAt(s.nowMilli(), audit.EventLeaseClaimed, claimed.SessionID, claimed.TaskID, claimed.InFlightStepID, map[string]any{
			"lease_id":         claimed.LeaseID,
			"claim_mode":       string(mode),
			"lease_claimed_at": claimed.LeaseClaimedAt,
			"lease_expires_at": claimed.LeaseExpiresAt,
			"ttl_ms":           leaseTTL.Milliseconds(),
		})
		if sink != nil {
			return s.emitEventsWithSink(ctx, sink, []audit.Event{event})
		}
		_ = s.emitEvents(ctx, []audit.Event{event})
		return nil
	}

	if s.Runner != nil {
		if err := s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
			store := s.Sessions
			if repos.Sessions != nil {
				store = repos.Sessions
			}
			return claim(store, s.eventSinkForRepos(repos))
		}); err != nil {
			return session.State{}, false, err
		}
		s.exportLeaseObservability(ctx, "lease.claim", claimed, claimed.LeaseID, startedAt, s.nowMilli(), map[string]any{"claimed": ok, "claim_mode": string(mode), "ttl_ms": leaseTTL.Milliseconds()})
		return claimed, ok, nil
	}

	if err := claim(s.Sessions, nil); err != nil {
		return session.State{}, false, err
	}
	s.exportLeaseObservability(ctx, "lease.claim", claimed, claimed.LeaseID, startedAt, s.nowMilli(), map[string]any{"claimed": ok, "claim_mode": string(mode), "ttl_ms": leaseTTL.Milliseconds()})
	return claimed, ok, nil
}
