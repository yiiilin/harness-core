package approval

import (
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
)

type Reply string

type Status string

const (
	ReplyOnce   Reply = "once"
	ReplyAlways Reply = "always"
	ReplyReject Reply = "reject"

	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusRejected Status = "rejected"
	StatusConsumed Status = "consumed"
)

type Request struct {
	SessionID   string         `json:"session_id"`
	TaskID      string         `json:"task_id,omitempty"`
	StepID      string         `json:"step_id,omitempty"`
	ToolName    string         `json:"tool_name,omitempty"`
	Reason      string         `json:"reason,omitempty"`
	MatchedRule string         `json:"matched_rule,omitempty"`
	Step        plan.StepSpec  `json:"step"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type Response struct {
	Reply    Reply          `json:"reply"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type Record struct {
	ApprovalID  string         `json:"approval_id"`
	SessionID   string         `json:"session_id"`
	TaskID      string         `json:"task_id,omitempty"`
	StepID      string         `json:"step_id,omitempty"`
	ToolName    string         `json:"tool_name,omitempty"`
	Reason      string         `json:"reason,omitempty"`
	MatchedRule string         `json:"matched_rule,omitempty"`
	Status      Status         `json:"status"`
	Reply       Reply          `json:"reply,omitempty"`
	Step        plan.StepSpec  `json:"step"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	RequestedAt int64          `json:"requested_at"`
	RespondedAt int64          `json:"responded_at,omitempty"`
	ConsumedAt  int64          `json:"consumed_at,omitempty"`
	CreatedAt   int64          `json:"created_at"`
	UpdatedAt   int64          `json:"updated_at"`
}

type Store interface {
	CreatePending(req Request) Record
	Get(id string) (Record, error)
	Update(next Record) error
	List(sessionID string) []Record
}

type ResumePolicy interface {
	Resolve(record Record, step plan.StepSpec) (permission.Decision, bool)
}

type DefaultResumePolicy struct{}

func (DefaultResumePolicy) Resolve(record Record, step plan.StepSpec) (permission.Decision, bool) {
	if record.Status != StatusApproved {
		return permission.Decision{}, false
	}
	if record.Reply == ReplyAlways && record.ToolName == step.Action.ToolName {
		return permission.Decision{
			Action:      permission.Allow,
			Reason:      "approval previously granted",
			MatchedRule: "approval/always",
		}, true
	}
	if record.StepID != step.StepID || record.ToolName != step.Action.ToolName {
		return permission.Decision{}, false
	}
	switch record.Reply {
	case ReplyOnce:
		return permission.Decision{
			Action:      permission.Allow,
			Reason:      "approval granted",
			MatchedRule: "approval/once",
		}, true
	case ReplyAlways:
		return permission.Decision{
			Action:      permission.Allow,
			Reason:      "approval previously granted",
			MatchedRule: "approval/always",
		}, true
	case ReplyReject:
		return permission.Decision{
			Action:      permission.Deny,
			Reason:      "approval rejected",
			MatchedRule: "approval/reject",
		}, true
	default:
		return permission.Decision{}, false
	}
}

type MemoryStore struct {
	mu        sync.RWMutex
	approvals map[string]Record
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{approvals: map[string]Record{}}
}

func (s *MemoryStore) CreatePending(req Request) Record {
	now := time.Now().UnixMilli()
	rec := Record{
		ApprovalID:  uuid.NewString(),
		SessionID:   req.SessionID,
		TaskID:      req.TaskID,
		StepID:      req.StepID,
		ToolName:    req.ToolName,
		Reason:      req.Reason,
		MatchedRule: req.MatchedRule,
		Status:      StatusPending,
		Step:        req.Step,
		Metadata:    req.Metadata,
		RequestedAt: now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.approvals[rec.ApprovalID] = rec
	return rec
}

func (s *MemoryStore) Get(id string) (Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.approvals[id]
	if !ok {
		return Record{}, ErrApprovalNotFound
	}
	return rec, nil
}

func (s *MemoryStore) Update(next Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.approvals[next.ApprovalID]; !ok {
		return ErrApprovalNotFound
	}
	next.UpdatedAt = time.Now().UnixMilli()
	s.approvals[next.ApprovalID] = next
	return nil
}

func (s *MemoryStore) List(sessionID string) []Record {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Record, 0, len(s.approvals))
	for _, rec := range s.approvals {
		if sessionID == "" || rec.SessionID == sessionID {
			out = append(out, rec)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].RequestedAt == out[j].RequestedAt {
			return out[i].ApprovalID < out[j].ApprovalID
		}
		return out[i].RequestedAt < out[j].RequestedAt
	})
	return out
}
