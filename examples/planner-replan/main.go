package main

import (
	"context"
	"fmt"

	"github.com/yiiilin/harness-core/pkg/harness"
	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type SequencePlanner struct{}

func (SequencePlanner) PlanNext(_ context.Context, state session.State, _ task.Spec, _ map[string]any) (plan.StepSpec, error) {
	switch state.CurrentStepID {
	case "":
		return plan.StepSpec{
			StepID: "step_alpha",
			Title:  "echo alpha",
			Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo alpha", "timeout_ms": 5000}},
			Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
				{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
				{Kind: "output_contains", Args: map[string]any{"text": "alpha"}},
			}},
			OnFail: plan.OnFailSpec{Strategy: "abort"},
		}, nil
	case "step_alpha":
		return plan.StepSpec{
			StepID: "step_beta",
			Title:  "echo beta",
			Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo beta", "timeout_ms": 5000}},
			Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
				{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
				{Kind: "output_contains", Args: map[string]any{"text": "beta"}},
			}},
			OnFail: plan.OnFailSpec{Strategy: "abort"},
		}, nil
	default:
		return plan.StepSpec{}, fmt.Errorf("no further steps for %s", state.CurrentStepID)
	}
}

func main() {
	opts := harness.Options{}
	harness.RegisterBuiltins(&opts)
	rt := harness.New(opts).WithPlanner(SequencePlanner{})

	sess := rt.CreateSession("planner replan", "derive and replan shell work")
	tsk := rt.CreateTask(task.Spec{TaskType: "demo", Goal: "run alpha then beta"})
	sess, _ = rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)

	initialPlan, _, err := rt.CreatePlanFromPlanner(context.Background(), sess.SessionID, "initial multi-step plan", 2)
	if err != nil {
		panic(err)
	}
	fmt.Printf("initial revision=%d steps=%d\n", initialPlan.Revision, len(initialPlan.Steps))

	firstRun, err := rt.RunStep(context.Background(), sess.SessionID, initialPlan.Steps[0])
	if err != nil {
		panic(err)
	}
	fmt.Printf("phase after first step=%s\n", firstRun.Session.Phase)

	replan, _, err := rt.CreatePlanFromPlanner(context.Background(), sess.SessionID, "replan after first step", 1)
	if err != nil {
		panic(err)
	}
	fmt.Printf("replan revision=%d steps=%d\n", replan.Revision, len(replan.Steps))

	secondRun, err := rt.RunStep(context.Background(), sess.SessionID, replan.Steps[0])
	if err != nil {
		panic(err)
	}
	fmt.Printf("phase after replanned step=%s\n", secondRun.Session.Phase)
}
