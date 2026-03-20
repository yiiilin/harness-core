package execution

import (
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

var ErrRecordNotFound = errors.New("execution record not found")

type AttemptStore interface {
	Create(spec Attempt) (Attempt, error)
	Get(id string) (Attempt, error)
	Update(next Attempt) error
	List(sessionID string) ([]Attempt, error)
}

type ActionStore interface {
	Create(spec ActionRecord) (ActionRecord, error)
	Get(id string) (ActionRecord, error)
	Update(next ActionRecord) error
	List(sessionID string) ([]ActionRecord, error)
}

type VerificationStore interface {
	Create(spec VerificationRecord) (VerificationRecord, error)
	Get(id string) (VerificationRecord, error)
	Update(next VerificationRecord) error
	List(sessionID string) ([]VerificationRecord, error)
}

type ArtifactStore interface {
	Create(spec Artifact) (Artifact, error)
	Get(id string) (Artifact, error)
	Update(next Artifact) error
	List(sessionID string) ([]Artifact, error)
}

type RuntimeHandleStore interface {
	Create(spec RuntimeHandle) (RuntimeHandle, error)
	Get(id string) (RuntimeHandle, error)
	Update(next RuntimeHandle) error
	List(sessionID string) ([]RuntimeHandle, error)
}

type MemoryAttemptStore struct {
	mu    sync.RWMutex
	items map[string]Attempt
}

type MemoryActionStore struct {
	mu    sync.RWMutex
	items map[string]ActionRecord
}

type MemoryVerificationStore struct {
	mu    sync.RWMutex
	items map[string]VerificationRecord
}

type MemoryArtifactStore struct {
	mu    sync.RWMutex
	items map[string]Artifact
}

type MemoryRuntimeHandleStore struct {
	mu    sync.RWMutex
	items map[string]RuntimeHandle
}

func NewMemoryAttemptStore() *MemoryAttemptStore {
	return &MemoryAttemptStore{items: map[string]Attempt{}}
}

func NewMemoryActionStore() *MemoryActionStore {
	return &MemoryActionStore{items: map[string]ActionRecord{}}
}

func NewMemoryVerificationStore() *MemoryVerificationStore {
	return &MemoryVerificationStore{items: map[string]VerificationRecord{}}
}

func NewMemoryArtifactStore() *MemoryArtifactStore {
	return &MemoryArtifactStore{items: map[string]Artifact{}}
}

func NewMemoryRuntimeHandleStore() *MemoryRuntimeHandleStore {
	return &MemoryRuntimeHandleStore{items: map[string]RuntimeHandle{}}
}

func (s *MemoryAttemptStore) Create(spec Attempt) (Attempt, error) {
	if spec.AttemptID == "" {
		spec.AttemptID = uuid.NewString()
	}
	if spec.StartedAt == 0 {
		spec.StartedAt = time.Now().UnixMilli()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[spec.AttemptID] = spec
	return spec, nil
}

func (s *MemoryAttemptStore) Get(id string) (Attempt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[id]
	if !ok {
		return Attempt{}, ErrRecordNotFound
	}
	return item, nil
}

func (s *MemoryAttemptStore) Update(next Attempt) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[next.AttemptID]; !ok {
		return ErrRecordNotFound
	}
	s.items[next.AttemptID] = next
	return nil
}

func (s *MemoryAttemptStore) List(sessionID string) ([]Attempt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Attempt, 0, len(s.items))
	for _, item := range s.items {
		if sessionID == "" || item.SessionID == sessionID {
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt < out[j].StartedAt })
	return out, nil
}

