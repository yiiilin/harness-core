package contextsummary

import (
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Trigger string

type Summary struct {
	SummaryID           string         `json:"summary_id"`
	SessionID           string         `json:"session_id,omitempty"`
	TaskID              string         `json:"task_id,omitempty"`
	Sequence            int64          `json:"sequence,omitempty"`
	Trigger             Trigger        `json:"trigger,omitempty"`
	SupersedesSummaryID string         `json:"supersedes_summary_id,omitempty"`
	Strategy            string         `json:"strategy,omitempty"`
	Summary             map[string]any `json:"summary,omitempty"`
	Metadata            map[string]any `json:"metadata,omitempty"`
	OriginalBytes       int            `json:"original_bytes,omitempty"`
	CompactedBytes      int            `json:"compacted_bytes,omitempty"`
	CreatedAt           int64          `json:"created_at"`
}

type Store interface {
	Create(spec Summary) (Summary, error)
	Get(id string) (Summary, error)
	List(sessionID string) ([]Summary, error)
}

var ErrContextSummaryNotFound = errors.New("context summary not found")

type MemoryStore struct {
	mu           sync.RWMutex
	items        map[string]Summary
	nextSequence int64
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{items: map[string]Summary{}}
}

func (s *MemoryStore) Create(spec Summary) (Summary, error) {
	if spec.SummaryID == "" {
		spec.SummaryID = "ctx_" + uuid.NewString()
	}
	if spec.CreatedAt == 0 {
		spec.CreatedAt = time.Now().UnixMilli()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if spec.Sequence == 0 {
		s.nextSequence++
		spec.Sequence = s.nextSequence
	}
	s.items[spec.SummaryID] = spec
	return spec, nil
}

func (s *MemoryStore) Get(id string) (Summary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[id]
	if !ok {
		return Summary{}, ErrContextSummaryNotFound
	}
	return item, nil
}

func (s *MemoryStore) List(sessionID string) ([]Summary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Summary, 0, len(s.items))
	for _, item := range s.items {
		if sessionID == "" || item.SessionID == sessionID {
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt == out[j].CreatedAt {
			if out[i].Sequence == out[j].Sequence {
				return out[i].SummaryID < out[j].SummaryID
			}
			return out[i].Sequence < out[j].Sequence
		}
		return out[i].CreatedAt < out[j].CreatedAt
	})
	return out, nil
}
