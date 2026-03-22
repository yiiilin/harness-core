package websocket_test

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	gorillaws "github.com/gorilla/websocket"
	adapterws "github.com/yiiilin/harness-core/adapters/websocket"
	"github.com/yiiilin/harness-core/pkg/harness/builtins"
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

type askAllWebSocketPolicy struct{}

func (askAllWebSocketPolicy) Evaluate(_ context.Context, _ session.State, _ plan.StepSpec) (permission.Decision, error) {
	return permission.Decision{Action: permission.Ask, Reason: "approval required", MatchedRule: "test/ask"}, nil
}

func TestWebSocketApprovalRespondAndResumeFlow(t *testing.T) {
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

	mustWrite(wsEnvelope{ID: "2", Type: "request", Action: "session.create", Payload: map[string]any{"title": "ws-approval", "goal": "approval flow"}})
	sessionResp := mustRecvMap(t, conn)
	sessionID := sessionResp["result"].(map[string]any)["session_id"]

	mustWrite(wsEnvelope{ID: "3", Type: "request", Action: "task.create", Payload: map[string]any{"task_type": "demo", "goal": "resume after approval"}})
	taskResp := mustRecvMap(t, conn)
	taskID := taskResp["result"].(map[string]any)["task_id"]

	mustWrite(wsEnvelope{ID: "4", Type: "request", Action: "session.attach_task", Payload: map[string]any{"session_id": sessionID, "task_id": taskID}})
	if ok, _ := mustRecvMap(t, conn)["ok"].(bool); !ok {
		t.Fatalf("attach failed")
	}

	step := map[string]any{
		"step_id": "step_ws_approval",
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

	mustWrite(wsEnvelope{ID: "5", Type: "request", Action: "plan.create", Payload: map[string]any{"session_id": sessionID, "change_reason": "approval test", "steps": []any{step}}})
	if ok, _ := mustRecvMap(t, conn)["ok"].(bool); !ok {
		t.Fatalf("plan.create failed")
	}

	mustWrite(wsEnvelope{ID: "6", Type: "request", Action: "step.run", Payload: map[string]any{"session_id": sessionID, "step": step}})
	initial := mustRecvMap(t, conn)
	if ok, _ := initial["ok"].(bool); !ok {
		t.Fatalf("step.run failed: %#v", initial)
	}
	result := initial["result"].(map[string]any)
	execution := result["execution"].(map[string]any)
	pendingApproval := execution["pending_approval"].(map[string]any)
	approvalID := pendingApproval["approval_id"]

	mustWrite(wsEnvelope{ID: "7", Type: "request", Action: "approval.respond", Payload: map[string]any{"approval_id": approvalID, "reply": "once"}})
	response := mustRecvMap(t, conn)
	if ok, _ := response["ok"].(bool); !ok {
		t.Fatalf("approval.respond failed: %#v", response)
	}

	mustWrite(wsEnvelope{ID: "8", Type: "request", Action: "session.resume", Payload: map[string]any{"session_id": sessionID}})
	resume := mustRecvMap(t, conn)
	if ok, _ := resume["ok"].(bool); !ok {
		t.Fatalf("session.resume failed: %#v", resume)
	}
}
