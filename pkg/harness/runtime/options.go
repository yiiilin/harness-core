package runtime

import (
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/observability"
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type Options struct {
	Sessions         session.Store
	Tasks            task.Store
	Plans            plan.Store
	Tools            *tool.Registry
	Verifiers        *verify.Registry
	Audit            audit.Store
	Policy           permission.Evaluator
	ContextAssembler ContextAssembler
	Planner          Planner
	EventSink        EventSink
	Metrics          Metrics
	MetricsRecorder  *observability.MemoryRecorder
}

func WithDefaults(opts Options) Options {
	if opts.Sessions == nil {
		opts.Sessions = session.NewMemoryStore()
	}
	if opts.Tasks == nil {
		opts.Tasks = task.NewMemoryStore()
	}
	if opts.Plans == nil {
		opts.Plans = plan.NewMemoryStore()
	}
	if opts.Tools == nil {
		opts.Tools = tool.NewRegistry()
	}
	if opts.Verifiers == nil {
		opts.Verifiers = verify.NewRegistry()
	}
	if opts.Audit == nil {
		opts.Audit = audit.NewMemoryStore()
	}
	if opts.Policy == nil {
		opts.Policy = permission.DefaultEvaluator{}
	}
	if opts.ContextAssembler == nil {
		opts.ContextAssembler = DefaultContextAssembler{}
	}
	if opts.Planner == nil {
		opts.Planner = NoopPlanner{}
	}
	if opts.EventSink == nil {
		opts.EventSink = AuditStoreSink{Store: opts.Audit}
	}
	if opts.MetricsRecorder == nil {
		opts.MetricsRecorder = observability.NewMemoryRecorder()
	}
	if opts.Metrics == nil {
		opts.Metrics = opts.MetricsRecorder
	}
	return opts
}
