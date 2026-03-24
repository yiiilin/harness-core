package websocket_test

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gorillaws "github.com/gorilla/websocket"
	adapterws "github.com/yiiilin/harness-core/adapters/websocket"
	"github.com/yiiilin/harness-core/pkg/harness/builtins"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
)

func TestWebSocketInteractiveControlFlow(t *testing.T) {
	opts := hruntime.Options{}
	builtins.Register(&opts)
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

	mustWrite(wsEnvelope{ID: "2", Type: "request", Action: "session.create", Payload: map[string]any{"title": "ws-interactive", "goal": "exercise interactive websocket control"}})
	sessionResp := mustRecvMap(t, conn)
	sessionID := sessionResp["result"].(map[string]any)["session_id"].(string)

	mustWrite(wsEnvelope{
		ID:     "3",
		Type:   "request",
		Action: "interactive.start",
		Payload: map[string]any{
			"session_id": sessionID,
			"request": map[string]any{
				"kind": "pty",
				"spec": map[string]any{"command": "cat"},
			},
		},
	})
	started := mustRecvMap(t, conn)
	if ok, _ := started["ok"].(bool); !ok {
		t.Fatalf("interactive.start failed: %#v", started)
	}
	startedRuntime := started["result"].(map[string]any)
	handleID := startedRuntime["handle"].(map[string]any)["handle_id"].(string)

	mustWrite(wsEnvelope{ID: "4", Type: "request", Action: "interactive.list", Payload: map[string]any{"session_id": sessionID}})
	listResp := mustRecvMap(t, conn)
	if ok, _ := listResp["ok"].(bool); !ok {
		t.Fatalf("interactive.list failed: %#v", listResp)
	}
	items := listResp["result"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected one interactive runtime, got %#v", listResp)
	}

	mustWrite(wsEnvelope{
		ID:     "5",
		Type:   "request",
		Action: "interactive.write",
		Payload: map[string]any{
			"handle_id": handleID,
			"request":   map[string]any{"input": "hello over websocket\n"},
		},
	})
	written := mustRecvMap(t, conn)
	if ok, _ := written["ok"].(bool); !ok {
		t.Fatalf("interactive.write failed: %#v", written)
	}

	var viewed map[string]any
	for range 10 {
		mustWrite(wsEnvelope{
			ID:     "6",
			Type:   "request",
			Action: "interactive.view",
			Payload: map[string]any{
				"handle_id": handleID,
				"request":   map[string]any{"offset": 0, "max_bytes": 4096},
			},
		})
		viewed = mustRecvMap(t, conn)
		if ok, _ := viewed["ok"].(bool); !ok {
			t.Fatalf("interactive.view failed: %#v", viewed)
		}
		data, _ := viewed["result"].(map[string]any)["data"].(string)
		if strings.Contains(data, "hello over websocket") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	data, _ := viewed["result"].(map[string]any)["data"].(string)
	if !strings.Contains(data, "hello over websocket") {
		t.Fatalf("expected interactive output to contain written text, got %#v", viewed)
	}

	mustWrite(wsEnvelope{
		ID:     "7",
		Type:   "request",
		Action: "interactive.reopen",
		Payload: map[string]any{
			"handle_id": handleID,
			"request":   map[string]any{},
		},
	})
	reopened := mustRecvMap(t, conn)
	if ok, _ := reopened["ok"].(bool); !ok {
		t.Fatalf("interactive.reopen failed: %#v", reopened)
	}

	mustWrite(wsEnvelope{ID: "8", Type: "request", Action: "interactive.get", Payload: map[string]any{"handle_id": handleID}})
	getResp := mustRecvMap(t, conn)
	if ok, _ := getResp["ok"].(bool); !ok {
		t.Fatalf("interactive.get failed: %#v", getResp)
	}

	mustWrite(wsEnvelope{
		ID:     "9",
		Type:   "request",
		Action: "interactive.close",
		Payload: map[string]any{
			"handle_id": handleID,
			"request":   map[string]any{"reason": "done"},
		},
	})
	closed := mustRecvMap(t, conn)
	if ok, _ := closed["ok"].(bool); !ok {
		t.Fatalf("interactive.close failed: %#v", closed)
	}
	closedResult := closed["result"].(map[string]any)
	handle := closedResult["handle"].(map[string]any)
	if got, _ := handle["status"].(string); got != "closed" {
		t.Fatalf("expected closed interactive handle, got %#v", closedResult)
	}
}
