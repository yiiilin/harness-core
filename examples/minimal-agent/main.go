package main

import (
	"context"
	"fmt"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

func main() {
	opts := hruntime.Options{}
	hruntime.RegisterBuiltins(&opts)
	rt := hruntime.New(opts)

	sess := rt.CreateSession("happy-path", "Run one shell step")
	tsk := rt.CreateTask(task.Spec{TaskType: "demo", Goal: "execute one shell command and verify it"})
	sess, _ = rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	pl, _ := rt.CreatePlan(sess.SessionID, "initial", []plan.StepSpec{{
		StepID: "step_1",
		Title:  "echo hello",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo hello", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
			{Kind: "output_contains", Args: map[string]any{"text": "hello"}},
		}},
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
