package runtime_test

import (
	"context"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/capability"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type versionedHandler struct {
	version string
	calls   int
}

func (h *versionedHandler) Invoke(_ context.Context, _ map[string]any) (action.Result, error) {
	h.calls++
	return action.Result{OK: true, Data: map[string]any{"version": h.version}}, nil
}

func TestRunStepResolvesRequestedCapabilityVersionAndPersistsSnapshot(t *testing.T) {
	tools := tool.NewRegistry()
	v1 := &versionedHandler{version: "v1"}
	v2 := &versionedHandler{version: "v2"}
	tools.Register(tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, v1)
	tools.Register(tool.Definition{ToolName: "shell.exec", Version: "v2", CapabilityType: "executor", RiskLevel: tool.RiskHigh, Enabled: true}, v2)

	snapshots := capability.NewMemorySnapshotStore()
	rt := hruntime.New(hruntime.Options{
		Tools:               tools,
		CapabilitySnapshots: snapshots,
	})

	sess := mustCreateSession(t, rt, "capabilities", "resolve versioned tools")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "run the requested capability version"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(attached.SessionID, "versioned tool", []plan.StepSpec{{
		StepID: "step_capability",
		Title:  "use v2 tool",
		Action: action.Spec{ToolName: "shell.exec", ToolVersion: "v2", Args: map[string]any{"mode": "pipe", "command": "echo version"}},
		Verify: verify.Spec{},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	out, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}

	if v1.calls != 0 {
		t.Fatalf("expected v1 handler to remain unused, got %d calls", v1.calls)
	}
	if v2.calls != 1 {
		t.Fatalf("expected v2 handler to be used once, got %d calls", v2.calls)
	}
	items := mustListCapabilitySnapshots(t, rt, attached.SessionID)
	if len(items) != 1 {
		t.Fatalf("expected one persisted capability snapshot, got %#v", items)
	}
	if items[0].ToolName != "shell.exec" || items[0].Version != "v2" || items[0].RiskLevel != tool.RiskHigh {
		t.Fatalf("unexpected capability snapshot: %#v", items[0])
	}
	if !out.Execution.Action.OK {
		t.Fatalf("expected successful action result, got %#v", out.Execution.Action)
	}
}
