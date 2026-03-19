package audit

import "sync"

type Store interface {
	Emit(event Event) error
	List(sessionID string) []Event
}

type MemoryStore struct {
	mu     sync.RWMutex
	events []Event
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{events: []Event{}}
}

func (s *MemoryStore) Emit(event Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

func (s *MemoryStore) List(sessionID string) []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []Event{}
	for _, e := range s.events {
		if sessionID == "" || e.SessionID == sessionID {
			out = append(out, e)
		}
	}
	return out
}
