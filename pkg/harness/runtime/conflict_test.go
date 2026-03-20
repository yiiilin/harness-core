package runtime_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type staleApprovalUpdateStore struct {
	approval.Store
	mu          sync.Mutex
	targetID    string
	injected    bool
	injectedErr error
}

func (s *staleApprovalUpdateStore) arm(approvalID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.targetID = approvalID
	s.injected = false
	s.injectedErr = nil
}

func (s *staleApprovalUpdateStore) Update(next approval.Record) error {
	s.mu.Lock()
	targetID := s.targetID
	shouldInject := !s.injected && targetID != "" && next.ApprovalID == targetID
	if shouldInject {
		s.injected = true
	}
	s.mu.Unlock()

	if shouldInject {
		latest, err := s.Store.Get(next.ApprovalID)
		if err != nil {
			return err
		}
		latest.Reason = "stale approval race"
		latest.Version++
		if err := s.Store.Update(latest); err != nil {
			return err
		}
	}
	return s.Store.Update(next)
}

type staleSessionUpdateStore struct {
	session.Store
	mu       sync.Mutex
	targetID string
	injected bool
}

func (s *staleSessionUpdateStore) arm(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.targetID = sessionID
	s.injected = false
}

func (s *staleSessionUpdateStore) Update(next session.State) error {
	s.mu.Lock()
	targetID := s.targetID
	shouldInject := !s.injected && targetID != "" && next.SessionID == targetID
	if shouldInject {
		s.injected = true
	}
	s.mu.Unlock()

	if shouldInject {
		latest, err := s.Store.Get(next.SessionID)
		if err != nil {
			return err
		}
		latest.Summary = "stale session race"
		latest.Version++
		if err := s.Store.Update(latest); err != nil {
			return err
		}
	}
	return s.Store.Update(next)
}

type coordinatedSessionUpdateStore struct {
	session.Store
	mu        sync.Mutex
	targetID  string
	remaining int
	release   chan struct{}
}

func (s *coordinatedSessionUpdateStore) arm(sessionID string, count int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.targetID = sessionID
	s.remaining = count
	s.release = make(chan struct{})
}

func (s *coordinatedSessionUpdateStore) Update(next session.State) error {
	var waitCh chan struct{}
	s.mu.Lock()
	if s.targetID != "" && next.SessionID == s.targetID && s.remaining > 0 {
		s.remaining--
		waitCh = s.release
		if s.remaining == 0 {
			close(s.release)
			s.targetID = ""
		}
	}
	s.mu.Unlock()

	if waitCh != nil {
		<-waitCh
	}
	return s.Store.Update(next)
}

type coordinatedApprovalUpdateStore struct {
	approval.Store
	mu        sync.Mutex
	targetID  string
	remaining int
	release   chan struct{}
}

func (s *coordinatedApprovalUpdateStore) arm(approvalID string, count int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.targetID = approvalID
	s.remaining = count
	s.release = make(chan struct{})
}

func (s *coordinatedApprovalUpdateStore) Update(next approval.Record) error {
	var waitCh chan struct{}
	s.mu.Lock()
	if s.targetID != "" && next.ApprovalID == s.targetID && s.remaining > 0 {
		s.remaining--
		waitCh = s.release
		if s.remaining == 0 {
			close(s.release)
			s.targetID = ""
		}
	}
	s.mu.Unlock()

	if waitCh != nil {
		<-waitCh
	}
	return s.Store.Update(next)
}

