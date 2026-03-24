package execution

import (
	"errors"

	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
)

var ErrBlockedRuntimeNotFound = errors.New("blocked runtime not found")

type BlockedRuntimeKind string
type BlockedRuntimeStatus string

const (
	BlockedRuntimeApproval     BlockedRuntimeKind = "approval"
	BlockedRuntimeConfirmation BlockedRuntimeKind = "confirmation"
	BlockedRuntimeExternal     BlockedRuntimeKind = "external"
	BlockedRuntimeInteractive  BlockedRuntimeKind = "interactive"

	BlockedRuntimePending   BlockedRuntimeStatus = "pending"
	BlockedRuntimeApproved  BlockedRuntimeStatus = "approved"
	BlockedRuntimeRejected  BlockedRuntimeStatus = "rejected"
	BlockedRuntimeConfirmed BlockedRuntimeStatus = "confirmed"
	BlockedRuntimeResumed   BlockedRuntimeStatus = "resumed"
	BlockedRuntimeAborted   BlockedRuntimeStatus = "aborted"
)

type BlockedRuntime struct {
	BlockedRuntimeID string                  `json:"blocked_runtime_id"`
	Kind             BlockedRuntimeKind      `json:"kind"`
	Status           BlockedRuntimeStatus    `json:"status"`
	WaitingFor       string                  `json:"waiting_for"`
	SessionID        string                  `json:"session_id"`
	TaskID           string                  `json:"task_id,omitempty"`
	StepID           string                  `json:"step_id,omitempty"`
	ActionID         string                  `json:"action_id,omitempty"`
	ApprovalID       string                  `json:"approval_id,omitempty"`
	AttemptID        string                  `json:"attempt_id,omitempty"`
	CycleID          string                  `json:"cycle_id,omitempty"`
	Target           TargetRef               `json:"target,omitempty"`
	Condition        BlockedRuntimeCondition `json:"condition,omitempty"`
	Metadata         map[string]any          `json:"metadata,omitempty"`
	Step             plan.StepSpec           `json:"step"`
	Approval         approval.Record         `json:"approval"`
	RuntimeHandles   []RuntimeHandle         `json:"runtime_handles,omitempty"`
	RequestedAt      int64                   `json:"requested_at"`
	UpdatedAt        int64                   `json:"updated_at,omitempty"`
}

type BlockedRuntimeConditionKind string

const (
	BlockedRuntimeConditionApproval     BlockedRuntimeConditionKind = "approval"
	BlockedRuntimeConditionConfirmation BlockedRuntimeConditionKind = "confirmation"
	BlockedRuntimeConditionExternal     BlockedRuntimeConditionKind = "external"
	BlockedRuntimeConditionInteractive  BlockedRuntimeConditionKind = "interactive"
)

type BlockedRuntimeSubject struct {
	StepID    string         `json:"step_id,omitempty"`
	ActionID  string         `json:"action_id,omitempty"`
	AttemptID string         `json:"attempt_id,omitempty"`
	CycleID   string         `json:"cycle_id,omitempty"`
	Target    TargetRef      `json:"target,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type BlockedRuntimeCondition struct {
	Kind        BlockedRuntimeConditionKind `json:"kind"`
	ReferenceID string                      `json:"reference_id,omitempty"`
	WaitingFor  string                      `json:"waiting_for,omitempty"`
	Metadata    map[string]any              `json:"metadata,omitempty"`
}

type BlockedRuntimeRecord struct {
	BlockedRuntimeID string                  `json:"blocked_runtime_id"`
	Kind             BlockedRuntimeKind      `json:"kind"`
	Status           BlockedRuntimeStatus    `json:"status"`
	SessionID        string                  `json:"session_id"`
	TaskID           string                  `json:"task_id,omitempty"`
	Subject          BlockedRuntimeSubject   `json:"subject,omitempty"`
	Condition        BlockedRuntimeCondition `json:"condition,omitempty"`
	Metadata         map[string]any          `json:"metadata,omitempty"`
	RequestedAt      int64                   `json:"requested_at,omitempty"`
	UpdatedAt        int64                   `json:"updated_at,omitempty"`
	ResolvedAt       int64                   `json:"resolved_at,omitempty"`
}

func (s BlockedRuntimeSubject) ReferencesAction() bool {
	return s.ActionID != ""
}

func (s BlockedRuntimeSubject) ReferencesTarget() bool {
	return s.Target.TargetID != ""
}

func (c BlockedRuntimeCondition) HasReference() bool {
	return c.ReferenceID != ""
}
