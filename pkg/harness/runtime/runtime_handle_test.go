package runtime_test

import (
	"context"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/builtins"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
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

type runtimeHandlesTypedStructHandler struct{}

func (runtimeHandlesTypedStructHandler) Invoke(_ context.Context, _ map[string]any) (action.Result, error) {
	return action.Result{
		OK: true,
		Data: map[string]any{
			"runtime_handles": []execution.RuntimeHandle{
				{
					HandleID: "hdl_test_4",
					Kind:     "pty",
					Value:    "pty-session-4",
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

func TestRunStepPersistsRuntimeHandlesFromExecutionStructSlice(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.handle.struct-slice", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, runtimeHandlesTypedStructHandler{})

	rt := hruntime.New(hruntime.Options{Tools: tools})

	sess := mustCreateSession(t, rt, "runtime handle struct slices", "persist handles from execution.RuntimeHandle slices")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "capture struct runtime handles"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(attached.SessionID, "runtime handle struct slice", []plan.StepSpec{{
		StepID: "step_runtime_handle_struct_slice",
		Title:  "launch struct handle slice",
		Action: action.Spec{ToolName: "demo.handle.struct-slice", Args: map[string]any{"mode": "interactive"}},
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
	if handles[0].HandleID != "hdl_test_4" || handles[0].Kind != "pty" || handles[0].Value != "pty-session-4" {
		t.Fatalf("unexpected runtime handle from struct slice: %#v", handles[0])
	}
}

func TestRuntimeHandleControlSurfaceUpdatesAndClosesHandles(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.handle", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, runtimeHandleHandler{})

	rt := hruntime.New(hruntime.Options{Tools: tools})

	sess := mustCreateSession(t, rt, "runtime handle lifecycle", "manage handle lifecycle through runtime service")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "capture runtime handle lifecycle"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(attached.SessionID, "runtime handle lifecycle", []plan.StepSpec{{
		StepID: "step_runtime_handle_lifecycle",
		Title:  "launch handle for lifecycle",
		Action: action.Spec{ToolName: "demo.handle", Args: map[string]any{"mode": "interactive"}},
		Verify: verify.Spec{},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	if _, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0]); err != nil {
		t.Fatalf("run step: %v", err)
	}

	initial, err := rt.GetRuntimeHandle("hdl_test_1")
	if err != nil {
		t.Fatalf("get runtime handle: %v", err)
	}
	if initial.Status != execution.RuntimeHandleActive {
		t.Fatalf("expected active runtime handle by default, got %#v", initial)
	}

	nextValue := "pty-session-1-updated"
	updated, err := rt.UpdateRuntimeHandle(context.Background(), "hdl_test_1", hruntime.RuntimeHandleUpdate{
		Value:    &nextValue,
		Metadata: map[string]any{"attached_client": "cli"},
	})
	if err != nil {
		t.Fatalf("update runtime handle: %v", err)
	}
	if updated.Status != execution.RuntimeHandleActive || updated.Value != nextValue {
		t.Fatalf("unexpected updated runtime handle: %#v", updated)
	}
	if got, _ := updated.Metadata["attached_client"].(string); got != "cli" {
		t.Fatalf("expected merged update metadata, got %#v", updated.Metadata)
	}

	closed, err := rt.CloseRuntimeHandle(context.Background(), "hdl_test_1", hruntime.RuntimeHandleCloseRequest{
		Reason:   "client closed",
		Metadata: map[string]any{"closed_by": "operator"},
	})
	if err != nil {
		t.Fatalf("close runtime handle: %v", err)
	}
	if closed.Status != execution.RuntimeHandleClosed || closed.ClosedAt == 0 || closed.StatusReason != "client closed" {
		t.Fatalf("unexpected closed runtime handle: %#v", closed)
	}
	if got, _ := closed.Metadata["closed_by"].(string); got != "operator" {
		t.Fatalf("expected close metadata to persist, got %#v", closed.Metadata)
	}
}

func TestRuntimeHandleControlSurfaceInvalidatesHandle(t *testing.T) {
	rt := hruntime.New(hruntime.Options{})

	sess := mustCreateSession(t, rt, "runtime handle invalidation", "invalidate active runtime handles")
	if _, err := rt.RuntimeHandles.Create(execution.RuntimeHandle{
		HandleID:  "hdl_invalidate",
		SessionID: sess.SessionID,
		Kind:      "pty",
		Value:     "pty-invalidate",
		Status:    execution.RuntimeHandleActive,
	}); err != nil {
		t.Fatalf("seed runtime handle: %v", err)
	}

	invalidated, err := rt.InvalidateRuntimeHandle(context.Background(), "hdl_invalidate", hruntime.RuntimeHandleInvalidateRequest{
		Reason:   "kernel reconcile",
		Metadata: map[string]any{"reconciled_by": "runtime"},
	})
	if err != nil {
		t.Fatalf("invalidate runtime handle: %v", err)
	}
	if invalidated.Status != execution.RuntimeHandleInvalidated || invalidated.InvalidatedAt == 0 || invalidated.StatusReason != "kernel reconcile" {
		t.Fatalf("unexpected invalidated runtime handle: %#v", invalidated)
	}
	if got, _ := invalidated.Metadata["reconciled_by"].(string); got != "runtime" {
		t.Fatalf("expected invalidate metadata to persist, got %#v", invalidated.Metadata)
	}
}

func TestAbortSessionInvalidatesActiveRuntimeHandles(t *testing.T) {
	rt := hruntime.New(hruntime.Options{})

	sess := mustCreateSession(t, rt, "abort runtime handles", "abort should invalidate active handles")
	if _, err := rt.MarkSessionInFlight(context.Background(), sess.SessionID, "step_abort_handles"); err != nil {
		t.Fatalf("mark in-flight: %v", err)
	}
	if _, err := rt.RuntimeHandles.Create(execution.RuntimeHandle{
		HandleID:  "hdl_abort_runtime",
		SessionID: sess.SessionID,
		Kind:      "pty",
		Value:     "pty-abort",
		Status:    execution.RuntimeHandleActive,
	}); err != nil {
		t.Fatalf("seed runtime handle: %v", err)
	}

	if _, err := rt.AbortSession(context.Background(), sess.SessionID, hruntime.AbortRequest{
		Code:   "operator.abort",
		Reason: "abort runtime handles",
	}); err != nil {
		t.Fatalf("abort session: %v", err)
	}

	got, err := rt.GetRuntimeHandle("hdl_abort_runtime")
	if err != nil {
		t.Fatalf("get runtime handle: %v", err)
	}
	if got.Status != execution.RuntimeHandleInvalidated || got.InvalidatedAt == 0 || got.StatusReason != "session aborted" {
		t.Fatalf("expected abort to invalidate active runtime handles, got %#v", got)
	}
}

func TestRecoverSessionInvalidatesDanglingRuntimeHandlesAcrossRuntimeReinit(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	runtimeHandles := execution.NewMemoryRuntimeHandleStore()

	opts := hruntime.Options{
		Sessions:       sessions,
		Tasks:          tasks,
		Plans:          plans,
		RuntimeHandles: runtimeHandles,
	}
	builtins.Register(&opts)

	rt1 := hruntime.New(opts)
	sess := mustCreateSession(t, rt1, "recover handles", "recover should invalidate stale handles")
	tsk := mustCreateTask(t, rt1, task.Spec{TaskType: "demo", Goal: "recover with stale runtime handle"})
	attached, err := rt1.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	if _, err := rt1.CreatePlan(attached.SessionID, "recover runtime handle", []plan.StepSpec{{
		StepID: "step_recover_runtime_handle",
		Title:  "recover after stale handle",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo recovered", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
			{Kind: "output_contains", Args: map[string]any{"text": "recovered"}},
		}},
	}}); err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := rt1.MarkSessionInterrupted(context.Background(), attached.SessionID); err != nil {
		t.Fatalf("mark interrupted: %v", err)
	}
	if _, err := runtimeHandles.Create(execution.RuntimeHandle{
		HandleID:  "hdl_recover_runtime",
		SessionID: attached.SessionID,
		TaskID:    attached.TaskID,
		Kind:      "pty",
		Value:     "pty-recover",
		Status:    execution.RuntimeHandleActive,
	}); err != nil {
		t.Fatalf("seed runtime handle: %v", err)
	}

	rt2 := hruntime.New(opts)
	out, err := rt2.RecoverSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("recover session: %v", err)
	}
	if out.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected recovery to complete, got %#v", out.Session)
	}

	got, err := rt2.GetRuntimeHandle("hdl_recover_runtime")
	if err != nil {
		t.Fatalf("get runtime handle: %v", err)
	}
	if got.Status != execution.RuntimeHandleInvalidated || got.InvalidatedAt == 0 || got.StatusReason != "session recovered" {
		t.Fatalf("expected recovery to invalidate stale runtime handles, got %#v", got)
	}
}
