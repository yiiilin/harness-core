package runtime_test

import (
	"context"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type runtimeHandleHandler struct{}

func (runtimeHandleHandler) Invoke(_ context.Context, _ map[string]any) (action.Result, error) {
	return action.Result{
		OK: true,
		Data: map[string]any{
			"stdout": "interactive ready",
			"runtime_handle": map[string]any{
				"handle_id": "hdl_test_1",
				"kind":      "pty",
				"value":     "pty-session-1",
				"metadata":  map[string]any{"mode": "interactive"},
			},
		},
	}, nil
}

type runtimeHandlesSliceHandler struct{}

func (runtimeHandlesSliceHandler) Invoke(_ context.Context, _ map[string]any) (action.Result, error) {
	return action.Result{
		OK: true,
		Data: map[string]any{
			"stdout": "multiple interactive handles ready",
			"runtime_handles": []map[string]any{
				{
					"handle_id": "hdl_test_2",
					"kind":      "pty",
					"value":     "pty-session-2",
				},
				{
					"handle_id": "hdl_test_3",
					"kind":      "pty",
					"value":     "pty-session-3",
				},
			},
		},
	}, nil
}

func TestRunStepPersistsRuntimeHandles(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.handle", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, runtimeHandleHandler{})

	rt := hruntime.New(hruntime.Options{Tools: tools})

	sess := mustCreateSession(t, rt, "runtime handles", "persist handle records from tool results")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "capture runtime handle"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(attached.SessionID, "runtime handle", []plan.StepSpec{{
		StepID: "step_runtime_handle",
		Title:  "launch handle",
		Action: action.Spec{ToolName: "demo.handle", Args: map[string]any{"mode": "interactive"}},
		Verify: verify.Spec{},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	if _, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0]); err != nil {
		t.Fatalf("run step: %v", err)
	}

	handles, err := rt.ListRuntimeHandles(attached.SessionID)
	if err != nil {
		t.Fatalf("list runtime handles: %v", err)
	}
	if len(handles) != 1 {
		t.Fatalf("expected one runtime handle, got %#v", handles)
	}
	if handles[0].HandleID != "hdl_test_1" || handles[0].AttemptID == "" || handles[0].TraceID == "" {
		t.Fatalf("unexpected runtime handle: %#v", handles[0])
	}

	got, err := rt.GetRuntimeHandle("hdl_test_1")
	if err != nil {
		t.Fatalf("get runtime handle: %v", err)
	}
	if got.Value != "pty-session-1" || got.Kind != "pty" {
		t.Fatalf("unexpected runtime handle lookup result: %#v", got)
	}
}

func TestRunStepPersistsRuntimeHandlesFromTypedSlice(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.handle.slice", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, runtimeHandlesSliceHandler{})

	rt := hruntime.New(hruntime.Options{Tools: tools})

	sess := mustCreateSession(t, rt, "runtime handle slices", "persist handles from typed slices")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "capture typed runtime handles"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(attached.SessionID, "runtime handle slice", []plan.StepSpec{{
		StepID: "step_runtime_handles",
		Title:  "launch typed handle slice",
		Action: action.Spec{ToolName: "demo.handle.slice", Args: map[string]any{"mode": "interactive"}},
		Verify: verify.Spec{},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	if _, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0]); err != nil {
		t.Fatalf("run step: %v", err)
	}

	handles, err := rt.ListRuntimeHandles(attached.SessionID)
	if err != nil {
		t.Fatalf("list runtime handles: %v", err)
	}
	if len(handles) != 2 {
		t.Fatalf("expected two runtime handles, got %#v", handles)
	}
	ids := map[string]bool{}
	for _, handle := range handles {
		ids[handle.HandleID] = true
	}
	if !ids["hdl_test_2"] || !ids["hdl_test_3"] {
		t.Fatalf("unexpected runtime handles: %#v", handles)
	}
}
