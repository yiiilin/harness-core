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

func TestRunStepTimeoutPath(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	audits := audit.NewMemoryStore()

	tools.Register(tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true}, shellexec.PipeExecutor{})
	verifiers.Register(verify.Definition{Kind: "exit_code", Description: "Verify exit code."}, verify.ExitCodeChecker{})

	rt := hruntime.New(hruntime.Options{Sessions: sessions, Tasks: tasks, Plans: plans, Tools: tools, Verifiers: verifiers, Audit: audits})
	sess := rt.CreateSession("timeout session", "timeout path")
	tsk := rt.CreateTask(task.Spec{TaskType: "demo", Goal: "timeout path"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(attached.SessionID, "timeout", []plan.StepSpec{{
		StepID: "step_timeout",
		Title:  "sleep with short timeout",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "sleep 1", "timeout_ms": 10}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}}}},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	out, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	if out.Session.Phase != session.PhaseRecover {
		t.Fatalf("expected recover phase after timeout verification failure, got %s", out.Session.Phase)
	}
	if out.Execution.Action.OK {
		t.Fatalf("expected action not ok on timeout, got %#v", out.Execution.Action)
	}
	status, _ := out.Execution.Action.Data["status"].(string)
	if status != "timed_out" && status != "failed" {
		t.Fatalf("expected timed_out or failed status, got %q", status)
	}
}
