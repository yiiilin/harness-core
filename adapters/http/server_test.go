package httpadapter_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	httpadapter "github.com/yiiilin/harness-core/adapters/http"
	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/builtins"
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

func TestHTTPAdapterHealthAndRuntimeInfo(t *testing.T) {
	handler := newTestServer(t)

	health := httptest.NewRecorder()
	handler.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("expected 200 healthz, got %d", health.Code)
	}

	info := httptest.NewRecorder()
	handler.ServeHTTP(info, httptest.NewRequest(http.MethodGet, "/runtime/info", nil))
	if info.Code != http.StatusOK {
		t.Fatalf("expected 200 runtime info, got %d", info.Code)
	}
}

func TestHTTPAdapterLifecycleAndRunStep(t *testing.T) {
	handler := newTestServer(t)

	sessionBody := decodeMap(t, postJSON(t, handler, "/sessions", map[string]any{
		"title": "http-adapter",
		"goal":  "run one step over http",
	}).Body.Bytes())
	sessionID, _ := sessionBody["session_id"].(string)
	if sessionID == "" {
		t.Fatalf("expected session id, got %#v", sessionBody)
	}

	taskBody := decodeMap(t, postJSON(t, handler, "/tasks", map[string]any{
		"task_type": "demo",
		"goal":      "echo over http",
	}).Body.Bytes())
	taskID, _ := taskBody["task_id"].(string)
	if taskID == "" {
		t.Fatalf("expected task id, got %#v", taskBody)
	}

	attach := postJSON(t, handler, "/sessions/"+sessionID+"/attach-task", map[string]any{
		"task_id": taskID,
	})
	if attach.Code != http.StatusOK {
		t.Fatalf("expected attach success, got %d: %s", attach.Code, attach.Body.String())
	}

	planResp := postJSON(t, handler, "/plans", map[string]any{
		"session_id":    sessionID,
		"change_reason": "http adapter demo",
		"steps": []map[string]any{{
			"step_id": "step_http_adapter",
			"title":   "run echo",
			"action": map[string]any{
				"tool_name": "shell.exec",
				"args": map[string]any{
					"mode":       "pipe",
					"command":    "echo hello from http adapter",
					"timeout_ms": 5000,
				},
			},
			"verify": map[string]any{
				"mode": "all",
				"checks": []map[string]any{
					{"kind": "exit_code", "args": map[string]any{"allowed": []any{0}}},
				},
			},
		}},
	})
	if planResp.Code != http.StatusOK {
		t.Fatalf("expected plan success, got %d: %s", planResp.Code, planResp.Body.String())
	}

	runResp := postJSON(t, handler, "/sessions/"+sessionID+"/steps/run", map[string]any{
		"step": map[string]any{
			"step_id": "step_http_adapter",
			"title":   "run echo",
			"action": map[string]any{
				"tool_name": "shell.exec",
				"args": map[string]any{
					"mode":       "pipe",
					"command":    "echo hello from http adapter",
					"timeout_ms": 5000,
				},
			},
			"verify": map[string]any{
				"mode": "all",
				"checks": []map[string]any{
					{"kind": "exit_code", "args": map[string]any{"allowed": []any{0}}},
				},
			},
		},
	})
	if runResp.Code != http.StatusOK {
		t.Fatalf("expected run-step success, got %d: %s", runResp.Code, runResp.Body.String())
	}
}

