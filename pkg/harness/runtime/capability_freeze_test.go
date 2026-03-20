package runtime_test

import (
	"context"
	"errors"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/capability"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type unversionedShellPlanner struct {
	stepID string
}

func (p unversionedShellPlanner) PlanNext(_ context.Context, state session.State, _ task.Spec, _ hruntime.ContextPackage) (plan.StepSpec, error) {
	if state.CurrentStepID != "" {
		return plan.StepSpec{}, errors.New("planner exhausted")
	}
	return plan.StepSpec{
		StepID: p.stepID,
		Title:  "use frozen shell capability",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo frozen", "timeout_ms": 5000}},
		Verify: verify.Spec{},
	}, nil
}

func TestCreatePlanFromPlannerFreezesCapabilityViewAndPinsToolVersion(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, &versionedHandler{version: "v1"})
	tools.Register(tool.Definition{ToolName: "shell.exec", Version: "v2", CapabilityType: "executor", RiskLevel: tool.RiskHigh, Enabled: true}, &versionedHandler{version: "v2"})

	rt := hruntime.New(hruntime.Options{
		Tools:   tools,
		Planner: unversionedShellPlanner{stepID: "step_freeze_plan"},
	})

	sess := mustCreateSession(t, rt, "capability freeze", "freeze visible capabilities during planning")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "plan with frozen capabilities"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, _, err := rt.CreatePlanFromPlanner(context.Background(), attached.SessionID, "freeze plan", 1)
	if err != nil {
		t.Fatalf("create plan from planner: %v", err)
	}
	if len(pl.Steps) != 1 {
		t.Fatalf("expected one planned step, got %#v", pl.Steps)
	}
	if pl.Steps[0].Action.ToolVersion != "v2" {
		t.Fatalf("expected planner output to be pinned to frozen highest version, got %#v", pl.Steps[0].Action)
	}
	viewID, _ := pl.Steps[0].Metadata["capability_view_id"].(string)
	if viewID == "" {
		t.Fatalf("expected planned step to carry capability_view_id, got %#v", pl.Steps[0].Metadata)
	}

	items := mustListCapabilitySnapshots(t, rt, attached.SessionID)
	if len(items) != 2 {
		t.Fatalf("expected two plan-scope capability snapshots, got %#v", items)
	}
	for _, item := range items {
		if item.Scope != capability.SnapshotScopePlan || item.ViewID != viewID || item.PlanID != pl.PlanID {
			t.Fatalf("unexpected frozen capability snapshot: %#v", item)
		}
	}
}

func TestRecoverSessionUsesFrozenCapabilityViewAfterRegistryDrift(t *testing.T) {
	tools := tool.NewRegistry()
	v1 := &versionedHandler{version: "v1"}
	v2 := &versionedHandler{version: "v2"}
	v3 := &versionedHandler{version: "v3"}
	tools.Register(tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, v1)
	tools.Register(tool.Definition{ToolName: "shell.exec", Version: "v2", CapabilityType: "executor", RiskLevel: tool.RiskHigh, Enabled: true}, v2)

	rt := hruntime.New(hruntime.Options{
		Tools:   tools,
		Planner: unversionedShellPlanner{stepID: "step_freeze_recover"},
	})

	sess := mustCreateSession(t, rt, "capability recover", "recover with frozen capability view")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "recover planned frozen capability"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, _, err := rt.CreatePlanFromPlanner(context.Background(), attached.SessionID, "freeze before recover", 1)
	if err != nil {
		t.Fatalf("create plan from planner: %v", err)
	}
	viewID, _ := pl.Steps[0].Metadata["capability_view_id"].(string)
	if viewID == "" {
		t.Fatalf("expected planned step to carry capability_view_id")
	}
	if _, err := rt.MarkSessionInterrupted(context.Background(), attached.SessionID); err != nil {
		t.Fatalf("mark interrupted: %v", err)
	}

	tools.Register(tool.Definition{ToolName: "shell.exec", Version: "v3", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, v3)

	out, err := rt.RecoverSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("recover session: %v", err)
	}
	if out.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected recovered session to complete, got %#v", out.Session)
	}
	if v1.calls != 0 || v3.calls != 0 {
		t.Fatalf("expected frozen plan to avoid drifted handlers, got v1=%d v3=%d", v1.calls, v3.calls)
	}
	if v2.calls != 1 {
		t.Fatalf("expected frozen v2 handler to run once, got %d", v2.calls)
	}

	items := mustListCapabilitySnapshots(t, rt, attached.SessionID)
	if len(items) != 3 {
		t.Fatalf("expected two plan snapshots and one action snapshot, got %#v", items)
	}
	foundAction := false
	for _, item := range items {
		if item.Scope == capability.SnapshotScopeAction {
			foundAction = true
			if item.ViewID != viewID || item.Version != "v2" {
				t.Fatalf("unexpected action snapshot relationship: %#v", item)
			}
		}
	}
	if !foundAction {
		t.Fatalf("expected action-scope capability snapshot, got %#v", items)
	}
}

func TestCreatePlanFromPlannerReplanCreatesNewCapabilityView(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, &versionedHandler{version: "v1"})

	rt := hruntime.New(hruntime.Options{
		Tools:   tools,
		Planner: unversionedShellPlanner{stepID: "step_replan_freeze"},
	})

	sess := mustCreateSession(t, rt, "capability replan", "replanning should freeze a fresh view")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "replan frozen capability view"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	first, _, err := rt.CreatePlanFromPlanner(context.Background(), attached.SessionID, "initial freeze", 1)
	if err != nil {
		t.Fatalf("create first plan from planner: %v", err)
	}
	firstViewID, _ := first.Steps[0].Metadata["capability_view_id"].(string)
	if first.Steps[0].Action.ToolVersion != "v1" || firstViewID == "" {
		t.Fatalf("unexpected first frozen plan: %#v", first)
	}

	tools.Register(tool.Definition{ToolName: "shell.exec", Version: "v2", CapabilityType: "executor", RiskLevel: tool.RiskHigh, Enabled: true}, &versionedHandler{version: "v2"})

	second, _, err := rt.CreatePlanFromPlanner(context.Background(), attached.SessionID, "replan freeze", 1)
	if err != nil {
		t.Fatalf("create second plan from planner: %v", err)
	}
	secondViewID, _ := second.Steps[0].Metadata["capability_view_id"].(string)
	if second.Steps[0].Action.ToolVersion != "v2" || secondViewID == "" {
		t.Fatalf("unexpected replanned frozen plan: %#v", second)
	}
	if firstViewID == secondViewID {
		t.Fatalf("expected replanning to create a new capability view, got %q", firstViewID)
	}

	items := mustListCapabilitySnapshots(t, rt, attached.SessionID)
	views := map[string]bool{}
	for _, item := range items {
		if item.Scope == capability.SnapshotScopePlan {
			views[item.ViewID] = true
		}
	}
	if len(views) != 2 {
		t.Fatalf("expected two distinct frozen capability views after replanning, got %#v", items)
	}
}