func (s *MemoryActionStore) Create(spec ActionRecord) (ActionRecord, error) {
	if spec.ActionID == "" {
		spec.ActionID = uuid.NewString()
	}
	if spec.StartedAt == 0 {
		spec.StartedAt = time.Now().UnixMilli()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[spec.ActionID] = spec
	return spec, nil
}

func (s *MemoryActionStore) Get(id string) (ActionRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[id]
	if !ok {
		return ActionRecord{}, ErrRecordNotFound
	}
	return item, nil
}

func (s *MemoryActionStore) Update(next ActionRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[next.ActionID]; !ok {
		return ErrRecordNotFound
	}
	s.items[next.ActionID] = next
	return nil
}

func (s *MemoryActionStore) List(sessionID string) ([]ActionRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ActionRecord, 0, len(s.items))
	for _, item := range s.items {
		if sessionID == "" || item.SessionID == sessionID {
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt < out[j].StartedAt })
	return out, nil
}

func (s *MemoryVerificationStore) Create(spec VerificationRecord) (VerificationRecord, error) {
	if spec.VerificationID == "" {
		spec.VerificationID = uuid.NewString()
	}
	if spec.StartedAt == 0 {
		spec.StartedAt = time.Now().UnixMilli()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[spec.VerificationID] = spec
	return spec, nil
}

func (s *MemoryVerificationStore) Get(id string) (VerificationRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[id]
	if !ok {
		return VerificationRecord{}, ErrRecordNotFound
	}
	return item, nil
}

func (s *MemoryVerificationStore) Update(next VerificationRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[next.VerificationID]; !ok {
		return ErrRecordNotFound
	}
	s.items[next.VerificationID] = next
	return nil
}

func (s *MemoryVerificationStore) List(sessionID string) ([]VerificationRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]VerificationRecord, 0, len(s.items))
	for _, item := range s.items {
		if sessionID == "" || item.SessionID == sessionID {
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt < out[j].StartedAt })
	return out, nil
}

func (s *MemoryArtifactStore) Create(spec Artifact) (Artifact, error) {
	if spec.ArtifactID == "" {
		spec.ArtifactID = uuid.NewString()
	}
	if spec.CreatedAt == 0 {
		spec.CreatedAt = time.Now().UnixMilli()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[spec.ArtifactID] = spec
	return spec, nil
}

func (s *MemoryArtifactStore) Get(id string) (Artifact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[id]
	if !ok {
		return Artifact{}, ErrRecordNotFound
	}
	return item, nil
}

func (s *MemoryArtifactStore) Update(next Artifact) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[next.ArtifactID]; !ok {
		return ErrRecordNotFound
	}
	s.items[next.ArtifactID] = next
	return nil
}

func (s *MemoryArtifactStore) List(sessionID string) ([]Artifact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Artifact, 0, len(s.items))
	for _, item := range s.items {
		if sessionID == "" || item.SessionID == sessionID {
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt < out[j].CreatedAt })
	return out, nil
}

func (s *MemoryRuntimeHandleStore) Create(spec RuntimeHandle) (RuntimeHandle, error) {
	now := time.Now().UnixMilli()
	if spec.HandleID == "" {
		spec.HandleID = uuid.NewString()
	}
	if spec.CreatedAt == 0 {
		spec.CreatedAt = now
	}
	if spec.UpdatedAt == 0 {
		spec.UpdatedAt = now
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[spec.HandleID] = spec
	return spec, nil
}

func (s *MemoryRuntimeHandleStore) Get(id string) (RuntimeHandle, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[id]
	if !ok {
		return RuntimeHandle{}, ErrRecordNotFound
	}
	return item, nil
}

func (s *MemoryRuntimeHandleStore) Update(next RuntimeHandle) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[next.HandleID]; !ok {
		return ErrRecordNotFound
	}
	next.UpdatedAt = time.Now().UnixMilli()
	s.items[next.HandleID] = next
	return nil
}

func (s *MemoryRuntimeHandleStore) List(sessionID string) ([]RuntimeHandle, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]RuntimeHandle, 0, len(s.items))
	for _, item := range s.items {
		if sessionID == "" || item.SessionID == sessionID {
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt < out[j].CreatedAt })
	return out, nil
}
