package runtime_test

import (
	"context"
	"errors"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

type budgetedSequencePlanner struct{}

func (budgetedSequencePlanner) PlanNext(_ context.Context, state session.State, _ task.Spec, _ hruntime.ContextPackage) (plan.StepSpec, error) {
	switch state.CurrentStepID {
	case "":
		return plan.StepSpec{
			StepID: "step_alpha",
			Title:  "alpha",
			Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"command": "echo alpha"}},
		}, nil
	case "step_alpha":
		return plan.StepSpec{
			StepID: "step_beta",
			Title:  "beta",
			Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"command": "echo beta"}},
		}, nil
	case "step_beta":
		return plan.StepSpec{
			StepID: "step_gamma",
			Title:  "gamma",
			Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"command": "echo gamma"}},
		}, nil
	default:
		return plan.StepSpec{}, errors.New("no more steps")
	}
}

type recordingCompactor struct {
	calls int
}

func (c *recordingCompactor) Compact(_ context.Context, pkg hruntime.ContextPackage, state session.State, spec task.Spec, budgets hruntime.LoopBudgets) (hruntime.ContextPackage, *hruntime.ContextSummary, error) {
	c.calls++
	if pkg.Derived == nil {
		pkg.Derived = map[string]any{}
	}
	pkg.Derived["compacted"] = true
	return pkg, &hruntime.ContextSummary{
		SessionID:      state.SessionID,
		TaskID:         spec.TaskID,
		Strategy:       "truncate",
		OriginalBytes:  128,
		CompactedBytes: 64,
		Summary:        map[string]any{"goal": spec.Goal},
	}, nil
}

func TestWithDefaultsSetsLoopBudgetAndCompactionDefaults(t *testing.T) {
	opts := hruntime.WithDefaults(hruntime.Options{})

	if opts.LoopBudgets.MaxSteps <= 0 {
		t.Fatalf("expected positive MaxSteps default, got %#v", opts.LoopBudgets)
	}
	if opts.LoopBudgets.MaxRetriesPerStep <= 0 {
		t.Fatalf("expected positive MaxRetriesPerStep default, got %#v", opts.LoopBudgets)
	}
	if opts.LoopBudgets.MaxPlanRevisions <= 0 {
		t.Fatalf("expected positive MaxPlanRevisions default, got %#v", opts.LoopBudgets)
	}
	if opts.LoopBudgets.MaxTotalRuntimeMS <= 0 {
		t.Fatalf("expected positive MaxTotalRuntimeMS default, got %#v", opts.LoopBudgets)
	}
	if opts.LoopBudgets.MaxToolOutputChars <= 0 {
		t.Fatalf("expected positive MaxToolOutputChars default, got %#v", opts.LoopBudgets)
	}
	if opts.Compactor == nil {
		t.Fatalf("expected default compactor")
	}
	if opts.ContextSummaries == nil {
		t.Fatalf("expected default context summary store")
	}
}

func TestAssembleContextForSessionAppliesCompactorAndPersistsSummary(t *testing.T) {
	compactor := &recordingCompactor{}
	summaries := hruntime.NewMemoryContextSummaryStore()
	rt := hruntime.New(hruntime.Options{
		Compactor:        compactor,
		ContextSummaries: summaries,
	})

	sess := rt.CreateSession("compaction", "compact planner context")
	tsk := rt.CreateTask(task.Spec{
		TaskType: "demo",
		Goal:     "compact the planner context",
		Metadata: map[string]any{"notes": "this metadata should go through the compactor hook"},
	})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	assembled, state, spec, err := rt.AssembleContextForSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("assemble context: %v", err)
	}

	if compactor.calls != 1 {
		t.Fatalf("expected compactor to be called once, got %d", compactor.calls)
	}
	if state.SessionID != attached.SessionID {
		t.Fatalf("expected assembled state for %s, got %#v", attached.SessionID, state)
	}
	if spec.TaskID != tsk.TaskID {
		t.Fatalf("expected assembled task %s, got %#v", tsk.TaskID, spec)
	}
	if assembled.Compaction == nil || assembled.Compaction.SummaryID == "" {
		t.Fatalf("expected compaction metadata with persisted summary id, got %#v", assembled.Compaction)
	}
	if compacted, _ := assembled.Derived["compacted"].(bool); !compacted {
		t.Fatalf("expected compactor to annotate derived context, got %#v", assembled.Derived)
	}

	items := summaries.List(attached.SessionID)
	if len(items) != 1 {
		t.Fatalf("expected one persisted summary, got %#v", items)
	}
	if items[0].SessionID != attached.SessionID || items[0].TaskID != tsk.TaskID || items[0].Strategy != "truncate" {
		t.Fatalf("unexpected persisted summary: %#v", items[0])
	}
}

func TestCreatePlanFromPlannerUsesConfiguredLoopBudgetWhenMaxStepsOmitted(t *testing.T) {
	rt := hruntime.New(hruntime.Options{
		LoopBudgets: hruntime.LoopBudgets{
			MaxSteps:           2,
			MaxRetriesPerStep:  2,
			MaxPlanRevisions:   2,
			MaxTotalRuntimeMS:  60000,
			MaxToolOutputChars: 2048,
		},
	}).WithPlanner(budgetedSequencePlanner{})

	sess := rt.CreateSession("budgeted planner", "respect runtime max_steps")
	tsk := rt.CreateTask(task.Spec{TaskType: "demo", Goal: "generate three planner steps"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, assembled, err := rt.CreatePlanFromPlanner(context.Background(), attached.SessionID, "budgeted plan", 0)
	if err != nil {
		t.Fatalf("create plan from planner: %v", err)
	}

	if len(pl.Steps) != 2 {
		t.Fatalf("expected planner to be capped to 2 steps by runtime budget, got %#v", pl.Steps)
	}
	if assembled.Task.TaskID != tsk.TaskID {
		t.Fatalf("expected typed context package to expose task info, got %#v", assembled)
	}
}
