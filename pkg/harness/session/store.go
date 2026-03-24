package session

import (
	"sort"
	"sync"

	"github.com/google/uuid"
)

type Store interface {
	Create(title, goal string) (State, error)
	Get(id string) (State, error)
	Update(next State) error
	ClaimNext(mode ClaimMode, leaseID string, claimedAt, expiresAt int64) (State, bool, error)
	RenewLease(sessionID, leaseID string, now, expiresAt int64) (State, error)
	ReleaseLease(sessionID, leaseID string, now int64) (State, error)
	List() ([]State, error)
}

type MemoryStore struct {
	mu       sync.RWMutex
	sessions map[string]State
	clock    Clock
}

func NewMemoryStore() *MemoryStore {
	return NewMemoryStoreWithClock(systemClock{})
}

func NewMemoryStoreWithClock(clock Clock) *MemoryStore {
	if clock == nil {
		clock = systemClock{}
	}
	return &MemoryStore{sessions: map[string]State{}, clock: clock}
}

func (s *MemoryStore) Create(title, goal string) (State, error) {
	now := s.clock.NowMilli()
	st := State{
		SessionID:       uuid.NewString(),
		Title:           title,
		Goal:            goal,
		Phase:           PhaseReceived,
		RetryCount:      0,
		ExecutionState:  ExecutionIdle,
		LastHeartbeatAt: now,
		Version:         1,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[st.SessionID] = st
	return st, nil
}

func (s *MemoryStore) Get(id string) (State, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, ok := s.sessions[id]
	if !ok {
		return State{}, ErrSessionNotFound
	}
	return st, nil
}

func (s *MemoryStore) Update(next State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.sessions[next.SessionID]
	if !ok {
		return ErrSessionNotFound
	}
	if next.Version != current.Version+1 {
		return ErrSessionVersionConflict
	}
	if next.CreatedAt == 0 {
		next.CreatedAt = current.CreatedAt
	}
	if next.RuntimeStartedAt == 0 {
		next.RuntimeStartedAt = current.RuntimeStartedAt
	}
	next.UpdatedAt = s.clock.NowMilli()
	s.sessions[next.SessionID] = next
	return nil
}

func (s *MemoryStore) List() ([]State, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]State, 0, len(s.sessions))
	for _, v := range s.sessions {
		out = append(out, v)
	}
	return out, nil
}

func (s *MemoryStore) ClaimNext(mode ClaimMode, leaseID string, claimedAt, expiresAt int64) (State, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ordered := make([]State, 0, len(s.sessions))
	for _, st := range s.sessions {
		ordered = append(ordered, st)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].CreatedAt == ordered[j].CreatedAt {
			return ordered[i].SessionID < ordered[j].SessionID
		}
		return ordered[i].CreatedAt < ordered[j].CreatedAt
	})
	for _, orderedState := range ordered {
		st := s.sessions[orderedState.SessionID]
		if !claimableState(st, mode, claimedAt) {
			continue
		}
		st.LeaseID = leaseID
		st.LeaseClaimedAt = claimedAt
		st.LeaseExpiresAt = expiresAt
		st.LastHeartbeatAt = claimedAt
		st.Version++
		st.UpdatedAt = claimedAt
		s.sessions[st.SessionID] = st
		return st, true, nil
	}
	return State{}, false, nil
}

func (s *MemoryStore) RenewLease(sessionID, leaseID string, now, expiresAt int64) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	st, ok := s.sessions[sessionID]
	if !ok {
		return State{}, ErrSessionNotFound
	}
	if !leaseHeldAt(st, leaseID, now) {
		return State{}, ErrSessionLeaseNotHeld
	}
	st.LeaseExpiresAt = expiresAt
	st.LastHeartbeatAt = now
	st.Version++
	st.UpdatedAt = now
	s.sessions[sessionID] = st
	return st, nil
}

func (s *MemoryStore) ReleaseLease(sessionID, leaseID string, now int64) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	st, ok := s.sessions[sessionID]
	if !ok {
		return State{}, ErrSessionNotFound
	}
	if !leaseHeldAt(st, leaseID, now) {
		return State{}, ErrSessionLeaseNotHeld
	}
	st.LeaseID = ""
	st.LeaseClaimedAt = 0
	st.LeaseExpiresAt = 0
	st.Version++
	st.UpdatedAt = now
	s.sessions[sessionID] = st
	return st, nil
}

func claimableState(st State, mode ClaimMode, now int64) bool {
	if leaseActiveAt(st, now) {
		return false
	}
	if st.ExecutionState == ExecutionAwaitingApproval || st.ExecutionState == ExecutionBlocked {
		return false
	}
	switch mode {
	case ClaimModeRunnable:
		if st.ExecutionState != ExecutionIdle {
			return false
		}
		return st.Phase != PhaseComplete && st.Phase != PhaseFailed && st.Phase != PhaseAborted
	case ClaimModeRecoverable:
		return IsRecoverableState(st)
	default:
		return false
	}
}

func leaseActiveAt(st State, now int64) bool {
	return st.LeaseID != "" && st.LeaseExpiresAt > now
}

func leaseHeldAt(st State, leaseID string, now int64) bool {
	return leaseID != "" && st.LeaseID == leaseID && leaseActiveAt(st, now)
}
