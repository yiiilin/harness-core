package harness

import (
	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

// Core constructor and options.
type Options = hruntime.Options
type Service = hruntime.Service

// Common domain types.
type TaskSpec = task.Spec
type TaskRecord = task.Record
type TaskStatus = task.Status

type SessionState = session.State
type SessionPhase = session.Phase

type PlanSpec = plan.Spec
type PlanStatus = plan.Status

type StepSpec = plan.StepSpec
type StepStatus = plan.StepStatus

type ActionSpec = action.Spec
type ActionResult = action.Result

type VerifySpec = verify.Spec
type VerifyResult = verify.Result

type ToolDefinition = tool.Definition
type ToolRiskLevel = tool.RiskLevel

type AuditEvent = audit.Event

type PermissionDecision = permission.Decision
type PermissionAction = permission.Action

type ContextAssembler = hruntime.ContextAssembler
type Planner = hruntime.Planner
type EventSink = hruntime.EventSink

type PolicyEvaluator = permission.Evaluator

const (
	RiskLow    = tool.RiskLow
	RiskMedium = tool.RiskMedium
	RiskHigh   = tool.RiskHigh

	Allow = permission.Allow
	Ask   = permission.Ask
	Deny  = permission.Deny
)

// New constructs a runtime service with defaults applied.
func New(opts Options) *Service {
	return hruntime.New(opts)
}

// NewDefault constructs a runtime using default in-memory components.
func NewDefault() *Service {
	return hruntime.New(Options{})
}

// NewWithBuiltins constructs a runtime with default in-memory components and built-in tools/verifiers.
func NewWithBuiltins() *Service {
	opts := Options{}
	RegisterBuiltins(&opts)
	return hruntime.New(opts)
}

// RegisterBuiltins wires the default built-in tools and verifiers into options.
func RegisterBuiltins(opts *Options) {
	hruntime.RegisterBuiltins(opts)
}
