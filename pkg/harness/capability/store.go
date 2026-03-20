package capability

import (
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

type SnapshotStore interface {
	Create(spec Snapshot) (Snapshot, error)
	Get(id string) (Snapshot, error)
	List(sessionID string) ([]Snapshot, error)
}

type MemorySnapshotStore struct {
	mu    sync.RWMutex
	items map[string]Snapshot
}

func NewMemorySnapshotStore() *MemorySnapshotStore {
	return &MemorySnapshotStore{items: map[string]Snapshot{}}
}

func (s *MemorySnapshotStore) Create(spec Snapshot) (Snapshot, error) {
	if spec.SnapshotID == "" {
		spec.SnapshotID = "cap_" + uuid.NewString()
	}
	if spec.Scope == "" {
		spec.Scope = SnapshotScopeAction
	}
	if spec.ResolvedAt == 0 {
		spec.ResolvedAt = time.Now().UnixMilli()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[spec.SnapshotID] = spec
	return spec, nil
}

func (s *MemorySnapshotStore) Get(id string) (Snapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[id]
	if !ok {
		return Snapshot{}, ErrSnapshotNotFound
	}
	return item, nil
}

func (s *MemorySnapshotStore) List(sessionID string) ([]Snapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Snapshot, 0, len(s.items))
	for _, item := range s.items {
		if sessionID == "" || item.SessionID == sessionID {
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ResolvedAt < out[j].ResolvedAt })
	return out, nil
}
