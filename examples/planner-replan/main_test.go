package main

import (
	"context"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

func TestSequencePlannerSupportsRevisionedReplan(t *testing.T) {
	opts := harness.Options{}
	harness.RegisterBuiltins(&opts)
	rt := harness.New(opts).WithPlanner(SequencePlanner{})

	sess, err := rt.CreateSession("planner replan", "derive and replan shell work")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	tsk, err := rt.CreateTask(task.Spec{TaskType: "demo", Goal: "run alpha then beta"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	sess, err = rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	initialPlan, _, err := rt.CreatePlanFromPlanner(context.Background(), sess.SessionID, "initial multi-step plan", 2)
	if err != nil {
		t.Fatalf("initial plan: %v", err)
	}
	if initialPlan.Revision != 1 || len(initialPlan.Steps) != 2 {
		t.Fatalf("unexpected initial plan: %#v", initialPlan)
	}

	firstRun, err := rt.RunStep(context.Background(), sess.SessionID, initialPlan.Steps[0])
	if err != nil {
		t.Fatalf("run first step: %v", err)
	}
	if firstRun.Session.Phase != session.PhasePlan {
		t.Fatalf("expected plan phase after first step, got %s", firstRun.Session.Phase)
	}

	replan, _, err := rt.CreatePlanFromPlanner(context.Background(), sess.SessionID, "replan after first step", 1)
	if err != nil {
		t.Fatalf("replan: %v", err)
	}
	if replan.Revision != 2 || len(replan.Steps) != 1 || replan.Steps[0].StepID != "step_beta" {
		t.Fatalf("unexpected replanned plan: %#v", replan)
	}
}
