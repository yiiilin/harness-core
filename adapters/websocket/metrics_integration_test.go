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

type metricsEnvelope struct {
	ID      string      `json:"id,omitempty"`
	Type    string      `json:"type"`
	Action  string      `json:"action,omitempty"`
	Token   string      `json:"token,omitempty"`
	Payload interface{} `json:"payload,omitempty"`
}

func recvMap(t *testing.T, conn *gorillaws.Conn) map[string]any {
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

func TestWebSocketRuntimeMetrics(t *testing.T) {
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

	mustWrite := func(v any) {
		if err := conn.WriteJSON(v); err != nil {
			t.Fatalf("write json: %v", err)
		}
	}

	mustWrite(metricsEnvelope{ID: "1", Type: "auth", Token: "dev-token"})
	if ok, _ := recvMap(t, conn)["ok"].(bool); !ok {
		t.Fatalf("auth failed")
	}

	mustWrite(metricsEnvelope{ID: "2", Type: "request", Action: "runtime.metrics"})
	resp := recvMap(t, conn)
	if ok, _ := resp["ok"].(bool); !ok {
		t.Fatalf("runtime.metrics failed: %#v", resp)
	}
	result, _ := resp["result"].(map[string]any)
	if result == nil {
		t.Fatalf("missing metrics result")
	}
	if _, ok := result["step_runs"]; !ok {
		t.Fatalf("expected step_runs in metrics snapshot, got %#v", result)
	}
}
