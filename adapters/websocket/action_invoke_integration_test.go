package websocket_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	gorillaws "github.com/gorilla/websocket"
	adapterws "github.com/yiiilin/harness-core/adapters/websocket"
	"github.com/yiiilin/harness-core/internal/config"
	"github.com/yiiilin/harness-core/pkg/harness/builtins"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
)

func TestWebSocketActionInvokeRejected(t *testing.T) {
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

	mustWrite(wsEnvelope{
		ID:     "2",
		Type:   "request",
		Action: "action.invoke",
		Payload: map[string]any{
			"tool_name": "shell.exec",
			"args":      map[string]any{"mode": "pipe", "command": "echo bypass"},
		},
	})
	resp := mustRecvMap(t, conn)
	if ok, _ := resp["ok"].(bool); ok {
		t.Fatalf("expected action.invoke to be rejected, got %#v", resp)
	}
}
