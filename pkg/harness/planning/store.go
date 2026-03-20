package planning

import (
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

type Record struct {
	PlanningID       string         `json:"planning_id"`
	SessionID        string         `json:"session_id"`
	TaskID           string         `json:"task_id,omitempty"`
	Status           Status         `json:"status"`
	Reason           string         `json:"reason,omitempty"`
	Error            string         `json:"error,omitempty"`
	PlanID           string         `json:"plan_id,omitempty"`
	PlanRevision     int            `json:"plan_revision,omitempty"`
	CapabilityViewID string         `json:"capability_view_id,omitempty"`
	ContextSummaryID string         `json:"context_summary_id,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	StartedAt        int64          `json:"started_at"`
	FinishedAt       int64          `json:"finished_at,omitempty"`
}

type Store interface {
	Create(spec Record) (Record, error)
	Get(id string) (Record, error)
	List(sessionID string) ([]Record, error)
}

type MemoryStore struct {
	mu    sync.RWMutex
	items map[string]Record
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{items: map[string]Record{}}
}

func (s *MemoryStore) Create(spec Record) (Record, error) {
	if spec.PlanningID == "" {
		spec.PlanningID = "pln_" + uuid.NewString()
	}
	if spec.StartedAt == 0 {
		spec.StartedAt = time.Now().UnixMilli()
	}
	if spec.FinishedAt == 0 {
		spec.FinishedAt = spec.StartedAt
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[spec.PlanningID] = spec
	return spec, nil
}

func (s *MemoryStore) Get(id string) (Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[id]
	if !ok {
		return Record{}, ErrPlanningRecordNotFound
	}
	return item, nil
}

func (s *MemoryStore) List(sessionID string) ([]Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Record, 0, len(s.items))
	for _, item := range s.items {
		if sessionID == "" || item.SessionID == sessionID {
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartedAt == out[j].StartedAt {
			if out[i].PlanRevision == out[j].PlanRevision {
				return out[i].PlanningID < out[j].PlanningID
			}
			return out[i].PlanRevision < out[j].PlanRevision
		}
		return out[i].StartedAt < out[j].StartedAt
	})
	return out, nil
}
