package session

import (
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Store interface {
	Create(title, goal string) State
	Get(id string) (State, error)
	Update(next State) error
	List() []State
}

type MemoryStore struct {
	mu       sync.RWMutex
	sessions map[string]State
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{sessions: map[string]State{}}
}

func (s *MemoryStore) Create(title, goal string) State {
	now := time.Now().UnixMilli()
	st := State{
		SessionID: uuid.NewString(),
		Title:     title,
		Goal:      goal,
		Phase:     PhaseReceived,
		RetryCount: 0,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[st.SessionID] = st
	return st
}

func (s *MemoryStore) Get(id string) (State, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, ok := s.sessions[id]
	if !ok {
		return State{}, errors.New("session not found")
	}
	return st, nil
}

func (s *MemoryStore) Update(next State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sessions[next.SessionID]; !ok {
		return errors.New("session not found")
	}
	next.UpdatedAt = time.Now().UnixMilli()
	s.sessions[next.SessionID] = next
	return nil
}

func (s *MemoryStore) List() []State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]State, 0, len(s.sessions))
	for _, v := range s.sessions {
		out = append(out, v)
	}
	return out
}
