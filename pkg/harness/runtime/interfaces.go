package runtime

import (
	"context"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/observability"
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

// ContextAssembler is responsible for producing the minimal sufficient context
// for the current step decision.
type ContextAssembler interface {
	Assemble(ctx context.Context, state session.State, spec task.Spec) (ContextPackage, error)
}

// Planner decides the next step or plan revision based on the current session state.
type Planner interface {
	PlanNext(ctx context.Context, state session.State, spec task.Spec, assembled ContextPackage) (plan.StepSpec, error)
}

// PlannerProjector controls how raw assembled runtime context is projected into
// planner-facing context when the runtime is configured for custom projection.
type PlannerProjector interface {
	ProjectPlannerContext(ctx context.Context, assembled ContextPackage, state session.State, spec task.Spec, policy PlannerPolicy) (ContextPackage, error)
}

// TargetResolver discovers concrete execution targets for transport-neutral
// multi-target program nodes such as fanout_all.
type TargetResolver interface {
	ResolveTargets(ctx context.Context, state session.State, spec task.Record, program execution.Program, node execution.ProgramNode) ([]execution.Target, error)
}

// AttachmentMaterializeRequest describes a transport-neutral attachment
// materialization request for program input bindings.
type AttachmentMaterializeRequest struct {
	SessionID string                    `json:"session_id,omitempty"`
	Step      plan.StepSpec             `json:"step"`
	Input     execution.AttachmentInput `json:"input"`
	Artifact  *execution.Artifact       `json:"artifact,omitempty"`
}

// AttachmentMaterializer turns typed attachment inputs plus an explicit
// materialization hint into concrete runtime values such as temp-file paths or
// embedder-defined opaque handles.
type AttachmentMaterializer interface {
	Materialize(ctx context.Context, request AttachmentMaterializeRequest) (any, error)
}

// InteractiveController owns transport-neutral interactive runtime lifecycle
// operations while leaving backend-specific semantics to companion modules or
// embedders.
type InteractiveController interface {
	StartInteractive(ctx context.Context, request InteractiveStartRequest) (InteractiveStartResult, error)
	ReopenInteractive(ctx context.Context, handle execution.RuntimeHandle, request InteractiveReopenRequest) (InteractiveReopenResult, error)
	ViewInteractive(ctx context.Context, handle execution.RuntimeHandle, request InteractiveViewRequest) (InteractiveViewResult, error)
	WriteInteractive(ctx context.Context, handle execution.RuntimeHandle, request InteractiveWriteRequest) (InteractiveWriteResult, error)
	CloseInteractive(ctx context.Context, handle execution.RuntimeHandle, request InteractiveCloseRequest) (InteractiveCloseResult, error)
}

// ToolInvoker executes a tool action.
type ToolInvoker interface {
	Invoke(ctx context.Context, step plan.StepSpec) (action.Result, error)
}

// Verifier evaluates postconditions for a step result.
type Verifier interface {
	Verify(ctx context.Context, spec verify.Spec, result action.Result, state session.State) (verify.Result, error)
}

// PolicyEvaluator decides whether an action may run, needs approval, or is denied.
type PolicyEvaluator interface {
	Evaluate(ctx context.Context, state session.State, step plan.StepSpec) (permission.Decision, error)
}

// EventSink records runtime and audit events.
type EventSink interface {
	Emit(ctx context.Context, event audit.Event) error
}

// MetricsExporter ships vendor-neutral metric samples out of the kernel.
type MetricsExporter interface {
	ExportMetric(ctx context.Context, sample observability.MetricSample) error
}

// TraceExporter ships vendor-neutral trace spans out of the kernel.
type TraceExporter interface {
	ExportTrace(ctx context.Context, span observability.TraceSpan) error
}