func TestHTTPAdapterClaimLeaseAndRunClaimedSession(t *testing.T) {
	rt := newTestRuntime(t)
	handler := httpadapter.New(rt)
	sess, _ := seedPipeSession(t, rt, "claim-run", "echo claimed over http")

	claimResp := postJSON(t, handler, "/sessions/claim/runnable", map[string]any{
		"lease_ttl_ms": 60000,
	})
	if claimResp.Code != http.StatusOK {
		t.Fatalf("expected claim success, got %d: %s", claimResp.Code, claimResp.Body.String())
	}

	var claimed claimSessionResponse
	decodeJSON(t, claimResp.Body.Bytes(), &claimed)
	if !claimed.OK {
		t.Fatalf("expected runnable claim to succeed, got %#v", claimed)
	}
	if claimed.Session.SessionID != sess.SessionID {
		t.Fatalf("expected claimed session %s, got %#v", sess.SessionID, claimed)
	}
	if claimed.Session.LeaseID == "" {
		t.Fatalf("expected claimed lease id, got %#v", claimed.Session)
	}

	renewResp := postJSON(t, handler, "/sessions/"+sess.SessionID+"/lease/renew", map[string]any{
		"lease_id":     claimed.Session.LeaseID,
		"lease_ttl_ms": 120000,
	})
	if renewResp.Code != http.StatusOK {
		t.Fatalf("expected renew success, got %d: %s", renewResp.Code, renewResp.Body.String())
	}
	var renewed session.State
	decodeJSON(t, renewResp.Body.Bytes(), &renewed)
	if renewed.LeaseID != claimed.Session.LeaseID {
		t.Fatalf("expected lease id to stay %s, got %#v", claimed.Session.LeaseID, renewed)
	}

	runResp := postJSON(t, handler, "/sessions/"+sess.SessionID+"/run-claimed", map[string]any{
		"lease_id": claimed.Session.LeaseID,
	})
	if runResp.Code != http.StatusOK {
		t.Fatalf("expected run-claimed success, got %d: %s", runResp.Code, runResp.Body.String())
	}
	var runOut hruntime.SessionRunOutput
	decodeJSON(t, runResp.Body.Bytes(), &runOut)
	if runOut.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected claimed session complete, got %#v", runOut.Session)
	}
	if len(runOut.Executions) != 1 {
		t.Fatalf("expected one claimed execution, got %#v", runOut)
	}

	releaseResp := postJSON(t, handler, "/sessions/"+sess.SessionID+"/lease/release", map[string]any{
		"lease_id": claimed.Session.LeaseID,
	})
	if releaseResp.Code != http.StatusOK {
		t.Fatalf("expected release success, got %d: %s", releaseResp.Code, releaseResp.Body.String())
	}
	var released session.State
	decodeJSON(t, releaseResp.Body.Bytes(), &released)
	if released.LeaseID != "" {
		t.Fatalf("expected release to clear lease, got %#v", released)
	}
}

func TestHTTPAdapterClaimRecoverableAndRecoverClaimedSession(t *testing.T) {
	rt := newTestRuntime(t)
	handler := httpadapter.New(rt)
	sess, step := seedPipeSession(t, rt, "claim-recover", "echo recovered over http")

	claimed, ok, err := rt.ClaimRunnableSession(context.Background(), time.Minute)
	if err != nil {
		t.Fatalf("claim runnable for recovery seed: %v", err)
	}
	if !ok || claimed.SessionID != sess.SessionID {
		t.Fatalf("expected to claim seeded session %s, got %#v ok=%v", sess.SessionID, claimed, ok)
	}
	if _, err := rt.MarkClaimedSessionInFlight(context.Background(), sess.SessionID, claimed.LeaseID, step.StepID); err != nil {
		t.Fatalf("mark in flight: %v", err)
	}
	if _, err := rt.MarkClaimedSessionInterrupted(context.Background(), sess.SessionID, claimed.LeaseID); err != nil {
		t.Fatalf("mark interrupted: %v", err)
	}
	if _, err := rt.ReleaseSessionLease(context.Background(), sess.SessionID, claimed.LeaseID); err != nil {
		t.Fatalf("release seed lease: %v", err)
	}

	claimResp := postJSON(t, handler, "/sessions/claim/recoverable", map[string]any{
		"lease_ttl_ms": 60000,
	})
	if claimResp.Code != http.StatusOK {
		t.Fatalf("expected recoverable claim success, got %d: %s", claimResp.Code, claimResp.Body.String())
	}

	var recoverable claimSessionResponse
	decodeJSON(t, claimResp.Body.Bytes(), &recoverable)
	if !recoverable.OK || recoverable.Session.SessionID != sess.SessionID {
		t.Fatalf("expected recoverable session %s, got %#v", sess.SessionID, recoverable)
	}

	recoverResp := postJSON(t, handler, "/sessions/"+sess.SessionID+"/recover-claimed", map[string]any{
		"lease_id": recoverable.Session.LeaseID,
	})
	if recoverResp.Code != http.StatusOK {
		t.Fatalf("expected recover-claimed success, got %d: %s", recoverResp.Code, recoverResp.Body.String())
	}
	var recoverOut hruntime.SessionRunOutput
	decodeJSON(t, recoverResp.Body.Bytes(), &recoverOut)
	if recoverOut.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected recovered session complete, got %#v", recoverOut.Session)
	}
	if len(recoverOut.Executions) != 1 {
		t.Fatalf("expected one recovered execution, got %#v", recoverOut)
	}
}

