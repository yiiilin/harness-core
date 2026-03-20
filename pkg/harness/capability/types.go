package capability

import (
	"context"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
)

type Request struct {
	SessionID string      `json:"session_id,omitempty"`
	TaskID    string      `json:"task_id,omitempty"`
	StepID    string      `json:"step_id,omitempty"`
	Action    action.Spec `json:"action"`
}

type Snapshot struct {
	SnapshotID     string         `json:"snapshot_id"`
	SessionID      string         `json:"session_id,omitempty"`
	TaskID         string         `json:"task_id,omitempty"`
	StepID         string         `json:"step_id,omitempty"`
	ToolName       string         `json:"tool_name"`
	Version        string         `json:"version,omitempty"`
	CapabilityType string         `json:"capability_type,omitempty"`
	RiskLevel      tool.RiskLevel `json:"risk_level,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	ResolvedAt     int64          `json:"resolved_at"`
}

type Resolution struct {
	Snapshot   Snapshot        `json:"snapshot"`
	Definition tool.Definition `json:"definition"`
	Handler    tool.Handler    `json:"-"`
}

type Resolver interface {
	Resolve(ctx context.Context, req Request) (Resolution, error)
}
