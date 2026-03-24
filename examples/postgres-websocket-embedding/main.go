// Command postgres-websocket-embedding shows a durable embedding that wires the
// public Postgres bootstrap, companion builtins/modules, interactive control,
// and the reference WebSocket adapter together.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	gorillaws "github.com/gorilla/websocket"
	adapterws "github.com/yiiilin/harness-core/adapters/websocket"
	"github.com/yiiilin/harness-core/pkg/harness/builtins"
	hpostgres "github.com/yiiilin/harness-core/pkg/harness/postgres"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
)

type DemoResult struct {
	StorageMode           string
	Address               string
	SessionID             string
	HandleID              string
	InteractiveConfigured bool
	Echo                  string
}

type wsEnvelope struct {
	ID      string      `json:"id,omitempty"`
	Type    string      `json:"type"`
	Action  string      `json:"action,omitempty"`
	Token   string      `json:"token,omitempty"`
	Payload interface{} `json:"payload,omitempty"`
}

func main() {
	dsn := strings.TrimSpace(os.Getenv("HARNESS_POSTGRES_DSN"))
	if dsn == "" {
		panic("HARNESS_POSTGRES_DSN is required")
	}

	cfg := hpostgres.Config{
		DSN:             dsn,
		Schema:          strings.TrimSpace(os.Getenv("HARNESS_POSTGRES_SCHEMA")),
		MaxOpenConns:    4,
		MaxIdleConns:    2,
		ApplyMigrations: true,
	}
	result, err := RunPostgresWebSocketEmbeddingDemo(context.Background(), cfg)
	if err != nil {
		panic(err)
	}

	fmt.Printf("storage: %s\n", result.StorageMode)
	fmt.Printf("addr: %s\n", result.Address)
	fmt.Printf("interactive_configured: %t\n", result.InteractiveConfigured)
	fmt.Printf("session_id: %s\n", result.SessionID)
	fmt.Printf("handle_id: %s\n", result.HandleID)
	fmt.Printf("echo: %s\n", strings.TrimSpace(result.Echo))
}

// RunPostgresWebSocketEmbeddingDemo demonstrates a minimal durable embedder
// stack:
//   - public Postgres bootstrap via pkg/harness/postgres
//   - companion builtins/modules wiring
//   - builtins-provided interactive controller
//   - reference WebSocket adapter exposing interactive control actions
func RunPostgresWebSocketEmbeddingDemo(ctx context.Context, cfg hpostgres.Config) (DemoResult, error) {
	var opts hruntime.Options
	builtins.Register(&opts)
	result := DemoResult{InteractiveConfigured: opts.InteractiveController != nil}

	rt, db, err := hpostgres.OpenServiceWithConfig(ctx, cfg, opts)
	if err != nil {
		return DemoResult{}, err
	}
	defer db.Close()
	result.StorageMode = rt.StorageMode

	server := adapterws.New(adapterws.Config{
		Addr:        "127.0.0.1:0",
		SharedToken: "dev-token",
	}, rt)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return DemoResult{}, err
	}
	defer listener.Close()
	result.Address = listener.Addr().String()

	httpServer := &http.Server{Handler: server.Handler()}
	go func() {
		_ = httpServer.Serve(listener)
	}()
	defer httpServer.Shutdown(ctx)

	conn, _, err := gorillaws.DefaultDialer.Dial("ws://"+listener.Addr().String()+"/ws", nil)
	if err != nil {
		return DemoResult{}, err
	}
	defer conn.Close()

	if err := writeEnvelope(conn, wsEnvelope{ID: "1", Type: "auth", Token: "dev-token"}); err != nil {
		return DemoResult{}, err
	}
	if _, err := readEnvelope(conn); err != nil {
		return DemoResult{}, err
	}

	if err := writeEnvelope(conn, wsEnvelope{ID: "2", Type: "request", Action: "session.create", Payload: map[string]any{
		"title": "postgres-websocket-embedding",
		"goal":  "show durable interactive transport control",
	}}); err != nil {
		return DemoResult{}, err
	}
	sessionResp, err := readEnvelope(conn)
	if err != nil {
		return DemoResult{}, err
	}
	result.SessionID = nestedString(sessionResp, "result", "session_id")

	if err := writeEnvelope(conn, wsEnvelope{ID: "3", Type: "request", Action: "interactive.start", Payload: map[string]any{
		"session_id": result.SessionID,
		"request": map[string]any{
			"kind": "pty",
			"spec": map[string]any{"command": "cat"},
		},
	}}); err != nil {
		return DemoResult{}, err
	}
	started, err := readEnvelope(conn)
	if err != nil {
		return DemoResult{}, err
	}
	result.HandleID = nestedString(started, "result", "handle", "handle_id")

	if err := writeEnvelope(conn, wsEnvelope{ID: "4", Type: "request", Action: "interactive.write", Payload: map[string]any{
		"handle_id": result.HandleID,
		"request":   map[string]any{"input": "hello from postgres websocket example\n"},
	}}); err != nil {
		return DemoResult{}, err
	}
	if _, err := readEnvelope(conn); err != nil {
		return DemoResult{}, err
	}

	for range 10 {
		if err := writeEnvelope(conn, wsEnvelope{ID: "5", Type: "request", Action: "interactive.view", Payload: map[string]any{
			"handle_id": result.HandleID,
			"request":   map[string]any{"offset": 0, "max_bytes": 4096},
		}}); err != nil {
			return DemoResult{}, err
		}
		viewed, err := readEnvelope(conn)
		if err != nil {
			return DemoResult{}, err
		}
		result.Echo = nestedString(viewed, "result", "data")
		if strings.Contains(result.Echo, "hello from postgres websocket example") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if err := writeEnvelope(conn, wsEnvelope{ID: "6", Type: "request", Action: "interactive.close", Payload: map[string]any{
		"handle_id": result.HandleID,
		"request":   map[string]any{"reason": "example finished"},
	}}); err != nil {
		return DemoResult{}, err
	}
	if _, err := readEnvelope(conn); err != nil {
		return DemoResult{}, err
	}

	return result, nil
}

func writeEnvelope(conn *gorillaws.Conn, payload wsEnvelope) error {
	return conn.WriteJSON(payload)
}

func readEnvelope(conn *gorillaws.Conn) (map[string]any, error) {
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(msg, &out); err != nil {
		return nil, err
	}
	if ok, _ := out["ok"].(bool); !ok {
		return nil, fmt.Errorf("websocket request failed: %#v", out)
	}
	return out, nil
}

func nestedString(root map[string]any, keys ...string) string {
	current := any(root)
	for _, key := range keys {
		next, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = next[key]
	}
	text, _ := current.(string)
	return text
}
