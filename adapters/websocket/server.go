package websocket

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	gorillaws "github.com/gorilla/websocket"
	"github.com/yiiilin/harness-core/internal/auth"
	"github.com/yiiilin/harness-core/internal/protocol"
	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type Server struct {
	cfg      Config
	runtime  *hruntime.Service
	upgrader gorillaws.Upgrader
}

func New(cfg Config, runtime *hruntime.Service) *Server {
	return &Server{
		cfg:      cfg,
		runtime:  runtime,
		upgrader: gorillaws.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }},
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.health)
	mux.HandleFunc("/ws", s.ws)
	return mux
}

func (s *Server) ListenAndServe() error {
	log.Printf("harness-core websocket adapter listening on %s", s.cfg.Addr)
	return http.ListenAndServe(s.cfg.Addr, s.Handler())
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
	case "runtime.metrics":
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: s.runtime.MetricsSnapshot()})
	case "session.ping":
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: s.runtime.Ping()})
	case "session.create":
		var payload protocol.SessionCreatePayload
		_ = json.Unmarshal(env.Payload, &payload)
		sess, err := s.runtime.CreateSession(payload.Title, payload.Goal)
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "SESSION_CREATE_FAILED", Message: err.Error()}})
			return
		}
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
		items, err := s.runtime.ListSessions()
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "SESSION_LIST_FAILED", Message: err.Error()}})
			return
		}
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: items})
	case "task.create":
		var payload protocol.TaskCreatePayload
		_ = json.Unmarshal(env.Payload, &payload)
		tsk, err := s.runtime.CreateTask(task.Spec{TaskType: payload.TaskType, Goal: payload.Goal, Constraints: payload.Constraints, Metadata: payload.Metadata})
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "TASK_CREATE_FAILED", Message: err.Error()}})
			return
		}
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
		items, err := s.runtime.ListTasks()
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "TASK_LIST_FAILED", Message: err.Error()}})
			return
		}
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: items})
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
		items, err := s.runtime.ListPlans(payload.SessionID)
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "PLAN_LIST_FAILED", Message: err.Error()}})
			return
		}
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: items})
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
	case "session.resume":
		var payload protocol.SessionResumePayload
		_ = json.Unmarshal(env.Payload, &payload)
		out, err := s.runtime.ResumePendingApproval(context.Background(), payload.SessionID)
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "SESSION_RESUME_FAILED", Message: err.Error()}})
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
		items, err := s.runtime.ListAuditEvents(payload.SessionID)
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "AUDIT_LIST_FAILED", Message: err.Error()}})
			return
		}
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: items})
	case "attempt.list":
		var payload protocol.SessionScopedPayload
		_ = json.Unmarshal(env.Payload, &payload)
		items, err := s.runtime.ListAttempts(payload.SessionID)
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "ATTEMPT_LIST_FAILED", Message: err.Error()}})
			return
		}
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: items})
	case "action.list":
		var payload protocol.SessionScopedPayload
		_ = json.Unmarshal(env.Payload, &payload)
		items, err := s.runtime.ListActions(payload.SessionID)
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "ACTION_LIST_FAILED", Message: err.Error()}})
			return
		}
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: items})
	case "verification.list":
		var payload protocol.SessionScopedPayload
		_ = json.Unmarshal(env.Payload, &payload)
		items, err := s.runtime.ListVerifications(payload.SessionID)
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "VERIFICATION_LIST_FAILED", Message: err.Error()}})
			return
		}
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: items})
	case "artifact.list":
		var payload protocol.SessionScopedPayload
		_ = json.Unmarshal(env.Payload, &payload)
		items, err := s.runtime.ListArtifacts(payload.SessionID)
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "ARTIFACT_LIST_FAILED", Message: err.Error()}})
			return
		}
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: items})
	case "capability_snapshot.list":
		var payload protocol.SessionScopedPayload
		_ = json.Unmarshal(env.Payload, &payload)
		items, err := s.runtime.ListCapabilitySnapshots(payload.SessionID)
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "CAPABILITY_SNAPSHOT_LIST_FAILED", Message: err.Error()}})
			return
		}
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: items})
	case "context_summary.list":
		var payload protocol.SessionScopedPayload
		_ = json.Unmarshal(env.Payload, &payload)
		items, err := s.runtime.ListContextSummaries(payload.SessionID)
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "CONTEXT_SUMMARY_LIST_FAILED", Message: err.Error()}})
			return
		}
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: items})
	case "runtime_handle.get":
		var payload protocol.RuntimeHandleGetPayload
		_ = json.Unmarshal(env.Payload, &payload)
		rec, err := s.runtime.GetRuntimeHandle(payload.HandleID)
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "NOT_FOUND", Message: err.Error()}})
			return
		}
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: rec})
	case "runtime_handle.list":
		var payload protocol.SessionScopedPayload
		_ = json.Unmarshal(env.Payload, &payload)
		items, err := s.runtime.ListRuntimeHandles(payload.SessionID)
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "RUNTIME_HANDLE_LIST_FAILED", Message: err.Error()}})
			return
		}
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: items})
	case "event.replay":
		var payload protocol.SessionScopedPayload
		_ = json.Unmarshal(env.Payload, &payload)
		items, err := s.runtime.ListAuditEvents(payload.SessionID)
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "EVENT_REPLAY_FAILED", Message: err.Error()}})
			return
		}
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: map[string]any{"count": len(items)}})
		for _, event := range items {
			if err := conn.WriteJSON(map[string]any{
				"id":      env.ID,
				"type":    protocol.EnvelopeTypeEvent,
				"action":  "audit.event",
				"payload": event,
			}); err != nil {
				return
			}
		}
	case "approval.get":
		var payload protocol.ApprovalGetPayload
		_ = json.Unmarshal(env.Payload, &payload)
		rec, err := s.runtime.GetApproval(payload.ApprovalID)
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "NOT_FOUND", Message: err.Error()}})
			return
		}
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: rec})
	case "approval.list":
		var payload protocol.ApprovalListPayload
		_ = json.Unmarshal(env.Payload, &payload)
		items, err := s.runtime.ListApprovals(payload.SessionID)
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "APPROVAL_LIST_FAILED", Message: err.Error()}})
			return
		}
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: items})
	case "approval.respond":
		var payload protocol.ApprovalRespondPayload
		_ = json.Unmarshal(env.Payload, &payload)
		rec, st, err := s.runtime.RespondApproval(payload.ApprovalID, approval.Response{Reply: approval.Reply(payload.Reply), Metadata: payload.Metadata})
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "APPROVAL_RESPOND_FAILED", Message: err.Error()}})
			return
		}
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: map[string]any{"approval": rec, "session": st}})
	case "blocked_runtime.get":
		var payload struct {
			SessionID        string `json:"session_id,omitempty"`
			BlockedRuntimeID string `json:"blocked_runtime_id,omitempty"`
		}
		_ = json.Unmarshal(env.Payload, &payload)
		var (
			rec any
			err error
		)
		switch {
		case payload.BlockedRuntimeID != "":
			rec, err = s.runtime.GetBlockedRuntimeByID(payload.BlockedRuntimeID)
		case payload.SessionID != "":
			rec, err = s.runtime.GetBlockedRuntime(payload.SessionID)
		default:
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "BAD_BLOCKED_RUNTIME", Message: "session_id or blocked_runtime_id is required"}})
			return
		}
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "BLOCKED_RUNTIME_GET_FAILED", Message: err.Error()}})
			return
		}
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: rec})
	case "blocked_runtime.get_by_approval":
		var payload protocol.ApprovalGetPayload
		_ = json.Unmarshal(env.Payload, &payload)
		rec, err := s.runtime.GetBlockedRuntimeByApproval(payload.ApprovalID)
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "BLOCKED_RUNTIME_GET_FAILED", Message: err.Error()}})
			return
		}
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: rec})
	case "blocked_runtime.list":
		items, err := s.runtime.ListBlockedRuntimes()
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "BLOCKED_RUNTIME_LIST_FAILED", Message: err.Error()}})
			return
		}
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: items})
	case "blocked_runtime_projection.get":
		var payload struct {
			SessionID  string `json:"session_id,omitempty"`
			ApprovalID string `json:"approval_id,omitempty"`
		}
		_ = json.Unmarshal(env.Payload, &payload)
		var (
			rec any
			err error
		)
		switch {
		case payload.ApprovalID != "":
			rec, err = s.runtime.GetBlockedRuntimeProjectionByApproval(payload.ApprovalID)
		case payload.SessionID != "":
			rec, err = s.runtime.GetBlockedRuntimeProjection(payload.SessionID)
		default:
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "BAD_BLOCKED_RUNTIME_PROJECTION", Message: "session_id or approval_id is required"}})
			return
		}
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "BLOCKED_RUNTIME_PROJECTION_FAILED", Message: err.Error()}})
			return
		}
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: rec})
	case "blocked_runtime_projection.list":
		items, err := s.runtime.ListBlockedRuntimeProjections()
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "BLOCKED_RUNTIME_PROJECTION_LIST_FAILED", Message: err.Error()}})
			return
		}
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: items})
	case "interactive.get":
		var payload protocol.RuntimeHandleGetPayload
		_ = json.Unmarshal(env.Payload, &payload)
		rec, err := s.runtime.GetInteractiveRuntime(payload.HandleID)
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "INTERACTIVE_GET_FAILED", Message: err.Error()}})
			return
		}
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: rec})
	case "interactive.list":
		var payload protocol.SessionScopedPayload
		_ = json.Unmarshal(env.Payload, &payload)
		items, err := s.runtime.ListInteractiveRuntimes(payload.SessionID)
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "INTERACTIVE_LIST_FAILED", Message: err.Error()}})
			return
		}
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: items})
	case "interactive.start":
		var payload struct {
			SessionID string                           `json:"session_id"`
			Request   hruntime.InteractiveStartRequest `json:"request"`
		}
		_ = json.Unmarshal(env.Payload, &payload)
		rec, err := s.runtime.StartInteractive(context.Background(), payload.SessionID, payload.Request)
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "INTERACTIVE_START_FAILED", Message: err.Error()}})
			return
		}
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: rec})
	case "interactive.reopen":
		var payload struct {
			HandleID string                            `json:"handle_id"`
			Request  hruntime.InteractiveReopenRequest `json:"request"`
		}
		_ = json.Unmarshal(env.Payload, &payload)
		rec, err := s.runtime.ReopenInteractive(context.Background(), payload.HandleID, payload.Request)
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "INTERACTIVE_REOPEN_FAILED", Message: err.Error()}})
			return
		}
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: rec})
	case "interactive.view":
		var payload struct {
			HandleID string                          `json:"handle_id"`
			Request  hruntime.InteractiveViewRequest `json:"request"`
		}
		_ = json.Unmarshal(env.Payload, &payload)
		rec, err := s.runtime.ViewInteractive(context.Background(), payload.HandleID, payload.Request)
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "INTERACTIVE_VIEW_FAILED", Message: err.Error()}})
			return
		}
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: rec})
	case "interactive.write":
		var payload struct {
			HandleID string                           `json:"handle_id"`
			Request  hruntime.InteractiveWriteRequest `json:"request"`
		}
		_ = json.Unmarshal(env.Payload, &payload)
		rec, err := s.runtime.WriteInteractive(context.Background(), payload.HandleID, payload.Request)
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "INTERACTIVE_WRITE_FAILED", Message: err.Error()}})
			return
		}
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: rec})
	case "interactive.close":
		var payload struct {
			HandleID string                           `json:"handle_id"`
			Request  hruntime.InteractiveCloseRequest `json:"request"`
		}
		_ = json.Unmarshal(env.Payload, &payload)
		rec, err := s.runtime.CloseInteractive(context.Background(), payload.HandleID, payload.Request)
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: false, Error: &protocol.ErrorBody{Code: "INTERACTIVE_CLOSE_FAILED", Message: err.Error()}})
			return
		}
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: protocol.EnvelopeTypeResponse, OK: true, Result: rec})
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
