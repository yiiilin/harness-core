package runtime

import (
	"errors"

	"github.com/yiiilin/harness-core/pkg/harness/session"
)

const maxSessionUpdateAttempts = 4

func persistSessionUpdate(store session.Store, desired session.State, leaseID string) (session.State, error) {
	return persistSessionUpdateAt(store, desired, leaseID, 0)
}

func persistSessionUpdateAt(store session.Store, desired session.State, leaseID string, now int64) (session.State, error) {
	if leaseID == "" {
		desired.Version++
		if err := store.Update(desired); err != nil {
			return session.State{}, err
		}
		return desired, nil
	}
	return updateSessionWithRetry(store, desired, leaseID, now)
}

func updateSessionWithRetry(store session.Store, desired session.State, leaseID string, now int64) (session.State, error) {
	if store == nil {
		return session.State{}, session.ErrSessionNotFound
	}
	if now == 0 {
		now = systemClock{}.NowMilli()
	}

	for attempt := 0; attempt < maxSessionUpdateAttempts; attempt++ {
		current, err := store.Get(desired.SessionID)
		if err != nil {
			return session.State{}, err
		}
		if err := requireSessionLease(current, leaseID, now); err != nil {
			return session.State{}, err
		}

		candidate := mergeSessionUpdate(current, desired, leaseID)
		if err := store.Update(candidate); err != nil {
			if errors.Is(err, session.ErrSessionVersionConflict) {
				continue
			}
			return session.State{}, err
		}
		return candidate, nil
	}

	return session.State{}, session.ErrSessionVersionConflict
}

func mergeSessionUpdate(current, desired session.State, leaseID string) session.State {
	candidate := desired
	candidate.Version = current.Version + 1
	candidate.CreatedAt = current.CreatedAt
	if candidate.RuntimeStartedAt == 0 {
		candidate.RuntimeStartedAt = current.RuntimeStartedAt
	}

	if current.LeaseID != "" && (leaseID == "" || current.LeaseID == leaseID) {
		candidate.LeaseID = current.LeaseID
		candidate.LeaseClaimedAt = current.LeaseClaimedAt
		candidate.LeaseExpiresAt = current.LeaseExpiresAt
		if current.LastHeartbeatAt > candidate.LastHeartbeatAt {
			candidate.LastHeartbeatAt = current.LastHeartbeatAt
		}
	}
	if candidate.LastHeartbeatAt == 0 {
		candidate.LastHeartbeatAt = current.LastHeartbeatAt
	}

	return candidate
}
