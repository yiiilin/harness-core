package runtime

import (
	"context"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type selectorRuntimeHandleHandler struct{}

func (selectorRuntimeHandleHandler) Invoke(_ context.Context, _ map[string]any) (action.Result, error) {
	return action.Result{
		OK: true,
		Data: map[string]any{
			"runtime_handle": map[string]any{
				"handle_id": "hdl_selector",
				"kind":      "pty",
				"value":     "pty-session-selector",
			},
		},
	}, nil
}

func TestResolveProgramRuntimeHandleRefSupportsHandleIDAndActionIDSelectors(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.handle.selector", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, selectorRuntimeHandleHandler{})

	rt := New(Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess, err := rt.CreateSession("runtime handle selectors", "resolve runtime handle refs by direct selectors")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	tsk, err := rt.CreateTask(task.Spec{TaskType: "demo", Goal: "resolve runtime handle refs by direct selectors"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(attached.SessionID, "runtime handle selector setup", []plan.StepSpec{{
		StepID: "step_start",
		Title:  "start runtime handle",
		Action: action.Spec{ToolName: "demo.handle.selector"},
		Verify: verify.Spec{},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0]); err != nil {
		t.Fatalf("run step: %v", err)
	}

	actions, err := rt.ListActions(attached.SessionID)
	if err != nil {
		t.Fatalf("list actions: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("expected one action, got %#v", actions)
	}
	handles, err := rt.ListRuntimeHandles(attached.SessionID)
	if err != nil {
		t.Fatalf("list runtime handles: %v", err)
	}
	if len(handles) != 1 {
		t.Fatalf("expected one runtime handle, got %#v", handles)
	}
	if got, _ := handles[0].Metadata["action_id"].(string); got != actions[0].ActionID {
		t.Fatalf("expected runtime handle metadata to retain action_id %q, got %#v", actions[0].ActionID, handles[0].Metadata)
	}

	resolvedByHandleID, err := rt.resolveProgramRuntimeHandleRef(context.Background(), attached.SessionID, plan.StepSpec{StepID: "step_consume"}, execution.RuntimeHandleRef{
		HandleID: handles[0].HandleID,
	})
	if err != nil {
		t.Fatalf("resolve runtime handle ref by handle id: %v", err)
	}
	if resolvedByHandleID.HandleID != handles[0].HandleID || resolvedByHandleID.Kind != handles[0].Kind {
		t.Fatalf("expected handle-id selector to resolve runtime handle ref, got %#v", resolvedByHandleID)
	}

	resolvedByActionID, err := rt.resolveProgramRuntimeHandleRef(context.Background(), attached.SessionID, plan.StepSpec{StepID: "step_consume"}, execution.RuntimeHandleRef{
		ActionID: actions[0].ActionID,
	})
	if err != nil {
		t.Fatalf("resolve runtime handle ref by action id: %v", err)
	}
	if resolvedByActionID.HandleID != handles[0].HandleID || resolvedByActionID.ActionID != actions[0].ActionID {
		t.Fatalf("expected action-id selector to resolve runtime handle ref, got %#v", resolvedByActionID)
	}
}
