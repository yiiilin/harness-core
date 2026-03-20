package runtime

import (
	"context"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
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
