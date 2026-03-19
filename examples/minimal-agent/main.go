package main

import (
	"context"
	"fmt"

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

type shellHandler struct{ exec shellexec.PipeExecutor }

func (h shellHandler) Invoke(ctx context.Context, args map[string]any) (action.Result, error) {
	return h.exec.Invoke(ctx, args)
}

func main() {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	audits := audit.NewMemoryStore()

	tools.Register(tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true}, shellHandler{exec: shellexec.PipeExecutor{}})
	verifiers.Register(verify.Definition{Kind: "exit_code", Description: "Verify that an execution result exit code is in the allowed set."}, verify.ExitCodeChecker{})
	verifiers.Register(verify.Definition{Kind: "output_contains", Description: "Verify that stdout or stderr contains a target substring."}, verify.OutputContainsChecker{})

	rt := hruntime.New(sessions, tasks, plans, tools, verifiers, audits)
	sess := rt.CreateSession("happy-path", "Run one shell step")
	tsk := rt.CreateTask(task.Spec{TaskType: "demo", Goal: "execute one shell command and verify it"})
	sess, _ = rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	pl, _ := rt.CreatePlan(sess.SessionID, "initial", []plan.StepSpec{{
		StepID: "step_1",
		Title:  "echo hello",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo hello", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}}, {Kind: "output_contains", Args: map[string]any{"text": "hello"}}}},
		OnFail: plan.OnFailSpec{Strategy: "abort"},
	}})
	out, err := rt.RunStep(context.Background(), sess.SessionID, pl.Steps[0])
	if err != nil {
		panic(err)
	}
	fmt.Printf("session phase: %s\n", out.Session.Phase)
	fmt.Printf("step status: %s\n", out.Execution.Step.Status)
	fmt.Printf("verify success: %v\n", out.Execution.Verify.Success)
	fmt.Printf("events: %d\n", len(out.Events))
}
