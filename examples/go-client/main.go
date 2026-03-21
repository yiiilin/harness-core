// Command go-client exercises the reference WebSocket adapter from an external Go process.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/gorilla/websocket"
)

type Envelope struct {
	ID      string      `json:"id,omitempty"`
	Type    string      `json:"type"`
	Action  string      `json:"action,omitempty"`
	Token   string      `json:"token,omitempty"`
	Payload interface{} `json:"payload,omitempty"`
}

func mustSend(conn *websocket.Conn, v any) {
	if err := conn.WriteJSON(v); err != nil {
		log.Fatal(err)
	}
}

func mustRecv(conn *websocket.Conn) map[string]any {
	_, msg, err := conn.ReadMessage()
	if err != nil {
		log.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(msg, &out); err != nil {
		log.Fatal(err)
	}
	return out
}

// mustRequest sends one request envelope and fails fast if the adapter reports an error.
func mustRequest(conn *websocket.Conn, id, action string, payload any) map[string]any {
	mustSend(conn, Envelope{ID: id, Type: "request", Action: action, Payload: payload})
	out := mustRecv(conn)
	if ok, _ := out["ok"].(bool); !ok {
		log.Fatalf("%s failed: %#v", action, out)
	}
	return out
}

func main() {
	url := os.Getenv("HARNESS_URL")
	if url == "" {
		url = "ws://127.0.0.1:8787/ws"
	}
	token := os.Getenv("HARNESS_TOKEN")
	if token == "" {
		token = "dev-token"
	}
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	mustSend(conn, Envelope{ID: "1", Type: "auth", Token: token})
	fmt.Printf("auth => %#v\n", mustRecv(conn))

	fmt.Printf("runtime.info => %#v\n", mustRequest(conn, "2", "runtime.info", nil))

	createResp := mustRequest(conn, "3", "session.create", map[string]any{"title": "demo", "goal": "verify harness-core ws"})
	fmt.Printf("session.create => %#v\n", createResp)
	sessionID := createResp["result"].(map[string]any)["session_id"]

	taskResp := mustRequest(conn, "4", "task.create", map[string]any{"task_type": "demo", "goal": "validate task/session wiring"})
	fmt.Printf("task.create => %#v\n", taskResp)
	taskID := taskResp["result"].(map[string]any)["task_id"]

	fmt.Printf("session.attach_task => %#v\n", mustRequest(conn, "5", "session.attach_task", map[string]any{"session_id": sessionID, "task_id": taskID}))

	// Create and run one step so the client demonstrates a complete minimal request chain.
	planResp := mustRequest(conn, "6", "plan.create", map[string]any{
		"session_id":    sessionID,
		"change_reason": "client demo",
		"steps": []any{
			map[string]any{
				"step_id": "step_ws_demo",
				"title":   "echo over websocket",
				"action": map[string]any{
					"tool_name": "shell.exec",
					"args": map[string]any{
						"mode":       "pipe",
						"command":    "echo websocket demo",
						"timeout_ms": 5000,
					},
				},
				"verify": map[string]any{
					"mode": "all",
					"checks": []any{
						map[string]any{"kind": "exit_code", "args": map[string]any{"allowed": []any{0}}},
					},
				},
			},
		},
	})
	fmt.Printf("plan.create => %#v\n", planResp)
	step := planResp["result"].(map[string]any)["steps"].([]any)[0]

	fmt.Printf("step.run => %#v\n", mustRequest(conn, "7", "step.run", map[string]any{"session_id": sessionID, "step": step}))
	fmt.Printf("attempt.list => %#v\n", mustRequest(conn, "8", "attempt.list", map[string]any{"session_id": sessionID}))
	fmt.Printf("task.list => %#v\n", mustRequest(conn, "9", "task.list", nil))
	fmt.Printf("tool.list => %#v\n", mustRequest(conn, "10", "tool.list", nil))
	fmt.Printf("verify.list => %#v\n", mustRequest(conn, "11", "verify.list", nil))
	fmt.Printf("runtime.metrics => %#v\n", mustRequest(conn, "12", "runtime.metrics", nil))
}