func TestRespondApprovalSurfacesApprovalVersionConflict(t *testing.T) {
	sessions := session.NewMemoryStore()
	approvals := &staleApprovalUpdateStore{Store: approval.NewMemoryStore()}
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	audits := audit.NewMemoryStore()
	handler := &countingHandler{}

	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		handler,
	)

	rt := hruntime.New(hruntime.Options{
		Sessions:  sessions,
		Approvals: approvals,
		Tasks:     tasks,
		Plans:     plans,
		Tools:     tools,
		Audit:     audits,
	}).WithPolicyEvaluator(askPolicy{})

	sess := mustCreateSession(t, rt, "approval conflict", "surface stale approval writes")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "approval conflict"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(sess.SessionID, "approval conflict", []plan.StepSpec{{
		StepID: "step_conflict",
		Title:  "approval conflict",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo conflict", "timeout_ms": 5000}},
		Verify: verify.Spec{},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	initial, err := rt.RunStep(context.Background(), sess.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	approvalID := initial.Execution.PendingApproval.ApprovalID
	approvals.arm(approvalID)

	if _, _, err := rt.RespondApproval(approvalID, approval.Response{Reply: approval.ReplyOnce}); !errors.Is(err, approval.ErrApprovalVersionConflict) {
		t.Fatalf("expected approval version conflict, got %v", err)
	}

	storedSession, err := rt.GetSession(sess.SessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if storedSession.PendingApprovalID != approvalID {
		t.Fatalf("expected session to remain pending after approval conflict, got %#v", storedSession)
	}
	if handler.calls != 0 {
		t.Fatalf("expected no tool invocation, got %d", handler.calls)
	}
}

func TestRespondApprovalSurfacesSessionVersionConflict(t *testing.T) {
	sessions := &staleSessionUpdateStore{Store: session.NewMemoryStore()}
	approvals := approval.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	audits := audit.NewMemoryStore()
	handler := &countingHandler{}

	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		handler,
	)

	rt := hruntime.New(hruntime.Options{
		Sessions:  sessions,
		Approvals: approvals,
		Tasks:     tasks,
		Plans:     plans,
		Tools:     tools,
		Audit:     audits,
	}).WithPolicyEvaluator(askPolicy{})

	sess := mustCreateSession(t, rt, "session conflict", "surface stale session writes")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "session conflict"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(sess.SessionID, "session conflict", []plan.StepSpec{{
		StepID: "step_conflict",
		Title:  "session conflict",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo conflict", "timeout_ms": 5000}},
		Verify: verify.Spec{},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	initial, err := rt.RunStep(context.Background(), sess.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	sessions.arm(sess.SessionID)

	if _, _, err := rt.RespondApproval(initial.Execution.PendingApproval.ApprovalID, approval.Response{Reply: approval.ReplyOnce}); !errors.Is(err, session.ErrSessionVersionConflict) {
		t.Fatalf("expected session version conflict, got %v", err)
	}
	if handler.calls != 0 {
		t.Fatalf("expected no tool invocation, got %d", handler.calls)
	}
}

func TestResumePendingApprovalHasSingleWinnerUnderSessionCAS(t *testing.T) {
	sessions := &coordinatedSessionUpdateStore{Store: session.NewMemoryStore()}
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	approvals := approval.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	audits := audit.NewMemoryStore()
	handler := &countingHandler{}

	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		handler,
	)
	verifiers.Register(
		verify.Definition{Kind: "exit_code", Description: "Verify that an execution result exit code is in the allowed set."},
		verify.ExitCodeChecker{},
	)

	rt := hruntime.New(hruntime.Options{
		Sessions:  sessions,
		Approvals: approvals,
		Tasks:     tasks,
		Plans:     plans,
		Tools:     tools,
		Verifiers: verifiers,
		Audit:     audits,
	}).WithPolicyEvaluator(askPolicy{})

	sess := mustCreateSession(t, rt, "resume race", "only one resume should win")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "resume race"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(sess.SessionID, "resume race", []plan.StepSpec{{
		StepID: "step_resume_race",
		Title:  "resume race",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo resumed", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
		}},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	initial, err := rt.RunStep(context.Background(), sess.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	if _, _, err := rt.RespondApproval(initial.Execution.PendingApproval.ApprovalID, approval.Response{Reply: approval.ReplyOnce}); err != nil {
		t.Fatalf("respond approval: %v", err)
	}

	sessions.arm(sess.SessionID, 2)

	errs := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := rt.ResumePendingApproval(context.Background(), sess.SessionID)
			errs <- err
		}()
	}

	successes := 0
	conflicts := 0
	for range 2 {
		err := <-errs
		switch {
		case err == nil:
			successes++
		case errors.Is(err, session.ErrSessionVersionConflict):
			conflicts++
		default:
			t.Fatalf("unexpected resume result: %v", err)
		}
	}

	if successes != 1 || conflicts != 1 {
		t.Fatalf("expected one success and one session conflict, got %d successes and %d conflicts", successes, conflicts)
	}
	if handler.calls != 1 {
		t.Fatalf("expected exactly one tool execution, got %d", handler.calls)
	}

	attempts := mustListAttempts(t, rt, sess.SessionID)
	if len(attempts) != 1 || attempts[0].Status != "completed" {
		t.Fatalf("expected one completed winner attempt, got %#v", attempts)
	}
	storedApproval, err := rt.GetApproval(initial.Execution.PendingApproval.ApprovalID)
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	if storedApproval.Status != approval.StatusConsumed {
		t.Fatalf("expected winning resume to consume approval, got %#v", storedApproval)
	}
}

func TestRecoverSessionHasSingleWinnerUnderSessionCAS(t *testing.T) {
	sessions := &coordinatedSessionUpdateStore{Store: session.NewMemoryStore()}
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	audits := audit.NewMemoryStore()
	handler := &countingHandler{}

	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		handler,
	)
	verifiers.Register(
		verify.Definition{Kind: "exit_code", Description: "Verify that an execution result exit code is in the allowed set."},
		verify.ExitCodeChecker{},
	)

	rt := hruntime.New(hruntime.Options{
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Tools:     tools,
		Verifiers: verifiers,
		Audit:     audits,
	})

	sess := mustCreateSession(t, rt, "recover race", "only one recovery should win")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "recover race"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	if _, err := rt.CreatePlan(sess.SessionID, "recover race", []plan.StepSpec{{
		StepID: "step_recover_race",
		Title:  "recover race",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo recovered", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
		}},
	}}); err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := rt.MarkSessionInterrupted(context.Background(), sess.SessionID); err != nil {
		t.Fatalf("mark interrupted: %v", err)
	}

	sessions.arm(sess.SessionID, 2)

	errs := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := rt.RecoverSession(context.Background(), sess.SessionID)
			errs <- err
		}()
	}

	successes := 0
	conflicts := 0
	for range 2 {
		err := <-errs
		switch {
		case err == nil:
			successes++
		case errors.Is(err, session.ErrSessionVersionConflict):
			conflicts++
		default:
			t.Fatalf("unexpected recover result: %v", err)
		}
	}

	if successes != 1 || conflicts != 1 {
		t.Fatalf("expected one success and one session conflict, got %d successes and %d conflicts", successes, conflicts)
	}
	if handler.calls != 1 {
		t.Fatalf("expected exactly one recovery tool execution, got %d", handler.calls)
	}
}
