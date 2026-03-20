package websocket_test

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gorillaws "github.com/gorilla/websocket"
	adapterws "github.com/yiiilin/harness-core/adapters/websocket"
	"github.com/yiiilin/harness-core/internal/config"
	"github.com/yiiilin/harness-core/internal/protocol"
	"github.com/yiiilin/harness-core/pkg/harness/action"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
)

type websocketHandleHandler struct{}

func (websocketHandleHandler) Invoke(_ context.Context, _ map[string]any) (action.Result, error) {
	return action.Result{
		OK: true,
		Data: map[string]any{
			"stdout": "ready",
			"runtime_handle": map[string]any{
				"handle_id": "ws_handle_1",
				"kind":      "pty",
				"value":     "ws-pty-1",
				"metadata":  map[string]any{"mode": "interactive"},
			},
		},
	}, nil
}

func TestWebSocketExposesExecutionFactListsAndEventReplay(t *testing.T) {
	opts := hruntime.Options{Tools: tool.NewRegistry()}
	opts.Tools.Register(tool.Definition{ToolName: "demo.handle", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, websocketHandleHandler{})
	rt := hruntime.New(opts)

	srv := adapterws.New(config.Config{Addr: "127.0.0.1:0", SharedToken: "dev-token"}, rt)
	httpSrv := httptest.NewServer(srv.Handler())
	defer httpSrv.Close()
	wsURL := "ws" + strings.TrimPrefix(httpSrv.URL, "http") + "/ws"

	conn, _, err := gorillaws.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	mustWrite := func(v any) {
		if err := conn.WriteJSON(v); err != nil {
			t.Fatalf("write json: %v", err)
		}
	}

	mustWrite(wsEnvelope{ID: "1", Type: "auth", Token: "dev-token"})
	if ok, _ := mustRecvMap(t, conn)["ok"].(bool); !ok {
		t.Fatalf("auth failed")
	}

	mustWrite(wsEnvelope{ID: "2", Type: "request", Action: "session.create", Payload: map[string]any{"title": "ws-observe", "goal": "expose execution facts"}})
	sessionResp := mustRecvMap(t, conn)
	sessionID := sessionResp["result"].(map[string]any)["session_id"]

	mustWrite(wsEnvelope{ID: "3", Type: "request", Action: "task.create", Payload: map[string]any{"task_type": "demo", "goal": "collect runtime records"}})
	taskResp := mustRecvMap(t, conn)
	taskID := taskResp["result"].(map[string]any)["task_id"]

	mustWrite(wsEnvelope{ID: "4", Type: "request", Action: "session.attach_task", Payload: map[string]any{"session_id": sessionID, "task_id": taskID}})
	if ok, _ := mustRecvMap(t, conn)["ok"].(bool); !ok {
		t.Fatalf("attach failed")
	}

	step := map[string]any{
		"step_id": "step_ws_observe",
		"title":   "open runtime handle",
		"action": map[string]any{
			"tool_name": "demo.handle",
			"args":      map[string]any{"mode": "interactive"},
		},
		"verify": map[string]any{
			"mode":   "all",
			"checks": []any{},
		},
	}

	mustWrite(wsEnvelope{ID: "5", Type: "request", Action: "plan.create", Payload: map[string]any{"session_id": sessionID, "change_reason": "observe", "steps": []any{step}}})
	if ok, _ := mustRecvMap(t, conn)["ok"].(bool); !ok {
		t.Fatalf("plan.create failed")
	}

	mustWrite(wsEnvelope{ID: "6", Type: "request", Action: "step.run", Payload: map[string]any{"session_id": sessionID, "step": step}})
	stepResp := mustRecvMap(t, conn)
	if ok, _ := stepResp["ok"].(bool); !ok {
		t.Fatalf("step.run failed: %#v", stepResp)
	}

	for _, actionName := range []string{
		"attempt.list",
		"action.list",
		"verification.list",
		"artifact.list",
		"capability_snapshot.list",
		"context_summary.list",
	} {
		mustWrite(wsEnvelope{ID: actionName, Type: "request", Action: actionName, Payload: map[string]any{"session_id": sessionID}})
		resp := mustRecvMap(t, conn)
		if ok, _ := resp["ok"].(bool); !ok {
			t.Fatalf("%s failed: %#v", actionName, resp)
		}
	}

	mustWrite(wsEnvelope{ID: "runtime_handle.list", Type: "request", Action: "runtime_handle.list", Payload: map[string]any{"session_id": sessionID}})
	handleListResp := mustRecvMap(t, conn)
	if ok, _ := handleListResp["ok"].(bool); !ok {
		t.Fatalf("runtime_handle.list failed: %#v", handleListResp)
	}
	handleItems, _ := handleListResp["result"].([]any)
	if len(handleItems) != 1 {
		t.Fatalf("expected one runtime handle, got %#v", handleListResp)
	}
	handleID := handleItems[0].(map[string]any)["handle_id"]

	mustWrite(wsEnvelope{ID: "7", Type: "request", Action: "runtime_handle.get", Payload: map[string]any{"handle_id": handleID}})
	handleResp := mustRecvMap(t, conn)
	if ok, _ := handleResp["ok"].(bool); !ok {
		t.Fatalf("runtime_handle.get failed: %#v", handleResp)
	}

	mustWrite(wsEnvelope{ID: "8", Type: "request", Action: "event.replay", Payload: map[string]any{"session_id": sessionID}})
	replayResp := mustRecvMap(t, conn)
	if ok, _ := replayResp["ok"].(bool); !ok {
		t.Fatalf("event.replay failed: %#v", replayResp)
	}

	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	eventMsg := mustRecvMap(t, conn)
	if typ, _ := eventMsg["type"].(string); typ != protocol.EnvelopeTypeEvent {
		t.Fatalf("expected event envelope, got %#v", eventMsg)
	}
	_ = conn.SetReadDeadline(time.Time{})
}

func TestWebSocketApprovalRejectsInvalidAndRepeatedReplies(t *testing.T) {
	opts := hruntime.Options{}
	hruntime.RegisterBuiltins(&opts)
	opts.Policy = askAllWebSocketPolicy{}
	rt := hruntime.New(opts)

	srv := adapterws.New(config.Config{Addr: "127.0.0.1:0", SharedToken: "dev-token"}, rt)
	httpSrv := httptest.NewServer(srv.Handler())
	defer httpSrv.Close()
	wsURL := "ws" + strings.TrimPrefix(httpSrv.URL, "http") + "/ws"

	conn, _, err := gorillaws.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	mustWrite := func(v any) {
		if err := conn.WriteJSON(v); err != nil {
			t.Fatalf("write json: %v", err)
		}
	}

	mustWrite(wsEnvelope{ID: "1", Type: "auth", Token: "dev-token"})
	if ok, _ := mustRecvMap(t, conn)["ok"].(bool); !ok {
		t.Fatalf("auth failed")
	}

	mustWrite(wsEnvelope{ID: "2", Type: "request", Action: "session.create", Payload: map[string]any{"title": "ws-approval-validate", "goal": "approval validation"}})
	sessionResp := mustRecvMap(t, conn)
	sessionID := sessionResp["result"].(map[string]any)["session_id"]

	mustWrite(wsEnvelope{ID: "3", Type: "request", Action: "task.create", Payload: map[string]any{"task_type": "demo", "goal": "reject invalid approval replies"}})
	taskResp := mustRecvMap(t, conn)
	taskID := taskResp["result"].(map[string]any)["task_id"]

	mustWrite(wsEnvelope{ID: "4", Type: "request", Action: "session.attach_task", Payload: map[string]any{"session_id": sessionID, "task_id": taskID}})
	if ok, _ := mustRecvMap(t, conn)["ok"].(bool); !ok {
		t.Fatalf("attach failed")
	}

	step := map[string]any{
		"step_id": "step_ws_validate",
		"title":   "approval gated shell command",
		"action": map[string]any{
			"tool_name": "shell.exec",
			"args":      map[string]any{"mode": "pipe", "command": "echo approved", "timeout_ms": 5000},
		},
		"verify": map[string]any{
			"mode": "all",
			"checks": []map[string]any{
				{"kind": "exit_code", "args": map[string]any{"allowed": []any{0}}},
			},
		},
	}

	mustWrite(wsEnvelope{ID: "5", Type: "request", Action: "plan.create", Payload: map[string]any{"session_id": sessionID, "change_reason": "approval validation", "steps": []any{step}}})
	if ok, _ := mustRecvMap(t, conn)["ok"].(bool); !ok {
		t.Fatalf("plan.create failed")
	}

	mustWrite(wsEnvelope{ID: "6", Type: "request", Action: "step.run", Payload: map[string]any{"session_id": sessionID, "step": step}})
	initial := mustRecvMap(t, conn)
	if ok, _ := initial["ok"].(bool); !ok {
		t.Fatalf("step.run failed: %#v", initial)
	}
	approvalID := initial["result"].(map[string]any)["execution"].(map[string]any)["pending_approval"].(map[string]any)["approval_id"]

	mustWrite(wsEnvelope{ID: "7", Type: "request", Action: "approval.respond", Payload: map[string]any{"approval_id": approvalID, "reply": "bogus"}})
	invalidReply := mustRecvMap(t, conn)
	if ok, _ := invalidReply["ok"].(bool); ok {
		t.Fatalf("expected invalid approval reply to fail, got %#v", invalidReply)
	}

	mustWrite(wsEnvelope{ID: "8", Type: "request", Action: "approval.respond", Payload: map[string]any{"approval_id": approvalID, "reply": "once"}})
	approved := mustRecvMap(t, conn)
	if ok, _ := approved["ok"].(bool); !ok {
		t.Fatalf("approval.respond once failed: %#v", approved)
	}

	mustWrite(wsEnvelope{ID: "9", Type: "request", Action: "session.resume", Payload: map[string]any{"session_id": sessionID}})
	resume := mustRecvMap(t, conn)
	if ok, _ := resume["ok"].(bool); !ok {
		t.Fatalf("session.resume failed: %#v", resume)
	}

	mustWrite(wsEnvelope{ID: "10", Type: "request", Action: "approval.respond", Payload: map[string]any{"approval_id": approvalID, "reply": "always"}})
	repeated := mustRecvMap(t, conn)
	if ok, _ := repeated["ok"].(bool); ok {
		t.Fatalf("expected repeated approval response to fail, got %#v", repeated)
	}
}
