package execution

import (
	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type AttemptStatus string

type ActionStatus string

type VerificationStatus string

type RuntimeHandleStatus string

const (
	AttemptBlocked   AttemptStatus = "blocked"
	AttemptCompleted AttemptStatus = "completed"
	AttemptFailed    AttemptStatus = "failed"

	ActionCompleted ActionStatus = "completed"
	ActionFailed    ActionStatus = "failed"

	VerificationCompleted VerificationStatus = "completed"
	VerificationFailed    VerificationStatus = "failed"

	RuntimeHandleActive      RuntimeHandleStatus = "active"
	RuntimeHandleClosed      RuntimeHandleStatus = "closed"
	RuntimeHandleInvalidated RuntimeHandleStatus = "invalidated"
)

type Attempt struct {
	AttemptID  string         `json:"attempt_id"`
	SessionID  string         `json:"session_id"`
	TaskID     string         `json:"task_id,omitempty"`
	StepID     string         `json:"step_id,omitempty"`
	ApprovalID string         `json:"approval_id,omitempty"`
	CycleID    string         `json:"cycle_id,omitempty"`
	TraceID    string         `json:"trace_id,omitempty"`
	Status     AttemptStatus  `json:"status"`
	Step       plan.StepSpec  `json:"step"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	StartedAt  int64          `json:"started_at"`
	FinishedAt int64          `json:"finished_at,omitempty"`
}

type ActionRecord struct {
	ActionID    string         `json:"action_id"`
	AttemptID   string         `json:"attempt_id"`
	SessionID   string         `json:"session_id"`
	TaskID      string         `json:"task_id,omitempty"`
	StepID      string         `json:"step_id,omitempty"`
	CycleID     string         `json:"cycle_id,omitempty"`
	ToolName    string         `json:"tool_name,omitempty"`
	TraceID     string         `json:"trace_id,omitempty"`
	CausationID string         `json:"causation_id,omitempty"`
	Status      ActionStatus   `json:"status"`
	Result      action.Result  `json:"result"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	StartedAt   int64          `json:"started_at"`
	FinishedAt  int64          `json:"finished_at,omitempty"`
}

type VerificationRecord struct {
	VerificationID string             `json:"verification_id"`
	AttemptID      string             `json:"attempt_id"`
	SessionID      string             `json:"session_id"`
	TaskID         string             `json:"task_id,omitempty"`
	StepID         string             `json:"step_id,omitempty"`
	ActionID       string             `json:"action_id,omitempty"`
	CycleID        string             `json:"cycle_id,omitempty"`
	TraceID        string             `json:"trace_id,omitempty"`
	CausationID    string             `json:"causation_id,omitempty"`
	Status         VerificationStatus `json:"status"`
	Spec           verify.Spec        `json:"spec"`
	Result         verify.Result      `json:"result"`
	Metadata       map[string]any     `json:"metadata,omitempty"`
	StartedAt      int64              `json:"started_at"`
	FinishedAt     int64              `json:"finished_at,omitempty"`
}

type Artifact struct {
	ArtifactID     string         `json:"artifact_id"`
	SessionID      string         `json:"session_id"`
	TaskID         string         `json:"task_id,omitempty"`
	StepID         string         `json:"step_id,omitempty"`
	AttemptID      string         `json:"attempt_id,omitempty"`
	ActionID       string         `json:"action_id,omitempty"`
	VerificationID string         `json:"verification_id,omitempty"`
	CycleID        string         `json:"cycle_id,omitempty"`
	TraceID        string         `json:"trace_id,omitempty"`
	Name           string         `json:"name,omitempty"`
	Kind           string         `json:"kind,omitempty"`
	Payload        map[string]any `json:"payload,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	CreatedAt      int64          `json:"created_at"`
}

type RuntimeHandle struct {
	HandleID      string              `json:"handle_id"`
	SessionID     string              `json:"session_id"`
	TaskID        string              `json:"task_id,omitempty"`
	AttemptID     string              `json:"attempt_id,omitempty"`
	CycleID       string              `json:"cycle_id,omitempty"`
	TraceID       string              `json:"trace_id,omitempty"`
	Kind          string              `json:"kind,omitempty"`
	Value         string              `json:"value,omitempty"`
	Status        RuntimeHandleStatus `json:"status"`
	StatusReason  string              `json:"status_reason,omitempty"`
	Metadata      map[string]any      `json:"metadata,omitempty"`
	Version       int64               `json:"version,omitempty"`
	CreatedAt     int64               `json:"created_at"`
	UpdatedAt     int64               `json:"updated_at"`
	ClosedAt      int64               `json:"closed_at,omitempty"`
	InvalidatedAt int64               `json:"invalidated_at,omitempty"`
}
