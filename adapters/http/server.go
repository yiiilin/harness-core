package httpadapter

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

type Server struct {
	Runtime *hruntime.Service
	mux     *http.ServeMux
}

type createSessionRequest struct {
	Title string `json:"title"`
	Goal  string `json:"goal"`
}

type attachTaskRequest struct {
	TaskID string `json:"task_id"`
}

type createPlanRequest struct {
	SessionID    string          `json:"session_id"`
	ChangeReason string          `json:"change_reason"`
	Steps        []plan.StepSpec `json:"steps"`
}

type runStepRequest struct {
	Step plan.StepSpec `json:"step"`
}

type claimSessionRequest struct {
	LeaseTTLMS int64 `json:"lease_ttl_ms"`
}

type leaseRequest struct {
	LeaseID    string `json:"lease_id"`
	LeaseTTLMS int64  `json:"lease_ttl_ms,omitempty"`
}

type claimedExecutionRequest struct {
	LeaseID string `json:"lease_id"`
}

type claimSessionResponse struct {
	Session session.State `json:"session"`
	OK      bool          `json:"ok"`
}

func New(rt *hruntime.Service) http.Handler {
	server := &Server{Runtime: rt, mux: http.NewServeMux()}
	server.routes()
	return server
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.mux.HandleFunc("GET /runtime/info", s.handleRuntimeInfo)
	s.mux.HandleFunc("POST /sessions", s.handleCreateSession)
	s.mux.HandleFunc("POST /tasks", s.handleCreateTask)
	s.mux.HandleFunc("POST /sessions/{session_id}/attach-task", s.handleAttachTask)
	s.mux.HandleFunc("POST /plans", s.handleCreatePlan)
	s.mux.HandleFunc("POST /sessions/{session_id}/steps/run", s.handleRunStep)
	s.mux.HandleFunc("POST /sessions/claim/runnable", s.handleClaimRunnableSession)
	s.mux.HandleFunc("POST /sessions/claim/recoverable", s.handleClaimRecoverableSession)
	s.mux.HandleFunc("POST /sessions/{session_id}/lease/renew", s.handleRenewSessionLease)
	s.mux.HandleFunc("POST /sessions/{session_id}/lease/release", s.handleReleaseSessionLease)
	s.mux.HandleFunc("POST /sessions/{session_id}/run-claimed", s.handleRunClaimedSession)
	s.mux.HandleFunc("POST /sessions/{session_id}/recover-claimed", s.handleRecoverClaimedSession)
	s.mux.HandleFunc("POST /sessions/{session_id}/approval/resume-claimed", s.handleResumeClaimedApproval)
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"pong": true})
}

func (s *Server) handleRuntimeInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.Runtime.RuntimeInfo())
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var req createSessionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	state, err := s.Runtime.CreateSession(req.Title, req.Goal)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var spec task.Spec
	if err := decodeJSON(r, &spec); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	record, err := s.Runtime.CreateTask(spec)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) handleAttachTask(w http.ResponseWriter, r *http.Request) {
	var req attachTaskRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	state, err := s.Runtime.AttachTaskToSession(r.PathValue("session_id"), req.TaskID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) handleCreatePlan(w http.ResponseWriter, r *http.Request) {
	var req createPlanRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	spec, err := s.Runtime.CreatePlan(req.SessionID, req.ChangeReason, req.Steps)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, spec)
}

func (s *Server) handleRunStep(w http.ResponseWriter, r *http.Request) {
	var req runStepRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	out, err := s.Runtime.RunStep(r.Context(), r.PathValue("session_id"), req.Step)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleClaimRunnableSession(w http.ResponseWriter, r *http.Request) {
	s.handleClaimSession(w, r, s.Runtime.ClaimRunnableSession)
}

func (s *Server) handleClaimRecoverableSession(w http.ResponseWriter, r *http.Request) {
	s.handleClaimSession(w, r, s.Runtime.ClaimRecoverableSession)
}

func (s *Server) handleClaimSession(
	w http.ResponseWriter,
	r *http.Request,
	claim func(rctx context.Context, leaseTTL time.Duration) (session.State, bool, error),
) {
	var req claimSessionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	state, ok, err := claim(r.Context(), time.Duration(req.LeaseTTLMS)*time.Millisecond)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, claimSessionResponse{Session: state, OK: ok})
}

func (s *Server) handleRenewSessionLease(w http.ResponseWriter, r *http.Request) {
	var req leaseRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	state, err := s.Runtime.RenewSessionLease(r.Context(), r.PathValue("session_id"), req.LeaseID, time.Duration(req.LeaseTTLMS)*time.Millisecond)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) handleReleaseSessionLease(w http.ResponseWriter, r *http.Request) {
	var req leaseRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	state, err := s.Runtime.ReleaseSessionLease(r.Context(), r.PathValue("session_id"), req.LeaseID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) handleRunClaimedSession(w http.ResponseWriter, r *http.Request) {
	var req claimedExecutionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	out, err := s.Runtime.RunClaimedSession(r.Context(), r.PathValue("session_id"), req.LeaseID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleRecoverClaimedSession(w http.ResponseWriter, r *http.Request) {
	var req claimedExecutionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	out, err := s.Runtime.RecoverClaimedSession(r.Context(), r.PathValue("session_id"), req.LeaseID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleResumeClaimedApproval(w http.ResponseWriter, r *http.Request) {
	var req claimedExecutionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	out, err := s.Runtime.ResumeClaimedApproval(r.Context(), r.PathValue("session_id"), req.LeaseID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func decodeJSON(r *http.Request, target any) error {
	if r.Body == nil {
		return errors.New("request body is required")
	}
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{
		"error": err.Error(),
	})
}
