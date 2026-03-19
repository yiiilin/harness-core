package task

import (
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Store interface {
	Create(spec Spec) Record
	Get(id string) (Record, error)
	Update(next Record) error
	List() []Record
}

type MemoryStore struct {
	mu    sync.RWMutex
	tasks map[string]Record
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{tasks: map[string]Record{}}
}

func (s *MemoryStore) Create(spec Spec) Record {
	now := time.Now().UnixMilli()
	id := spec.TaskID
	if id == "" {
		id = uuid.NewString()
	}
	rec := Record{
		TaskID:      id,
		TaskType:    spec.TaskType,
		Goal:        spec.Goal,
		Status:      StatusReceived,
		Constraints: spec.Constraints,
		Metadata:    spec.Metadata,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[rec.TaskID] = rec
	return rec
}

func (s *MemoryStore) Get(id string) (Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.tasks[id]
	if !ok {
		return Record{}, errors.New("task not found")
	}
	return rec, nil
}

func (s *MemoryStore) Update(next Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tasks[next.TaskID]; !ok {
		return errors.New("task not found")
	}
	next.UpdatedAt = time.Now().UnixMilli()
	s.tasks[next.TaskID] = next
	return nil
}

func (s *MemoryStore) List() []Record {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Record, 0, len(s.tasks))
	for _, v := range s.tasks {
		out = append(out, v)
	}
	return out
}
