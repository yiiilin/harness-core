// Command minimal-agent shows the smallest in-process embedding of the harness kernel.
package main

import (
	"context"
	"fmt"

	"github.com/yiiilin/harness-core/pkg/harness"
	"github.com/yiiilin/harness-core/pkg/harness/builtins"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

func main() {
	// Compose the bare kernel with built-in tools/verifiers and a trivial planner.
	opts := harness.Options{}
	builtins.Register(&opts)
	rt := harness.New(opts).WithPlanner(hruntime.DemoPlanner{})

	// Create the minimum lifecycle objects the runtime expects.
	sess, err := rt.CreateSession("happy-path", "derive one shell step")
	if err != nil {
		panic(err)
	}
	tsk, err := rt.CreateTask(task.Spec{TaskType: "demo", Goal: "echo hello"})
	if err != nil {
		panic(err)
	}
	sess, err = rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		panic(err)
	}

	// Keep planning explicit so the example exposes the context and planner contracts directly.
	assembled, _ := rt.ContextAssembler.Assemble(context.Background(), sess, task.Spec{TaskID: tsk.TaskID, TaskType: tsk.TaskType, Goal: tsk.Goal, Constraints: tsk.Constraints, Metadata: tsk.Metadata})
	step, err := rt.Planner.PlanNext(context.Background(), sess, task.Spec{TaskID: tsk.TaskID, TaskType: tsk.TaskType, Goal: tsk.Goal, Constraints: tsk.Constraints, Metadata: tsk.Metadata}, assembled)
	if err != nil {
		panic(err)
	}
	pl, err := rt.CreatePlan(sess.SessionID, "planned by demo planner", []plan.StepSpec{step})
	if err != nil {
		panic(err)
	}
	out, err := rt.RunStep(context.Background(), sess.SessionID, pl.Steps[0])
	if err != nil {
		panic(err)
	}
	attempts, err := rt.ListAttempts(sess.SessionID)
	if err != nil {
		panic(err)
	}
	stdout, _ := out.Execution.Action.Data["stdout"].(string)

	fmt.Printf("planned step title: %s\n", step.Title)
	fmt.Printf("planned tool: %s\n", step.Action.ToolName)
	fmt.Printf("action stdout: %s\n", stdout)
	fmt.Printf("session phase: %s\n", out.Session.Phase)
	fmt.Printf("verify success: %v\n", out.Execution.Verify.Success)
	fmt.Printf("attempts: %d\n", len(attempts))
	fmt.Printf("metrics: %+v\n", rt.MetricsSnapshot())
}
