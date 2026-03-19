package runtime

import (
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Session struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Goal      string `json:"goal,omitempty"`
	Phase     string `json:"phase"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

type Store struct {
	mu       sync.RWMutex
	sessions map[string]Session
}

func NewStore() *Store {
	return &Store{sessions: map[string]Session{}}
}

func (s *Store) Create(title, goal string) Session {
	now := time.Now().UnixMilli()
	sess := Session{
		ID:        uuid.NewString(),
		Title:     title,
		Goal:      goal,
		Phase:     "received",
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sess.ID] = sess
	return sess
}

func (s *Store) Get(id string) (Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	if !ok {
		return Session{}, errors.New("session not found")
	}
	return sess, nil
}
