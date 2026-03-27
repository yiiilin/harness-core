package runtime

import (
	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/capability"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/observability"
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/persistence"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/planning"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type Options struct {
	Sessions            session.Store
	Tasks               task.Store
	Plans               plan.Store
	Approvals           approval.Store
	Attempts            execution.AttemptStore
	Actions             execution.ActionStore
	Verifications       execution.VerificationStore
	Artifacts           execution.ArtifactStore
	BlockedRuntimes     execution.BlockedRuntimeStore
	RuntimeHandles      execution.RuntimeHandleStore
	CapabilitySnapshots capability.SnapshotStore
	PlanningRecords     planning.Store
	CapabilityFreezer   capability.Freezer
	ResumePolicy        approval.ResumePolicy
	Tools               *tool.Registry
	CapabilityResolver  capability.Resolver
	Verifiers           *verify.Registry
	Audit               audit.Store
	Runner              persistence.Runner
	Policy              permission.Evaluator
	ContextAssembler    ContextAssembler
	ContextSummaries    ContextSummaryStore
	Compactor           Compactor
	CompactionPolicy    CompactionPolicy
	LoopBudgets         LoopBudgets
	// LoopBudgetOverrides overlays ordinary non-zero budget fields.
	LoopBudgetOverrides *LoopBudgets
	// LoopBudgetMaxRetriesOverride preserves explicit retry overrides,
	// including zero, without overloading the zero value semantics of the
	// legacy LoopBudgets struct.
	LoopBudgetMaxRetriesOverride *int
	Planner                      Planner
	TargetResolver               TargetResolver
	AttachmentMaterializer       AttachmentMaterializer
	InteractiveController        InteractiveController
	EventSink                    EventSink
	Clock                        Clock
	Metrics                      Metrics
	MetricsExporter              MetricsExporter
	TraceExporter                TraceExporter
	MetricsRecorder              *observability.MemoryRecorder
	StorageMode                  string
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
	if opts.Approvals == nil {
		opts.Approvals = approval.NewMemoryStore()
	}
	if opts.Attempts == nil {
		opts.Attempts = execution.NewMemoryAttemptStore()
	}
	if opts.Actions == nil {
		opts.Actions = execution.NewMemoryActionStore()
	}
	if opts.Verifications == nil {
		opts.Verifications = execution.NewMemoryVerificationStore()
	}
	if opts.Artifacts == nil {
		opts.Artifacts = execution.NewMemoryArtifactStore()
	}
	if opts.BlockedRuntimes == nil {
		opts.BlockedRuntimes = execution.NewMemoryBlockedRuntimeStore()
	}
	if opts.RuntimeHandles == nil {
		opts.RuntimeHandles = execution.NewMemoryRuntimeHandleStore()
	}
	if opts.Tools == nil {
		opts.Tools = tool.NewRegistry()
	}
	if opts.CapabilitySnapshots == nil {
		opts.CapabilitySnapshots = capability.NewMemorySnapshotStore()
	}
	if opts.PlanningRecords == nil {
		opts.PlanningRecords = planning.NewMemoryStore()
	}
	if opts.CapabilityFreezer == nil {
		opts.CapabilityFreezer = capability.RegistryFreezer{Registry: opts.Tools}
	}
	if opts.ResumePolicy == nil {
		opts.ResumePolicy = approval.DefaultResumePolicy{}
	}
	if opts.CapabilityResolver == nil {
		opts.CapabilityResolver = capability.RegistryResolver{Registry: opts.Tools}
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
	if opts.ContextSummaries == nil {
		opts.ContextSummaries = NewMemoryContextSummaryStore()
	}
	if opts.Compactor == nil {
		opts.Compactor = NoopCompactor{}
	}
	if !opts.CompactionPolicy.OnPlan && !opts.CompactionPolicy.OnExecute && !opts.CompactionPolicy.OnRecover {
		opts.CompactionPolicy = DefaultCompactionPolicy()
	}
	if opts.Runner == nil {
		opts.Runner = persistence.NewMemoryUnitOfWork(persistence.RepositorySet{
			Sessions:            opts.Sessions,
			Tasks:               opts.Tasks,
			Plans:               opts.Plans,
			Audits:              opts.Audit,
			Attempts:            opts.Attempts,
			Actions:             opts.Actions,
			Verifications:       opts.Verifications,
			Artifacts:           opts.Artifacts,
			BlockedRuntimes:     opts.BlockedRuntimes,
			RuntimeHandles:      opts.RuntimeHandles,
			Approvals:           opts.Approvals,
			CapabilitySnapshots: opts.CapabilitySnapshots,
			ContextSummaries:    opts.ContextSummaries,
			PlanningRecords:     opts.PlanningRecords,
		})
	}
	opts.LoopBudgets = normalizeLoopBudgets(opts.LoopBudgets, opts.LoopBudgetOverrides, opts.LoopBudgetMaxRetriesOverride)
	opts.LoopBudgetOverrides = nil
	opts.LoopBudgetMaxRetriesOverride = nil
	if opts.Planner == nil {
		opts.Planner = NoopPlanner{}
	}
	if opts.AttachmentMaterializer == nil {
		opts.AttachmentMaterializer = LocalTempFileMaterializer{}
	}
	if opts.Clock == nil {
		opts.Clock = systemClock{}
	}
	opts.EventSink = bindEventSinkToAuditStore(opts.EventSink, opts.Audit)
	if opts.MetricsRecorder == nil {
		opts.MetricsRecorder = observability.NewMemoryRecorder()
	}
	if opts.Metrics == nil {
		opts.Metrics = opts.MetricsRecorder
	}
	if opts.StorageMode == "" {
		opts.StorageMode = "in-memory-dev"
	}
	return opts
}

func normalizeLoopBudgets(budgets LoopBudgets, override *LoopBudgets, maxRetriesOverride *int) LoopBudgets {
	legacyRetriesExplicit := legacyRetryBudgetExplicit(budgets)
	explicitRetries := legacyRetriesExplicit
	if override != nil {
		if override.MaxRetriesPerStep > 0 {
			explicitRetries = true
		}
		budgets = mergeLoopBudgets(budgets, *override)
	}
	if maxRetriesOverride != nil {
		explicitRetries = true
		budgets.MaxRetriesPerStep = *maxRetriesOverride
	}
	if budgets.MaxSteps <= 0 || budgets.MaxRetriesPerStep < 0 || (!explicitRetries && budgets.MaxRetriesPerStep == 0) || budgets.MaxPlanRevisions <= 0 || budgets.MaxTotalRuntimeMS <= 0 || budgets.MaxToolOutputChars <= 0 {
		defaults := DefaultLoopBudgets()
		if budgets.MaxSteps <= 0 {
			budgets.MaxSteps = defaults.MaxSteps
		}
		if budgets.MaxRetriesPerStep < 0 || (!explicitRetries && budgets.MaxRetriesPerStep == 0) {
			budgets.MaxRetriesPerStep = defaults.MaxRetriesPerStep
		}
		if budgets.MaxPlanRevisions <= 0 {
			budgets.MaxPlanRevisions = defaults.MaxPlanRevisions
		}
		if budgets.MaxTotalRuntimeMS <= 0 {
			budgets.MaxTotalRuntimeMS = defaults.MaxTotalRuntimeMS
		}
		if budgets.MaxToolOutputChars <= 0 {
			budgets.MaxToolOutputChars = defaults.MaxToolOutputChars
		}
	}
	return budgets
}

func legacyRetryBudgetExplicit(budgets LoopBudgets) bool {
	if budgets.MaxRetriesPerStep != 0 {
		return true
	}
	defaults := DefaultLoopBudgets()
	return budgets.MaxSteps == defaults.MaxSteps &&
		budgets.MaxPlanRevisions == defaults.MaxPlanRevisions &&
		budgets.MaxTotalRuntimeMS == defaults.MaxTotalRuntimeMS &&
		budgets.MaxToolOutputChars == defaults.MaxToolOutputChars
}

func mergeLoopBudgets(base, override LoopBudgets) LoopBudgets {
	if override.MaxSteps > 0 {
		base.MaxSteps = override.MaxSteps
	}
	if override.MaxRetriesPerStep > 0 {
		base.MaxRetriesPerStep = override.MaxRetriesPerStep
	}
	if override.MaxPlanRevisions > 0 {
		base.MaxPlanRevisions = override.MaxPlanRevisions
	}
	if override.MaxTotalRuntimeMS > 0 {
		base.MaxTotalRuntimeMS = override.MaxTotalRuntimeMS
	}
	if override.MaxToolOutputChars > 0 {
		base.MaxToolOutputChars = override.MaxToolOutputChars
	}
	return base
}
