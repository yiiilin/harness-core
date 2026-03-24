package execution

import (
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

type BlockedRuntimeStore interface {
	Create(spec BlockedRuntimeRecord) (BlockedRuntimeRecord, error)
	Get(id string) (BlockedRuntimeRecord, error)
	Update(next BlockedRuntimeRecord) error
	List(sessionID string) ([]BlockedRuntimeRecord, error)
}

type MemoryBlockedRuntimeStore struct {
	mu    sync.RWMutex
	items map[string]BlockedRuntimeRecord
}

func NewMemoryBlockedRuntimeStore() *MemoryBlockedRuntimeStore {
	return &MemoryBlockedRuntimeStore{items: map[string]BlockedRuntimeRecord{}}
}

func (s *MemoryBlockedRuntimeStore) Create(spec BlockedRuntimeRecord) (BlockedRuntimeRecord, error) {
	if spec.BlockedRuntimeID == "" {
		spec.BlockedRuntimeID = "blocked_" + uuid.NewString()
	}
	now := time.Now().UnixMilli()
	if spec.RequestedAt == 0 {
		spec.RequestedAt = now
	}
	if spec.UpdatedAt == 0 {
		spec.UpdatedAt = spec.RequestedAt
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[spec.BlockedRuntimeID] = spec
	return spec, nil
}

func (s *MemoryBlockedRuntimeStore) Get(id string) (BlockedRuntimeRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[id]
	if !ok {
		return BlockedRuntimeRecord{}, ErrRecordNotFound
	}
	return item, nil
}

func (s *MemoryBlockedRuntimeStore) Update(next BlockedRuntimeRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[next.BlockedRuntimeID]; !ok {
		return ErrRecordNotFound
	}
	if next.UpdatedAt == 0 {
		next.UpdatedAt = time.Now().UnixMilli()
	}
	s.items[next.BlockedRuntimeID] = next
	return nil
}

func (s *MemoryBlockedRuntimeStore) List(sessionID string) ([]BlockedRuntimeRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]BlockedRuntimeRecord, 0, len(s.items))
	for _, item := range s.items {
		if sessionID == "" || item.SessionID == sessionID {
			out = append(out, item)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].RequestedAt != out[j].RequestedAt {
			return out[i].RequestedAt < out[j].RequestedAt
		}
		return out[i].BlockedRuntimeID < out[j].BlockedRuntimeID
	})
	return out, nil
}
