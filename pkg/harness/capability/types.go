package capability

import (
	"context"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
)

type Request struct {
	SessionID    string               `json:"session_id,omitempty"`
	TaskID       string               `json:"task_id,omitempty"`
	StepID       string               `json:"step_id,omitempty"`
	Action       action.Spec          `json:"action"`
	Requirements *SupportRequirements `json:"requirements,omitempty"`
}

type SnapshotScope string

const (
	SnapshotScopePlan   SnapshotScope = "plan"
	SnapshotScopeAction SnapshotScope = "action"
)

type Snapshot struct {
	SnapshotID     string         `json:"snapshot_id"`
	SessionID      string         `json:"session_id,omitempty"`
	TaskID         string         `json:"task_id,omitempty"`
	PlanID         string         `json:"plan_id,omitempty"`
	StepID         string         `json:"step_id,omitempty"`
	ViewID         string         `json:"view_id,omitempty"`
	Scope          SnapshotScope  `json:"scope,omitempty"`
	ToolName       string         `json:"tool_name"`
	Version        string         `json:"version,omitempty"`
	CapabilityType string         `json:"capability_type,omitempty"`
	RiskLevel      tool.RiskLevel `json:"risk_level,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	ResolvedAt     int64          `json:"resolved_at"`
}

type View struct {
	ViewID    string     `json:"view_id"`
	SessionID string     `json:"session_id,omitempty"`
	TaskID    string     `json:"task_id,omitempty"`
	Entries   []Snapshot `json:"entries,omitempty"`
	FrozenAt  int64      `json:"frozen_at"`
}

type Resolution struct {
	Snapshot   Snapshot        `json:"snapshot"`
	Definition tool.Definition `json:"definition"`
	Handler    tool.Handler    `json:"-"`
}

type Resolver interface {
	Resolve(ctx context.Context, req Request) (Resolution, error)
}

type Matcher interface {
	Match(ctx context.Context, req Request) (MatchResult, error)
}

type Freezer interface {
	Freeze(ctx context.Context, sessionID, taskID string) (View, error)
}
