package websocket

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	gorillaws "github.com/gorilla/websocket"
	"github.com/yiiilin/harness-core/internal/auth"
	"github.com/yiiilin/harness-core/internal/config"
	"github.com/yiiilin/harness-core/internal/protocol"
	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type Server struct {
	cfg      config.Config
	runtime  *hruntime.Service
	upgrader gorillaws.Upgrader
}

func New(cfg config.Config, runtime *hruntime.Service) *Server {
	return &Server{
		cfg:      cfg,
		runtime:  runtime,
		upgrader: gorillaws.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }},
	}
}

func (s *Server) ListenAndServe() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.health)
	mux.HandleFunc("/ws", s.ws)
	log.Printf("harness-core websocket adapter listening on %s", s.cfg.Addr)
	return http.ListenAndServe(s.cfg.Addr, mux)
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (s *Server) ws(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	authed := false
	if auth.ValidToken(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), s.cfg.SharedToken) {
		authed = true
	}
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var env protocol.Envelope
		if err := json.Unmarshal(data, &env); err != nil {
			_ = conn.WriteJSON(protocol.Response{Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "BAD_JSON", Message: err.Error()}})
			continue
		}
		if !authed {
			if env.Type == protocol.EnvelopeTypeAuth && auth.ValidToken(env.Token, s.cfg.SharedToken) {
				authed = true
				_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: map[string]any{"authenticated": true}})
				continue
			}
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "UNAUTHENTICATED", Message: "authenticate first"}})
			continue
		}
		s.handle(conn, env)
	}
}

func (s *Server) handle(conn *gorillaws.Conn, env protocol.Envelope) {
	switch env.Action {
	case "runtime.info":
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: s.runtime.RuntimeInfo()})
	case "session.ping":
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: s.runtime.Ping()})
	case "session.create":
		var payload protocol.SessionCreatePayload
		_ = json.Unmarshal(env.Payload, &payload)
		sess := s.runtime.CreateSession(payload.Title, payload.Goal)
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: sess})
	case "session.get":
		var payload protocol.SessionGetPayload
		_ = json.Unmarshal(env.Payload, &payload)
		sess, err := s.runtime.GetSession(payload.ID)
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "NOT_FOUND", Message: err.Error()}})
			return
		}
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: sess})
	case "session.list":
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: s.runtime.ListSessions()})
	case "task.create":
		var payload protocol.TaskCreatePayload
		_ = json.Unmarshal(env.Payload, &payload)
		tsk := s.runtime.CreateTask(task.Spec{TaskType: payload.TaskType, Goal: payload.Goal, Constraints: payload.Constraints, Metadata: payload.Metadata})
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: tsk})
	case "task.get":
		var payload struct {
			TaskID string `json:"task_id"`
		}
		_ = json.Unmarshal(env.Payload, &payload)
		tsk, err := s.runtime.GetTask(payload.TaskID)
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "NOT_FOUND", Message: err.Error()}})
			return
		}
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: tsk})
	case "task.list":
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: s.runtime.ListTasks()})
	case "session.attach_task":
		var payload protocol.SessionAttachTaskPayload
		_ = json.Unmarshal(env.Payload, &payload)
		sess, err := s.runtime.AttachTaskToSession(payload.SessionID, payload.TaskID)
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "ATTACH_FAILED", Message: err.Error()}})
			return
		}
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: sess})
	case "plan.create":
		var payload protocol.PlanCreatePayload
		_ = json.Unmarshal(env.Payload, &payload)
		steps := make([]plan.StepSpec, 0, len(payload.Steps))
		for _, raw := range payload.Steps {
			var step plan.StepSpec
			if err := json.Unmarshal(raw, &step); err != nil {
				_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "BAD_STEP", Message: err.Error()}})
				return
			}
			steps = append(steps, step)
		}
		pl, err := s.runtime.CreatePlan(payload.SessionID, payload.ChangeReason, steps)
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "PLAN_CREATE_FAILED", Message: err.Error()}})
			return
		}
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: pl})
	case "plan.get":
		var payload protocol.PlanGetPayload
		_ = json.Unmarshal(env.Payload, &payload)
		pl, err := s.runtime.GetPlan(payload.PlanID)
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "NOT_FOUND", Message: err.Error()}})
			return
		}
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: pl})
	case "plan.list":
		var payload protocol.PlanListPayload
		_ = json.Unmarshal(env.Payload, &payload)
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: s.runtime.ListPlans(payload.SessionID)})
	case "step.run":
		var payload protocol.StepRunPayload
		_ = json.Unmarshal(env.Payload, &payload)
		var step plan.StepSpec
		if err := json.Unmarshal(payload.Step, &step); err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "BAD_STEP", Message: err.Error()}})
			return
		}
		out, err := s.runtime.RunStep(context.Background(), payload.SessionID, step)
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "STEP_RUN_FAILED", Message: err.Error()}})
			return
		}
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: out})
	case "action.invoke":
		var spec action.Spec
		if err := json.Unmarshal(env.Payload, &spec); err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "BAD_ACTION", Message: err.Error()}})
			return
		}
		result, err := s.runtime.InvokeAction(context.Background(), spec)
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "ACTION_FAILED", Message: err.Error()}})
			return
		}
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: result})
	case "policy.evaluate":
		var payload struct {
			SessionID string        `json:"session_id"`
			Step      plan.StepSpec `json:"step"`
		}
		_ = json.Unmarshal(env.Payload, &payload)
		state, err := s.runtime.GetSession(payload.SessionID)
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "NOT_FOUND", Message: err.Error()}})
			return
		}
		decision, err := s.runtime.EvaluatePolicy(context.Background(), state, payload.Step)
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "POLICY_FAILED", Message: err.Error()}})
			return
		}
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: decision})
	case "tool.list":
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: s.runtime.ListTools()})
	case "verify.list":
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: s.runtime.ListVerifiers()})
	case "audit.list":
		var payload protocol.AuditListPayload
		_ = json.Unmarshal(env.Payload, &payload)
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: s.runtime.ListAuditEvents(payload.SessionID)})
	case "verify.evaluate":
		var payload struct {
			SessionID string        `json:"session_id"`
			Spec      verify.Spec   `json:"spec"`
			Result    action.Result `json:"result"`
		}
		_ = json.Unmarshal(env.Payload, &payload)
		state, err := s.runtime.GetSession(payload.SessionID)
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "NOT_FOUND", Message: err.Error()}})
			return
		}
		res, err := s.runtime.EvaluateVerify(context.Background(), payload.Spec, payload.Result, state)
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "VERIFY_FAILED", Message: err.Error()}})
			return
		}
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: res})
	default:
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "UNKNOWN_ACTION", Message: "unknown action"}})
	}
}
