package websocket_test

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	gorillaws "github.com/gorilla/websocket"
	adapterws "github.com/yiiilin/harness-core/adapters/websocket"
	"github.com/yiiilin/harness-core/internal/config"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
)

type envelope struct {
	ID      string      `json:"id,omitempty"`
	Type    string      `json:"type"`
	Action  string      `json:"action,omitempty"`
	Token   string      `json:"token,omitempty"`
	Payload interface{} `json:"payload,omitempty"`
}

func mustRecv(t *testing.T, conn *gorillaws.Conn) map[string]any {
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

func TestWebSocketHappyPath(t *testing.T) {
	opts := hruntime.Options{}
	hruntime.RegisterBuiltins(&opts)
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

	if err := conn.WriteJSON(envelope{ID: "1", Type: "auth", Token: "dev-token"}); err != nil {
		t.Fatalf("auth write: %v", err)
	}
	authResp := mustRecv(t, conn)
	if ok, _ := authResp["ok"].(bool); !ok {
		t.Fatalf("expected auth ok, got %#v", authResp)
	}

	if err := conn.WriteJSON(envelope{ID: "2", Type: "request", Action: "runtime.info"}); err != nil {
		t.Fatalf("runtime.info write: %v", err)
	}
	infoResp := mustRecv(t, conn)
	if ok, _ := infoResp["ok"].(bool); !ok {
		t.Fatalf("expected runtime.info ok, got %#v", infoResp)
	}

	if err := conn.WriteJSON(envelope{ID: "3", Type: "request", Action: "session.create", Payload: map[string]any{"title": "ws-test", "goal": "check websocket path"}}); err != nil {
		t.Fatalf("session.create write: %v", err)
	}
	createResp := mustRecv(t, conn)
	if ok, _ := createResp["ok"].(bool); !ok {
		t.Fatalf("expected session.create ok, got %#v", createResp)
	}
	result, _ := createResp["result"].(map[string]any)
	if result == nil || result["session_id"] == nil {
		t.Fatalf("expected session_id in result, got %#v", createResp)
	}
}
