package runtime_test

import (
	"context"
	"errors"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/builtins"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type sequencePlanner struct{}

func (sequencePlanner) PlanNext(_ context.Context, state session.State, _ task.Spec, _ hruntime.ContextPackage) (plan.StepSpec, error) {
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
		return plan.StepSpec{}, errors.New("no more steps")
	}
}

type failingPlanner struct{}

func (failingPlanner) PlanNext(_ context.Context, _ session.State, _ task.Spec, _ hruntime.ContextPackage) (plan.StepSpec, error) {
	return plan.StepSpec{}, errors.New("planner failed")
}

func TestCreatePlanFromPlannerBuildsMultiStepPlanAndRunsToCompletion(t *testing.T) {
	opts := hruntime.Options{}
	builtins.Register(&opts)
	rt := hruntime.New(withExplicitPlannerProjection(opts)).WithPlanner(sequencePlanner{})

	sess := mustCreateSession(t, rt, "planner integration", "execute planner-derived sequence")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "run two planned shell steps"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, _, err := rt.CreatePlanFromPlanner(context.Background(), sess.SessionID, "planner derived", 2)
	if err != nil {
		t.Fatalf("create plan from planner: %v", err)
	}
	if len(pl.Steps) != 2 {
		t.Fatalf("expected 2 planned steps, got %#v", pl.Steps)
	}

	out1, err := rt.RunStep(context.Background(), sess.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step 1: %v", err)
	}
	if out1.Session.Phase != session.PhasePlan {
		t.Fatalf("expected session to return to plan after first step, got %s", out1.Session.Phase)
	}
	if out1.UpdatedPlan == nil || out1.UpdatedPlan.Status != plan.StatusActive {
		t.Fatalf("expected active plan after first step, got %#v", out1.UpdatedPlan)
	}

	out2, err := rt.RunStep(context.Background(), sess.SessionID, pl.Steps[1])
	if err != nil {
		t.Fatalf("run step 2: %v", err)
	}
	if out2.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected complete session after second step, got %s", out2.Session.Phase)
	}
	if out2.UpdatedPlan == nil || out2.UpdatedPlan.Status != plan.StatusCompleted {
		t.Fatalf("expected completed plan after second step, got %#v", out2.UpdatedPlan)
	}
}

func TestCreatePlanFromPlannerFailurePath(t *testing.T) {
	opts := hruntime.Options{}
	builtins.Register(&opts)
	rt := hruntime.New(withExplicitPlannerProjection(opts)).WithPlanner(failingPlanner{})

	sess := mustCreateSession(t, rt, "planner failure", "planner failure path")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "planner should fail"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	if _, _, err := rt.CreatePlanFromPlanner(context.Background(), sess.SessionID, "planner failure", 2); err == nil {
		t.Fatalf("expected planner failure")
	}
}

func TestRunSessionDrivesPlannerDerivedPlanToCompletion(t *testing.T) {
	opts := hruntime.Options{}
	builtins.Register(&opts)
	rt := hruntime.New(withExplicitPlannerProjection(opts)).WithPlanner(sequencePlanner{})

	sess := mustCreateSession(t, rt, "planner session driver", "runtime should drive planner-derived steps")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "planner-driven session execution"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	out, err := rt.RunSession(context.Background(), sess.SessionID)
	if err != nil {
		t.Fatalf("run session: %v", err)
	}
	if out.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected session driver to complete the session, got %#v", out.Session)
	}
	if len(out.Executions) != 2 {
		t.Fatalf("expected two step executions, got %#v", out.Executions)
	}

	plans, err := rt.ListPlans(sess.SessionID)
	if err != nil {
		t.Fatalf("list plans: %v", err)
	}
	if len(plans) != 1 {
		t.Fatalf("expected a single generated plan, got %#v", plans)
	}
	if plans[0].Status != plan.StatusCompleted {
		t.Fatalf("expected generated plan to be completed, got %#v", plans[0])
	}
}
