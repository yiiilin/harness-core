package websocket_test

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	gorillaws "github.com/gorilla/websocket"
	adapterws "github.com/yiiilin/harness-core/adapters/websocket"
	"github.com/yiiilin/harness-core/internal/config"
	"github.com/yiiilin/harness-core/internal/postgrestest"
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

type denyAllWebSocketPolicy struct{}

func (denyAllWebSocketPolicy) Evaluate(_ context.Context, _ session.State, _ plan.StepSpec) (permission.Decision, error) {
	return permission.Decision{Action: permission.Deny, Reason: "websocket deny path", MatchedRule: "test/deny"}, nil
}

func TestWebSocketPostgresStepRunHappyPath(t *testing.T) {
	pg := postgrestest.Start(t)
	opts := hruntime.Options{}
	hruntime.RegisterBuiltins(&opts)
	rt, db := pg.OpenService(t, opts)
	defer db.Close()

	srv := adapterws.New(config.Config{
		Addr:        "127.0.0.1:0",
		SharedToken: "dev-token",
		StorageMode: "postgres",
		PostgresDSN: pg.DSN,
	}, rt)
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

	mustWrite(wsEnvelope{ID: "2", Type: "request", Action: "runtime.info"})
	infoResp := mustRecvMap(t, conn)
	infoResult := infoResp["result"].(map[string]any)
	if got, _ := infoResult["storage_mode"].(string); got != "postgres" {
		t.Fatalf("expected postgres storage mode, got %#v", infoResult["storage_mode"])
	}

	mustWrite(wsEnvelope{ID: "3", Type: "request", Action: "session.create", Payload: map[string]any{"title": "ws-postgres", "goal": "run durable happy path"}})
	sessionResp := mustRecvMap(t, conn)
	sessionID := sessionResp["result"].(map[string]any)["session_id"].(string)

	mustWrite(wsEnvelope{ID: "4", Type: "request", Action: "task.create", Payload: map[string]any{"task_type": "demo", "goal": "echo hello from durable path"}})
	taskResp := mustRecvMap(t, conn)
	taskID := taskResp["result"].(map[string]any)["task_id"].(string)

	mustWrite(wsEnvelope{ID: "5", Type: "request", Action: "session.attach_task", Payload: map[string]any{"session_id": sessionID, "task_id": taskID}})
	if ok, _ := mustRecvMap(t, conn)["ok"].(bool); !ok {
		t.Fatalf("attach failed")
	}

	step := map[string]any{
		"step_id": "step_1",
		"title":   "echo hello durable",
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
		"on_fail": map[string]any{"strategy": "abort"},
	}

	mustWrite(wsEnvelope{ID: "6", Type: "request", Action: "plan.create", Payload: map[string]any{"session_id": sessionID, "change_reason": "durable plan", "steps": []any{step}}})
	planResp := mustRecvMap(t, conn)
	planID := planResp["result"].(map[string]any)["plan_id"].(string)

	mustWrite(wsEnvelope{ID: "7", Type: "request", Action: "step.run", Payload: map[string]any{"session_id": sessionID, "step": step}})
	stepResp := mustRecvMap(t, conn)
	if ok, _ := stepResp["ok"].(bool); !ok {
		t.Fatalf("step.run failed: %#v", stepResp)
	}

	storedSession, err := rt.GetSession(sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if storedSession.Phase != session.PhaseComplete {
		t.Fatalf("expected complete session, got %s", storedSession.Phase)
	}
	storedTask, err := rt.GetTask(taskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if storedTask.Status != "completed" {
		t.Fatalf("expected completed task, got %s", storedTask.Status)
	}
	storedPlan, err := rt.GetPlan(planID)
	if err != nil {
		t.Fatalf("get plan: %v", err)
	}
	if storedPlan.Status != "completed" {
		t.Fatalf("expected completed plan, got %s", storedPlan.Status)
	}
	if len(storedPlan.Steps) != 1 || storedPlan.Steps[0].Status != "completed" {
		t.Fatalf("expected completed durable step, got %#v", storedPlan.Steps)
	}
	events, err := rt.ListAuditEvents(sessionID)
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if len(events) == 0 {
		t.Fatalf("expected durable audit events")
	}
}

func TestWebSocketPostgresPolicyDenyPath(t *testing.T) {
	pg := postgrestest.Start(t)
	opts := hruntime.Options{}
	hruntime.RegisterBuiltins(&opts)
	opts.Policy = denyAllWebSocketPolicy{}
	rt, db := pg.OpenService(t, opts)
	defer db.Close()

	srv := adapterws.New(config.Config{
		Addr:        "127.0.0.1:0",
		SharedToken: "dev-token",
		StorageMode: "postgres",
		PostgresDSN: pg.DSN,
	}, rt)
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

	mustWrite(wsEnvelope{ID: "2", Type: "request", Action: "session.create", Payload: map[string]any{"title": "ws-postgres-deny", "goal": "deny durable path"}})
	sessionResp := mustRecvMap(t, conn)
	sessionID := sessionResp["result"].(map[string]any)["session_id"].(string)

	mustWrite(wsEnvelope{ID: "3", Type: "request", Action: "task.create", Payload: map[string]any{"task_type": "demo", "goal": "deny durable action"}})
	taskResp := mustRecvMap(t, conn)
	taskID := taskResp["result"].(map[string]any)["task_id"].(string)

	mustWrite(wsEnvelope{ID: "4", Type: "request", Action: "session.attach_task", Payload: map[string]any{"session_id": sessionID, "task_id": taskID}})
	if ok, _ := mustRecvMap(t, conn)["ok"].(bool); !ok {
		t.Fatalf("attach failed")
	}

	step := map[string]any{
		"step_id": "step_deny",
		"title":   "deny durable",
		"action": map[string]any{
			"tool_name": "windows.native",
			"args":      map[string]any{"action": "click"},
		},
		"verify":  map[string]any{"mode": "all"},
		"on_fail": map[string]any{"strategy": "abort"},
	}
	mustWrite(wsEnvelope{ID: "5", Type: "request", Action: "plan.create", Payload: map[string]any{"session_id": sessionID, "change_reason": "durable deny", "steps": []any{step}}})
	if ok, _ := mustRecvMap(t, conn)["ok"].(bool); !ok {
		t.Fatalf("plan.create failed")
	}

	mustWrite(wsEnvelope{ID: "6", Type: "request", Action: "step.run", Payload: map[string]any{"session_id": sessionID, "step": step}})
	stepResp := mustRecvMap(t, conn)
	if ok, _ := stepResp["ok"].(bool); !ok {
		t.Fatalf("step.run failed: %#v", stepResp)
	}

	storedSession, err := rt.GetSession(sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if storedSession.Phase != session.PhaseFailed {
		t.Fatalf("expected failed session, got %s", storedSession.Phase)
	}
	storedTask, err := rt.GetTask(taskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if storedTask.Status != "failed" {
		t.Fatalf("expected failed task, got %s", storedTask.Status)
	}
	events, err := rt.ListAuditEvents(sessionID)
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	foundDenied := false
	for _, event := range events {
		if event.Type == "policy.denied" {
			foundDenied = true
			break
		}
	}
	if !foundDenied {
		t.Fatalf("expected policy.denied event, got %#v", events)
	}
}
