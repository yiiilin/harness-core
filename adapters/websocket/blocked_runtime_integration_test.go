package websocket_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	gorillaws "github.com/gorilla/websocket"
	adapterws "github.com/yiiilin/harness-core/adapters/websocket"
	"github.com/yiiilin/harness-core/pkg/harness/builtins"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
)

func TestWebSocketBlockedRuntimeReadFlow(t *testing.T) {
	opts := hruntime.Options{}
	builtins.Register(&opts)
	opts.Policy = askAllWebSocketPolicy{}
	rt := hruntime.New(opts)

	srv := adapterws.New(adapterws.Config{Addr: "127.0.0.1:0", SharedToken: "dev-token"}, rt)
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

	mustWrite(wsEnvelope{ID: "2", Type: "request", Action: "session.create", Payload: map[string]any{"title": "ws-blocked", "goal": "inspect blocked runtime reads"}})
	sessionResp := mustRecvMap(t, conn)
	sessionID := sessionResp["result"].(map[string]any)["session_id"]

	mustWrite(wsEnvelope{ID: "3", Type: "request", Action: "task.create", Payload: map[string]any{"task_type": "demo", "goal": "approval gated"}})
	taskResp := mustRecvMap(t, conn)
	taskID := taskResp["result"].(map[string]any)["task_id"]

	mustWrite(wsEnvelope{ID: "4", Type: "request", Action: "session.attach_task", Payload: map[string]any{"session_id": sessionID, "task_id": taskID}})
	if ok, _ := mustRecvMap(t, conn)["ok"].(bool); !ok {
		t.Fatalf("attach failed")
	}

	step := map[string]any{
		"step_id": "step_ws_blocked",
		"title":   "approval gated shell command",
		"action": map[string]any{
			"tool_name": "shell.exec",
			"args":      map[string]any{"mode": "pipe", "command": "echo blocked runtime", "timeout_ms": 5000},
		},
		"verify": map[string]any{
			"mode": "all",
			"checks": []map[string]any{
				{"kind": "exit_code", "args": map[string]any{"allowed": []any{0}}},
			},
		},
	}

	mustWrite(wsEnvelope{ID: "5", Type: "request", Action: "plan.create", Payload: map[string]any{"session_id": sessionID, "change_reason": "blocked runtime test", "steps": []any{step}}})
	if ok, _ := mustRecvMap(t, conn)["ok"].(bool); !ok {
		t.Fatalf("plan.create failed")
	}

	mustWrite(wsEnvelope{ID: "6", Type: "request", Action: "step.run", Payload: map[string]any{"session_id": sessionID, "step": step}})
	initial := mustRecvMap(t, conn)
	if ok, _ := initial["ok"].(bool); !ok {
		t.Fatalf("step.run failed: %#v", initial)
	}
	execution := initial["result"].(map[string]any)["execution"].(map[string]any)
	pendingApproval := execution["pending_approval"].(map[string]any)
	approvalID := pendingApproval["approval_id"]

	mustWrite(wsEnvelope{ID: "7", Type: "request", Action: "blocked_runtime.list"})
	listResp := mustRecvMap(t, conn)
	if ok, _ := listResp["ok"].(bool); !ok {
		t.Fatalf("blocked_runtime.list failed: %#v", listResp)
	}
	items := listResp["result"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected one blocked runtime, got %#v", listResp)
	}

	mustWrite(wsEnvelope{ID: "8", Type: "request", Action: "blocked_runtime.get", Payload: map[string]any{"session_id": sessionID}})
	getResp := mustRecvMap(t, conn)
	if ok, _ := getResp["ok"].(bool); !ok {
		t.Fatalf("blocked_runtime.get failed: %#v", getResp)
	}

	mustWrite(wsEnvelope{ID: "9", Type: "request", Action: "blocked_runtime.get_by_approval", Payload: map[string]any{"approval_id": approvalID}})
	byApproval := mustRecvMap(t, conn)
	if ok, _ := byApproval["ok"].(bool); !ok {
		t.Fatalf("blocked_runtime.get_by_approval failed: %#v", byApproval)
	}

	mustWrite(wsEnvelope{ID: "10", Type: "request", Action: "blocked_runtime_projection.get", Payload: map[string]any{"approval_id": approvalID}})
	projection := mustRecvMap(t, conn)
	if ok, _ := projection["ok"].(bool); !ok {
		t.Fatalf("blocked_runtime_projection.get failed: %#v", projection)
	}
	result := projection["result"].(map[string]any)
	wait := result["wait"].(map[string]any)
	if waitingFor, _ := wait["waiting_for"].(string); waitingFor == "" {
		t.Fatalf("expected waiting_for in projection, got %#v", result)
	}

	mustWrite(wsEnvelope{ID: "11", Type: "request", Action: "blocked_runtime_projection.list"})
	projectionList := mustRecvMap(t, conn)
	if ok, _ := projectionList["ok"].(bool); !ok {
		t.Fatalf("blocked_runtime_projection.list failed: %#v", projectionList)
	}
	projections := projectionList["result"].([]any)
	if len(projections) != 1 {
		t.Fatalf("expected one blocked runtime projection, got %#v", projectionList)
	}
}
