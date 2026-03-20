package runtime_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

func TestCreatePlanRejectsWhenRevisionBudgetExceeded(t *testing.T) {
	rt := hruntime.New(hruntime.Options{
		LoopBudgets: hruntime.LoopBudgets{
			MaxSteps:           8,
			MaxRetriesPerStep:  3,
			MaxPlanRevisions:   1,
			MaxTotalRuntimeMS:  60000,
			MaxToolOutputChars: 2048,
		},
	})

	sess := rt.CreateSession("plan revisions", "cap plan revisions")
	tsk := rt.CreateTask(task.Spec{TaskType: "demo", Goal: "enforce revision budget"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	if _, err := rt.CreatePlan(attached.SessionID, "initial", []plan.StepSpec{{StepID: "step_1", Title: "first"}}); err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := rt.CreatePlan(attached.SessionID, "revision 2", []plan.StepSpec{{StepID: "step_2", Title: "second"}}); !errors.Is(err, hruntime.ErrPlanRevisionBudgetExceeded) {
		t.Fatalf("expected ErrPlanRevisionBudgetExceeded, got %v", err)
	}
}

func TestRunStepRejectsWhenTotalRuntimeBudgetExceeded(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	audits := audit.NewMemoryStore()
	handler := &countingHandler{}

	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		handler,
	)

	rt := hruntime.New(hruntime.Options{
		Sessions: sessions,
		Tasks:    tasks,
		Plans:    plans,
		Tools:    tools,
		Audit:    audits,
		LoopBudgets: hruntime.LoopBudgets{
			MaxSteps:           8,
			MaxRetriesPerStep:  3,
			MaxPlanRevisions:   8,
			MaxTotalRuntimeMS:  1,
			MaxToolOutputChars: 2048,
		},
	})

	sess := rt.CreateSession("runtime budget", "reject stale sessions")
	tsk := rt.CreateTask(task.Spec{TaskType: "demo", Goal: "runtime budget"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	attached.CreatedAt = time.Now().Add(-time.Minute).UnixMilli()
	if err := sessions.Update(attached); err != nil {
		t.Fatalf("update session: %v", err)
	}
	pl, err := rt.CreatePlan(attached.SessionID, "initial", []plan.StepSpec{{
		StepID: "step_budget",
		Title:  "budgeted",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo late", "timeout_ms": 5000}},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	if _, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0]); !errors.Is(err, hruntime.ErrRuntimeBudgetExceeded) {
		t.Fatalf("expected ErrRuntimeBudgetExceeded, got %v", err)
	}
	if handler.calls != 0 {
		t.Fatalf("expected runtime budget to block tool execution, got %d calls", handler.calls)
	}
}

func TestRunStepAbortStrategyFailsSessionOnVerificationFailure(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	audits := audit.NewMemoryStore()
	handler := &countingHandler{}

	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		handler,
	)
	verifiers.Register(
		verify.Definition{Kind: "exit_code", Description: "Verify exit code."},
		verify.ExitCodeChecker{},
	)

	rt := hruntime.New(hruntime.Options{
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Tools:     tools,
		Verifiers: verifiers,
		Audit:     audits,
	})

	sess := rt.CreateSession("abort on fail", "abort verification failures")
	tsk := rt.CreateTask(task.Spec{TaskType: "demo", Goal: "abort failed verification"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(attached.SessionID, "initial", []plan.StepSpec{{
		StepID: "step_abort",
		Title:  "aborting verification failure",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo nope", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{1}}},
		}},
		OnFail: plan.OnFailSpec{Strategy: "abort"},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	out, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	if out.Session.Phase != session.PhaseFailed {
		t.Fatalf("expected abort strategy to fail the session, got %s", out.Session.Phase)
	}
}

func TestRunStepRetryBudgetBlocksFurtherAttempts(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	audits := audit.NewMemoryStore()
	handler := &countingHandler{}

	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		handler,
	)
	verifiers.Register(
		verify.Definition{Kind: "exit_code", Description: "Verify exit code."},
		verify.ExitCodeChecker{},
	)

	rt := hruntime.New(hruntime.Options{
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Tools:     tools,
		Verifiers: verifiers,
		Audit:     audits,
		LoopBudgets: hruntime.LoopBudgets{
			MaxSteps:           8,
			MaxRetriesPerStep:  1,
			MaxPlanRevisions:   8,
			MaxTotalRuntimeMS:  60000,
			MaxToolOutputChars: 2048,
		},
	})

	sess := rt.CreateSession("retry budget", "enforce retry limit")
	tsk := rt.CreateTask(task.Spec{TaskType: "demo", Goal: "enforce retry limit"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(attached.SessionID, "initial", []plan.StepSpec{{
		StepID: "step_retry",
		Title:  "retrying verification failure",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo retry", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{1}}},
		}},
		OnFail: plan.OnFailSpec{Strategy: "retry"},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	exhausted := pl.Steps[0]
	exhausted.Attempt = 2

	if _, err := rt.RunStep(context.Background(), attached.SessionID, exhausted); !errors.Is(err, hruntime.ErrStepRetryBudgetExceeded) {
		t.Fatalf("expected ErrStepRetryBudgetExceeded, got %v", err)
	}
	if handler.calls != 0 {
		t.Fatalf("expected retry budget to block tool invocation, got %d calls", handler.calls)
	}
}
