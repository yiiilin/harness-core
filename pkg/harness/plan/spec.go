package plan

import (
	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type Status string

type StepStatus string

const (
	StatusDraft      Status = "draft"
	StatusActive     Status = "active"
	StatusSuperseded Status = "superseded"
	StatusCompleted  Status = "completed"
	StatusFailed     Status = "failed"

	StepPending   StepStatus = "pending"
	StepRunning   StepStatus = "running"
	StepBlocked   StepStatus = "blocked"
	StepCompleted StepStatus = "completed"
	StepFailed    StepStatus = "failed"
	StepSkipped   StepStatus = "skipped"
)

type OnFailSpec struct {
	Strategy   string `json:"strategy"`
	MaxRetries int    `json:"max_retries,omitempty"`
	BackoffMS  int    `json:"backoff_ms,omitempty"`
}

type StepSpec struct {
	PlanID       string         `json:"plan_id,omitempty"`
	PlanRevision int            `json:"plan_revision,omitempty"`
	StepID       string         `json:"step_id"`
	Title        string         `json:"title"`
	Action       action.Spec    `json:"action"`
	Verify       verify.Spec    `json:"verify"`
	OnFail       OnFailSpec     `json:"on_fail,omitempty"`
	Status       StepStatus     `json:"status"`
	Attempt      int            `json:"attempt,omitempty"`
	Reason       string         `json:"reason,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	StartedAt    int64          `json:"started_at,omitempty"`
	FinishedAt   int64          `json:"finished_at,omitempty"`
}

type Spec struct {
	PlanID       string     `json:"plan_id"`
	SessionID    string     `json:"session_id"`
	Revision     int        `json:"revision"`
	Status       Status     `json:"status"`
	ChangeReason string     `json:"change_reason,omitempty"`
	Steps        []StepSpec `json:"steps"`
	CreatedAt    int64      `json:"created_at,omitempty"`
	UpdatedAt    int64      `json:"updated_at,omitempty"`
}
