package runtime_test

import (
	"context"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	shellexec "github.com/yiiilin/harness-core/pkg/harness/executor/shell"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

func TestHappyPathRunStep(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	audits := audit.NewMemoryStore()

	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		shellexec.PipeExecutor{},
	)

	verifiers.Register(
		verify.Definition{Kind: "exit_code", Description: "Verify that an execution result exit code is in the allowed set."},
		verify.ExitCodeChecker{},
	)
	verifiers.Register(
		verify.Definition{Kind: "output_contains", Description: "Verify that stdout or stderr contains a target substring."},
		verify.OutputContainsChecker{},
	)

	rt := hruntime.New(hruntime.Options{Sessions: sessions, Tasks: tasks, Plans: plans, Tools: tools, Verifiers: verifiers, Audit: audits})

	sess := mustCreateSession(t, rt, "test session", "run a shell command")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "execute one verified shell step"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(attached.SessionID, "initial", []plan.StepSpec{{
		StepID: "step_1",
		Title:  "echo hello",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo hello", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
			{Kind: "output_contains", Args: map[string]any{"text": "hello"}},
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

	if out.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected session phase complete, got %s", out.Session.Phase)
	}
	if !out.Execution.Action.OK {
		t.Fatalf("expected action ok, got %#v", out.Execution.Action)
	}
	if !out.Execution.Verify.Success {
		t.Fatalf("expected verify success, got %#v", out.Execution.Verify)
	}
	if out.Execution.Step.Status != plan.StepCompleted {
		t.Fatalf("expected step completed, got %s", out.Execution.Step.Status)
	}
	if out.UpdatedTask == nil || out.UpdatedTask.Status != task.StatusCompleted {
		t.Fatalf("expected task completed, got %#v", out.UpdatedTask)
	}
	if out.UpdatedPlan == nil || out.UpdatedPlan.Status != plan.StatusCompleted {
		t.Fatalf("expected plan completed, got %#v", out.UpdatedPlan)
	}
	if len(out.Events) == 0 {
		t.Fatalf("expected runtime events, got none")
	}
	stored := mustListAuditEvents(t, rt, attached.SessionID)
	if len(stored) == 0 {
		t.Fatalf("expected stored audit events, got none")
	}
}

func TestRunStepActionFailureFailsEvenWithoutVerifyChecks(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	audits := audit.NewMemoryStore()

	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		failingHandler{},
	)

	rt := hruntime.New(hruntime.Options{Sessions: sessions, Tasks: tasks, Plans: plans, Tools: tools, Verifiers: verifiers, Audit: audits})

	sess := mustCreateSession(t, rt, "failed action", "action failure should fail the step even without verify checks")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "fail a step without verify checks"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(attached.SessionID, "action failure", []plan.StepSpec{{
		StepID: "step_fail_action_no_verify",
		Title:  "simulated action failure without verify",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "false"}},
		Verify: verify.Spec{},
		OnFail: plan.OnFailSpec{Strategy: "abort"},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	out, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}

	if out.Execution.Action.Error == nil || out.Execution.Action.Error.Code != "SIMULATED_FAILURE" {
		t.Fatalf("expected simulated action failure, got %#v", out.Execution.Action)
	}
	if out.Execution.Step.Status != plan.StepFailed {
		t.Fatalf("expected failed step after action failure without verify checks, got %#v", out.Execution.Step)
	}
	if out.Session.Phase != session.PhaseFailed {
		t.Fatalf("expected failed session after action failure without verify checks, got %#v", out.Session)
	}
	if out.UpdatedTask == nil || out.UpdatedTask.Status != task.StatusFailed {
		t.Fatalf("expected failed task after action failure without verify checks, got %#v", out.UpdatedTask)
	}
	if out.UpdatedPlan == nil || out.UpdatedPlan.Status != plan.StatusFailed {
		t.Fatalf("expected failed plan after action failure without verify checks, got %#v", out.UpdatedPlan)
	}
}
