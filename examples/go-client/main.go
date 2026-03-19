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

	mustSend(conn, Envelope{ID: "2", Type: "request", Action: "runtime.info"})
	fmt.Printf("runtime.info => %#v\n", mustRecv(conn))

	mustSend(conn, Envelope{ID: "3", Type: "request", Action: "session.create", Payload: map[string]any{"title": "demo", "goal": "verify harness-core ws"}})
	createResp := mustRecv(conn)
	fmt.Printf("session.create => %#v\n", createResp)
	sessionID := createResp["result"].(map[string]any)["session_id"]

	mustSend(conn, Envelope{ID: "4", Type: "request", Action: "task.create", Payload: map[string]any{"task_type": "demo", "goal": "validate task/session wiring"}})
	taskResp := mustRecv(conn)
	fmt.Printf("task.create => %#v\n", taskResp)
	taskID := taskResp["result"].(map[string]any)["task_id"]

	mustSend(conn, Envelope{ID: "5", Type: "request", Action: "session.attach_task", Payload: map[string]any{"session_id": sessionID, "task_id": taskID}})
	fmt.Printf("session.attach_task => %#v\n", mustRecv(conn))

	mustSend(conn, Envelope{ID: "6", Type: "request", Action: "task.list"})
	fmt.Printf("task.list => %#v\n", mustRecv(conn))

	mustSend(conn, Envelope{ID: "7", Type: "request", Action: "tool.list"})
	fmt.Printf("tool.list => %#v\n", mustRecv(conn))

	mustSend(conn, Envelope{ID: "8", Type: "request", Action: "verify.list"})
	fmt.Printf("verify.list => %#v\n", mustRecv(conn))
}
