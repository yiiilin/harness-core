package plan

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

type Store interface {
	Create(sessionID, changeReason string, steps []StepSpec) Spec
	Get(id string) (Spec, error)
	ListBySession(sessionID string) []Spec
	LatestBySession(sessionID string) (Spec, bool)
	Update(next Spec) error
}

type MemoryStore struct {
	mu    sync.RWMutex
	plans map[string]Spec
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{plans: map[string]Spec{}}
}

func (s *MemoryStore) Create(sessionID, changeReason string, steps []StepSpec) Spec {
	now := time.Now().UnixMilli()
	revision := 1
	if latest, ok := s.LatestBySession(sessionID); ok {
		revision = latest.Revision + 1
	}
	cloned := make([]StepSpec, len(steps))
	copy(cloned, steps)
	for i := range cloned {
		if cloned[i].Status == "" {
			cloned[i].Status = StepPending
		}
	}
	plan := Spec{
		PlanID:       uuid.NewString(),
		SessionID:    sessionID,
		Revision:     revision,
		Status:       StatusActive,
		ChangeReason: changeReason,
		Steps:        cloned,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.plans[plan.PlanID] = plan
	return plan
}

func (s *MemoryStore) Get(id string) (Spec, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.plans[id]
	if !ok {
		return Spec{}, ErrPlanNotFound
	}
	return p, nil
}

func (s *MemoryStore) ListBySession(sessionID string) []Spec {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []Spec{}
	for _, p := range s.plans {
		if p.SessionID == sessionID {
			out = append(out, p)
		}
	}
	return out
}

func (s *MemoryStore) LatestBySession(sessionID string) (Spec, bool) {
	plans := s.ListBySession(sessionID)
	if len(plans) == 0 {
		return Spec{}, false
	}
	latest := plans[0]
	for _, p := range plans[1:] {
		if p.Revision > latest.Revision {
			latest = p
		}
	}
	return latest, true
}

func (s *MemoryStore) Update(next Spec) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.plans[next.PlanID]; !ok {
		return ErrPlanNotFound
	}
	next.UpdatedAt = time.Now().UnixMilli()
	s.plans[next.PlanID] = next
	return nil
}
