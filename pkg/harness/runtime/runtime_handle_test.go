package runtime_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

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
	if initial.CycleID == "" {
		t.Fatalf("expected runtime handle cycle_id, got %#v", initial)
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
	if updated.CycleID != initial.CycleID {
		t.Fatalf("expected update to preserve cycle_id %q, got %#v", initial.CycleID, updated)
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
	if closed.CycleID != initial.CycleID {
		t.Fatalf("expected close to preserve cycle_id %q, got %#v", initial.CycleID, closed)
	}
	if got, _ := closed.Metadata["closed_by"].(string); got != "operator" {
		t.Fatalf("expected close metadata to persist, got %#v", closed.Metadata)
	}
	if _, err := rt.UpdateRuntimeHandle(context.Background(), "hdl_test_1", hruntime.RuntimeHandleUpdate{
		Metadata: map[string]any{"late_update": true},
	}); !errors.Is(err, hruntime.ErrRuntimeHandleNotActive) {
		t.Fatalf("expected closed handle update to fail with ErrRuntimeHandleNotActive, got %v", err)
	}
	if _, err := rt.InvalidateRuntimeHandle(context.Background(), "hdl_test_1", hruntime.RuntimeHandleInvalidateRequest{
		Reason: "late invalidate",
	}); !errors.Is(err, hruntime.ErrRuntimeHandleNotActive) {
		t.Fatalf("expected closed handle invalidate to fail with ErrRuntimeHandleNotActive, got %v", err)
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
	if _, err := rt.CloseRuntimeHandle(context.Background(), "hdl_invalidate", hruntime.RuntimeHandleCloseRequest{
		Reason: "late close",
	}); !errors.Is(err, hruntime.ErrRuntimeHandleNotActive) {
		t.Fatalf("expected invalidated handle close to fail with ErrRuntimeHandleNotActive, got %v", err)
	}
}

func TestRuntimeHandleControlSurfaceRequiresUnclaimedSessionAndExposesVersion(t *testing.T) {
	rt := hruntime.New(hruntime.Options{})

	sess := mustCreateSession(t, rt, "claimed runtime handle", "control surfaces should respect active session leases")
	created, err := rt.RuntimeHandles.Create(execution.RuntimeHandle{
		HandleID:  "hdl_claimed_runtime",
		SessionID: sess.SessionID,
		Kind:      "pty",
		Value:     "pty-claimed",
		Status:    execution.RuntimeHandleActive,
	})
	if err != nil {
		t.Fatalf("seed runtime handle: %v", err)
	}

	initialVersion, ok := runtimeHandleInt64Field(created, "Version")
	if !ok {
		t.Fatalf("expected RuntimeHandle to expose Version field, got %#v", created)
	}

	claimed, found, err := rt.ClaimRunnableSession(context.Background(), time.Minute)
	if err != nil {
		t.Fatalf("claim runnable session: %v", err)
	}
	if !found || claimed.SessionID != sess.SessionID {
		t.Fatalf("expected session %q to be claimed, got found=%v state=%#v", sess.SessionID, found, claimed)
	}

	nextValue := "pty-claimed-updated"
	if _, err := rt.UpdateRuntimeHandle(context.Background(), created.HandleID, hruntime.RuntimeHandleUpdate{
		Value: &nextValue,
	}); !errors.Is(err, session.ErrSessionLeaseNotHeld) {
		t.Fatalf("expected unclaimed runtime handle mutation to fail while session lease is active, got %v", err)
	}

	if _, err := rt.ReleaseSessionLease(context.Background(), claimed.SessionID, claimed.LeaseID); err != nil {
		t.Fatalf("release session lease: %v", err)
	}

	updated, err := rt.UpdateRuntimeHandle(context.Background(), created.HandleID, hruntime.RuntimeHandleUpdate{
		Value: &nextValue,
	})
	if err != nil {
		t.Fatalf("update runtime handle after lease release: %v", err)
	}
	updatedVersion, ok := runtimeHandleInt64Field(updated, "Version")
	if !ok {
		t.Fatalf("expected updated RuntimeHandle to expose Version field, got %#v", updated)
	}
	if updatedVersion <= initialVersion {
		t.Fatalf("expected runtime handle version to advance after mutation, got before=%d after=%d", initialVersion, updatedVersion)
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

func runtimeHandleInt64Field(handle execution.RuntimeHandle, field string) (int64, bool) {
	value := reflect.ValueOf(handle).FieldByName(field)
	if !value.IsValid() {
		return 0, false
	}
	switch value.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return value.Int(), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int64(value.Uint()), true
	default:
		return 0, false
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