func TestHTTPAdapterResumeClaimedApproval(t *testing.T) {
	var opts hruntime.Options
	opts.Policy = askAllPolicy{}
	builtins.Register(&opts)
	rt := hruntime.New(opts)
	handler := httpadapter.New(rt)

	sess, step := seedPipeSession(t, rt, "resume-claimed-approval", "echo resumed over claimed approval")
	initial, err := rt.RunStep(context.Background(), sess.SessionID, step)
	if err != nil {
		t.Fatalf("run step into approval: %v", err)
	}
	if initial.Execution.PendingApproval == nil {
		t.Fatalf("expected pending approval, got %#v", initial.Execution)
	}
	if _, _, err := rt.RespondApproval(initial.Execution.PendingApproval.ApprovalID, approval.Response{Reply: approval.ReplyOnce}); err != nil {
		t.Fatalf("approve step: %v", err)
	}

	claimResp := postJSON(t, handler, "/sessions/claim/runnable", map[string]any{
		"lease_ttl_ms": 60000,
	})
	if claimResp.Code != http.StatusOK {
		t.Fatalf("expected runnable claim for approved session, got %d: %s", claimResp.Code, claimResp.Body.String())
	}
	var claimed claimSessionResponse
	decodeJSON(t, claimResp.Body.Bytes(), &claimed)
	if !claimed.OK || claimed.Session.SessionID != sess.SessionID {
		t.Fatalf("expected approved pending session %s to be claimed, got %#v", sess.SessionID, claimed)
	}

	resumeResp := postJSON(t, handler, "/sessions/"+sess.SessionID+"/approval/resume-claimed", map[string]any{
		"lease_id": claimed.Session.LeaseID,
	})
	if resumeResp.Code != http.StatusOK {
		t.Fatalf("expected claimed approval resume success, got %d: %s", resumeResp.Code, resumeResp.Body.String())
	}
	var resumed hruntime.StepRunOutput
	decodeJSON(t, resumeResp.Body.Bytes(), &resumed)
	if resumed.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected resumed approval session complete, got %#v", resumed.Session)
	}
	if resumed.Session.PendingApprovalID != "" {
		t.Fatalf("expected pending approval cleared after claimed resume, got %#v", resumed.Session)
	}
}

type claimSessionResponse struct {
	Session session.State `json:"session"`
	OK      bool          `json:"ok"`
}

func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	return httpadapter.New(newTestRuntime(t))
}

func newTestRuntime(t *testing.T) *hruntime.Service {
	t.Helper()
	var opts hruntime.Options
	builtins.Register(&opts)
	return hruntime.New(opts)
}

func postJSON(t *testing.T, handler http.Handler, path string, payload any) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req.WithContext(context.Background()))
	return rr
}

func decodeMap(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	return out
}

func decodeJSON(t *testing.T, raw []byte, target any) {
	t.Helper()
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatalf("decode json: %v", err)
	}
}

func seedPipeSession(t *testing.T, rt *hruntime.Service, title, command string) (session.State, plan.StepSpec) {
	t.Helper()

	sess, err := rt.CreateSession(title, command)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	record, err := rt.CreateTask(task.Spec{TaskType: "demo", Goal: command})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	sess, err = rt.AttachTaskToSession(sess.SessionID, record.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	step := plan.StepSpec{
		StepID: "step_" + title,
		Title:  title,
		Action: action.Spec{
			ToolName: "shell.exec",
			Args: map[string]any{
				"mode":       "pipe",
				"command":    command,
				"timeout_ms": 5000,
			},
		},
		Verify: verify.Spec{
			Mode: verify.ModeAll,
			Checks: []verify.Check{
				{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
			},
		},
	}
	if _, err := rt.CreatePlan(sess.SessionID, "seed session", []plan.StepSpec{step}); err != nil {
		t.Fatalf("create plan: %v", err)
	}
	return sess, step
}

type askAllPolicy struct{}

func (askAllPolicy) Evaluate(_ context.Context, _ session.State, step plan.StepSpec) (permission.Decision, error) {
	return permission.Decision{
		Action:      permission.Ask,
		Reason:      "approval required for adapter test",
		MatchedRule: "test/ask",
	}, nil
}

var _ = plan.StepSpec{}
