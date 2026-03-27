package runtime_test

import (
	"context"
	"errors"
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

func TestNativeInteractiveCapabilityAPIsExposeLifecycleTools(t *testing.T) {
	rt := hruntime.New(hruntime.Options{InteractiveController: &stubInteractiveController{}})
	toolNames := []string{
		hruntime.ProgramInteractiveStartToolName,
		hruntime.ProgramInteractiveViewToolName,
		hruntime.ProgramInteractiveWriteToolName,
		hruntime.ProgramInteractiveVerifyToolName,
		hruntime.ProgramInteractiveCloseToolName,
	}

	for _, toolName := range toolNames {
		req := capability.Request{
			SessionID: "sess_native_capability",
			TaskID:    "task_native_capability",
			StepID:    "step_native_capability",
			Action:    action.Spec{ToolName: toolName},
		}
		resolution, err := rt.ResolveCapability(context.Background(), req)
		if err != nil {
			t.Fatalf("resolve native interactive capability %q: %v", toolName, err)
		}
		if resolution.Definition.ToolName != toolName || resolution.Definition.Version != "native" || resolution.Definition.CapabilityType != "interactive" {
			t.Fatalf("unexpected native interactive resolution for %q: %#v", toolName, resolution)
		}
		if resolution.Snapshot.ToolName != toolName || resolution.Snapshot.Version != "native" || resolution.Snapshot.Scope != capability.SnapshotScopeAction {
			t.Fatalf("unexpected native interactive snapshot for %q: %#v", toolName, resolution.Snapshot)
		}
		if native, _ := resolution.Definition.Metadata["native"].(bool); !native {
			t.Fatalf("expected native metadata on definition for %q, got %#v", toolName, resolution.Definition.Metadata)
		}
		match, err := rt.MatchCapability(context.Background(), req)
		if err != nil {
			t.Fatalf("match native interactive capability %q: %v", toolName, err)
		}
		if !match.Supported || match.Resolution == nil || match.Resolution.Definition.ToolName != toolName {
			t.Fatalf("expected native interactive capability %q to match successfully, got %#v", toolName, match)
		}
	}
}

func TestNativeInteractiveCapabilityAPIsFailClosedWithoutInteractiveController(t *testing.T) {
	rt := hruntime.New(hruntime.Options{})
	controllerRequired := []string{
		hruntime.ProgramInteractiveStartToolName,
		hruntime.ProgramInteractiveViewToolName,
		hruntime.ProgramInteractiveWriteToolName,
		hruntime.ProgramInteractiveCloseToolName,
	}

	for _, toolName := range controllerRequired {
		req := capability.Request{Action: action.Spec{ToolName: toolName}}
		_, err := rt.ResolveCapability(context.Background(), req)
		if !errors.Is(err, capability.ErrCapabilityDisabled) {
			t.Fatalf("expected %q to resolve as CAPABILITY_DISABLED without interactive controller, got %v", toolName, err)
		}
		match, err := rt.MatchCapability(context.Background(), req)
		if err != nil {
			t.Fatalf("match native interactive capability %q without controller: %v", toolName, err)
		}
		if match.Supported || len(match.Reasons) != 1 || match.Reasons[0].Code != capability.ReasonCapabilityDisabled {
			t.Fatalf("expected %q to fail closed without controller, got %#v", toolName, match)
		}
	}

	verifyReq := capability.Request{Action: action.Spec{ToolName: hruntime.ProgramInteractiveVerifyToolName}}
	resolution, err := rt.ResolveCapability(context.Background(), verifyReq)
	if err != nil {
		t.Fatalf("resolve interactive.verify without controller: %v", err)
	}
	if resolution.Definition.ToolName != hruntime.ProgramInteractiveVerifyToolName {
		t.Fatalf("unexpected interactive.verify resolution without controller: %#v", resolution)
	}
	match, err := rt.MatchCapability(context.Background(), verifyReq)
	if err != nil {
		t.Fatalf("match interactive.verify without controller: %v", err)
	}
	if !match.Supported {
		t.Fatalf("expected interactive.verify to remain supported without controller, got %#v", match)
	}
}

func TestResolveCapabilityRejectsUnsupportedNativeInteractiveVersion(t *testing.T) {
	rt := hruntime.New(hruntime.Options{})
	_, err := rt.ResolveCapability(context.Background(), capability.Request{
		Action: action.Spec{
			ToolName:    hruntime.ProgramInteractiveStartToolName,
			ToolVersion: "v1",
		},
	})
	if err == nil {
		t.Fatal("expected native interactive version mismatch to fail resolution")
	}
	reason, ok := capability.UnsupportedReasonFromError(err, capability.Request{
		Action: action.Spec{
			ToolName:    hruntime.ProgramInteractiveStartToolName,
			ToolVersion: "v1",
		},
	})
	if !ok || reason.Code != capability.ReasonCapabilityVersionNotFound {
		t.Fatalf("expected native interactive version mismatch to map to CAPABILITY_VERSION_NOT_FOUND, got reason=%#v ok=%v err=%v", reason, ok, err)
	}
	match, err := rt.MatchCapability(context.Background(), capability.Request{
		Action: action.Spec{
			ToolName:    hruntime.ProgramInteractiveStartToolName,
			ToolVersion: "v1",
		},
	})
	if err != nil {
		t.Fatalf("match native interactive version mismatch: %v", err)
	}
	if match.Supported || len(match.Reasons) != 1 || match.Reasons[0].Code != capability.ReasonCapabilityVersionNotFound {
		t.Fatalf("expected native interactive version mismatch to surface CAPABILITY_VERSION_NOT_FOUND, got %#v", match)
	}
}
