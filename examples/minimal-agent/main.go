package main

import (
	"context"
	"fmt"

	"github.com/yiiilin/harness-core/pkg/harness"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

func main() {
	opts := harness.Options{}
	harness.RegisterBuiltins(&opts)
	rt := harness.New(opts).WithPlanner(hruntime.DemoPlanner{})

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

	fmt.Printf("planned step title: %s\n", step.Title)
	fmt.Printf("session phase: %s\n", out.Session.Phase)
	fmt.Printf("verify success: %v\n", out.Execution.Verify.Success)
	fmt.Printf("metrics: %+v\n", rt.MetricsSnapshot())
}
