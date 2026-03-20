package websocket_test

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	gorillaws "github.com/gorilla/websocket"
	adapterws "github.com/yiiilin/harness-core/adapters/websocket"
	"github.com/yiiilin/harness-core/internal/config"
	"github.com/yiiilin/harness-core/pkg/harness/builtins"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
)

type wsEnvelope struct {
	ID      string      `json:"id,omitempty"`
	Type    string      `json:"type"`
	Action  string      `json:"action,omitempty"`
	Token   string      `json:"token,omitempty"`
	Payload interface{} `json:"payload,omitempty"`
}

func mustRecvMap(t *testing.T, conn *gorillaws.Conn) map[string]any {
	t.Helper()
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read message: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(msg, &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	return out
}

func TestWebSocketStepRunHappyPath(t *testing.T) {
	opts := hruntime.Options{}
	builtins.Register(&opts)
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

	mustWrite(wsEnvelope{ID: "2", Type: "request", Action: "session.create", Payload: map[string]any{"title": "ws-step", "goal": "step.run happy path"}})
	sessionResp := mustRecvMap(t, conn)
	sessionID := sessionResp["result"].(map[string]any)["session_id"]

	mustWrite(wsEnvelope{ID: "3", Type: "request", Action: "task.create", Payload: map[string]any{"task_type": "demo", "goal": "run one shell step"}})
	taskResp := mustRecvMap(t, conn)
	taskID := taskResp["result"].(map[string]any)["task_id"]

	mustWrite(wsEnvelope{ID: "4", Type: "request", Action: "session.attach_task", Payload: map[string]any{"session_id": sessionID, "task_id": taskID}})
	attachResp := mustRecvMap(t, conn)
	if ok, _ := attachResp["ok"].(bool); !ok {
		t.Fatalf("attach failed: %#v", attachResp)
	}

	step := map[string]any{
		"step_id": "step_1",
		"title":   "echo hello",
		"action": map[string]any{
			"tool_name": "shell.exec",
			"args":      map[string]any{"mode": "pipe", "command": "echo hello", "timeout_ms": 5000},
		},
		"verify": map[string]any{
			"mode": "all",
			"checks": []map[string]any{
				{"kind": "exit_code", "args": map[string]any{"allowed": []any{0}}},
				{"kind": "output_contains", "args": map[string]any{"text": "hello"}},
			},
		},
	}
	mustWrite(wsEnvelope{ID: "5", Type: "request", Action: "step.run", Payload: map[string]any{"session_id": sessionID, "step": step}})
	stepResp := mustRecvMap(t, conn)
	if ok, _ := stepResp["ok"].(bool); !ok {
		t.Fatalf("step.run failed: %#v", stepResp)
	}
	result, _ := stepResp["result"].(map[string]any)
	if result == nil {
		t.Fatalf("missing result payload")
	}
	execPart, _ := result["execution"].(map[string]any)
	if execPart == nil {
		t.Fatalf("missing execution payload")
	}
	verifyPart, _ := execPart["verify"].(map[string]any)
	if verifyPart == nil {
		t.Fatalf("missing verify payload")
	}
	if success, _ := verifyPart["success"].(bool); !success {
		t.Fatalf("expected verify success, got %#v", verifyPart)
	}
}
