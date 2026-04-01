package runtime

import (
	"context"

	hcontextsummary "github.com/yiiilin/harness-core/pkg/harness/contextsummary"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

type LoopBudgets struct {
	MaxSteps          int   `json:"max_steps"`
	MaxRetriesPerStep int   `json:"max_retries_per_step"`
	MaxPlanRevisions  int   `json:"max_plan_revisions"`
	MaxTotalRuntimeMS int64 `json:"max_total_runtime_ms"`
}

type CompactionTrigger = hcontextsummary.Trigger

const (
	CompactionTriggerPlan    CompactionTrigger = "plan"
	CompactionTriggerExecute CompactionTrigger = "execute"
	CompactionTriggerRecover CompactionTrigger = "recover"
)

type CompactionPolicy struct {
	OnPlan    bool `json:"on_plan"`
	OnExecute bool `json:"on_execute"`
	OnRecover bool `json:"on_recover"`
}

func DefaultLoopBudgets() LoopBudgets {
	return LoopBudgets{
		MaxSteps:          8,
		MaxRetriesPerStep: 3,
		MaxPlanRevisions:  8,
		MaxTotalRuntimeMS: 300000,
	}
}

func DefaultCompactionPolicy() CompactionPolicy {
	return CompactionPolicy{
		OnPlan:    true,
		OnExecute: true,
		OnRecover: true,
	}
}

type ContextTask struct {
	TaskID   string `json:"task_id"`
	TaskType string `json:"task_type"`
	Goal     string `json:"goal"`
}

type ContextSession struct {
	SessionID      string                 `json:"session_id"`
	Phase          session.Phase          `json:"phase"`
	CurrentStepID  string                 `json:"current_step_id,omitempty"`
	RetryCount     int                    `json:"retry_count,omitempty"`
	ExecutionState session.ExecutionState `json:"execution_state,omitempty"`
}

type ContextCompaction struct {
	SummaryID         string            `json:"summary_id,omitempty"`
	PreviousSummaryID string            `json:"previous_summary_id,omitempty"`
	Trigger           CompactionTrigger `json:"trigger,omitempty"`
	Strategy          string            `json:"strategy,omitempty"`
	OriginalBytes     int               `json:"original_bytes,omitempty"`
	CompactedBytes    int               `json:"compacted_bytes,omitempty"`
	Metadata          map[string]any    `json:"metadata,omitempty"`
}

type ContextPackage struct {
	Task        ContextTask        `json:"task"`
	Session     ContextSession     `json:"session"`
	Constraints map[string]any     `json:"constraints,omitempty"`
	Metadata    map[string]any     `json:"metadata,omitempty"`
	Derived     map[string]any     `json:"derived,omitempty"`
	Compaction  *ContextCompaction `json:"compaction,omitempty"`
	Extras      map[string]any     `json:"extras,omitempty"`
}

func (pkg ContextPackage) ToMap() map[string]any {
	out := map[string]any{
		"task": map[string]any{
			"task_id":   pkg.Task.TaskID,
			"task_type": pkg.Task.TaskType,
			"goal":      pkg.Task.Goal,
		},
		"session": map[string]any{
			"session_id":      pkg.Session.SessionID,
			"phase":           pkg.Session.Phase,
			"current_step_id": pkg.Session.CurrentStepID,
			"retry_count":     pkg.Session.RetryCount,
			"execution_state": pkg.Session.ExecutionState,
		},
	}
	if len(pkg.Constraints) > 0 {
		out["constraints"] = pkg.Constraints
	}
	if len(pkg.Metadata) > 0 {
		out["metadata"] = pkg.Metadata
	}
	if len(pkg.Derived) > 0 {
		out["derived"] = pkg.Derived
	}
	if pkg.Compaction != nil {
		out["compaction"] = map[string]any{
			"summary_id":          pkg.Compaction.SummaryID,
			"previous_summary_id": pkg.Compaction.PreviousSummaryID,
			"trigger":             pkg.Compaction.Trigger,
			"strategy":            pkg.Compaction.Strategy,
			"original_bytes":      pkg.Compaction.OriginalBytes,
			"compacted_bytes":     pkg.Compaction.CompactedBytes,
			"metadata":            pkg.Compaction.Metadata,
		}
	}
	for key, value := range pkg.Extras {
		out[key] = value
	}
	return out
}

type ContextSummary = hcontextsummary.Summary

type ContextSummaryStore interface {
	Create(spec ContextSummary) (ContextSummary, error)
	Get(id string) (ContextSummary, error)
	List(sessionID string) ([]ContextSummary, error)
}

type Compactor interface {
	Compact(ctx context.Context, pkg ContextPackage, state session.State, spec task.Spec, budgets LoopBudgets) (ContextPackage, *ContextSummary, error)
}

type NoopCompactor struct{}

func (NoopCompactor) Compact(_ context.Context, pkg ContextPackage, _ session.State, _ task.Spec, _ LoopBudgets) (ContextPackage, *ContextSummary, error) {
	return pkg, nil, nil
}

var ErrContextSummaryNotFound = hcontextsummary.ErrContextSummaryNotFound

type MemoryContextSummaryStore = hcontextsummary.MemoryStore

func NewMemoryContextSummaryStore() *MemoryContextSummaryStore {
	return hcontextsummary.NewMemoryStore()
}
