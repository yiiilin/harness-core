package plan

import (
	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type OnFailSpec struct {
	Strategy   string `json:"strategy"`
	MaxRetries int    `json:"max_retries,omitempty"`
	BackoffMS  int    `json:"backoff_ms,omitempty"`
}

type StepSpec struct {
	StepID   string         `json:"step_id"`
	Title    string         `json:"title"`
	Action   action.Spec    `json:"action"`
	Verify   verify.Spec    `json:"verify"`
	OnFail   OnFailSpec     `json:"on_fail,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type Spec struct {
	PlanID    string     `json:"plan_id"`
	SessionID string     `json:"session_id"`
	Revision  int        `json:"revision"`
	Status    string     `json:"status"`
	Steps     []StepSpec `json:"steps"`
}
