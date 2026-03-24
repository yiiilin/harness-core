package runtime_test

import (
	"context"
	"errors"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type failingPlanUpdateStore struct {
	plan.Store
	updateErr error
}

func (s failingPlanUpdateStore) Update(plan.Spec) error {
	return s.updateErr
}

type nthFailingTaskUpdateStore struct {
	task.Store
	updateErr        error
	failOnUpdateCall int
	updateCalls      int
}

func (s *nthFailingTaskUpdateStore) Update(next task.Record) error {
	s.updateCalls++
	if s.failOnUpdateCall > 0 && s.updateCalls == s.failOnUpdateCall {
		return s.updateErr
	}
	return s.Store.Update(next)
}

type nthFailingAttemptCreateStore struct {
	execution.AttemptStore
	createErr        error
	failOnCreateCall int
	createCalls      int
}

func (s *nthFailingAttemptCreateStore) Create(next execution.Attempt) (execution.Attempt, error) {
	s.createCalls++
	if s.failOnCreateCall > 0 && s.createCalls == s.failOnCreateCall {
		return execution.Attempt{}, s.createErr
	}
	return s.AttemptStore.Create(next)
}

type nthFailingApprovalUpdateStore struct {
	approval.Store
	updateErr        error
	failOnUpdateCall int
	updateCalls      int
}

func (s *nthFailingApprovalUpdateStore) Update(next approval.Record) error {
	s.updateCalls++
	if s.failOnUpdateCall > 0 && s.updateCalls == s.failOnUpdateCall {
		return s.updateErr
	}
	return s.Store.Update(next)
}

func TestNoRunnerRunStepAskSurfacesPlanUpdateErrors(t *testing.T) {
	boom := errors.New("boom:plan.update")
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := failingPlanUpdateStore{Store: plan.NewMemoryStore(), updateErr: boom}
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	audits := audit.NewMemoryStore()

	rt := hruntime.New(hruntime.Options{
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Tools:     tools,
		Verifiers: verifiers,
		Audit:     audits,
	}).WithPolicyEvaluator(askPolicy{})
	rt.Runner = nil

	sess := mustCreateSession(t, rt, "ask plan update failure", "surface blocked plan update failures")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "ask path should surface plan update failures"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(attached.SessionID, "ask", []plan.StepSpec{{
		StepID: "step_ask_plan_update_fail",
		Title:  "ask plan update fail",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo ask", "timeout_ms": 5000}},
		Verify: verify.Spec{},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	if _, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0]); !errors.Is(err, boom) {
		t.Fatalf("expected ask path to surface plan update error, got %v", err)
	}
}

func TestNoRunnerRunStepAskDoesNotLeavePendingApprovalOrBlockedPlanWhenSessionWriteFails(t *testing.T) {
	boom := errors.New("boom:session.update")
	sessions := &nthFailingSessionUpdateStore{
		Store:            session.NewMemoryStore(),
		updateErr:        boom,
		failOnUpdateCall: 2,
	}
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	audits := audit.NewMemoryStore()

	rt := hruntime.New(hruntime.Options{
		Sessions: sessions,
		Tasks:    tasks,
		Plans:    plans,
		Audit:    audits,
	}).WithPolicyEvaluator(askPolicy{})
	rt.Runner = nil

	sess := mustCreateSession(t, rt, "ask session failure", "ask path must compensate no-runner session write failures")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "avoid stranded pending approvals"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(attached.SessionID, "ask", []plan.StepSpec{{
		StepID: "step_ask_session_failure",
		Title:  "ask session failure",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo ask", "timeout_ms": 5000}},
		Verify: verify.Spec{},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	if _, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0]); !errors.Is(err, boom) {
		t.Fatalf("expected ask path to surface session update error, got %v", err)
	}

	storedSession, err := rt.GetSession(attached.SessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if storedSession.PendingApprovalID != "" || storedSession.ExecutionState != session.ExecutionIdle {
		t.Fatalf("expected failed ask path not to leave session waiting approval, got %#v", storedSession)
	}

	approvals, err := rt.ListApprovals(attached.SessionID)
	if err != nil {
		t.Fatalf("list approvals: %v", err)
	}
	for _, rec := range approvals {
		if rec.Status == approval.StatusPending || rec.Status == approval.StatusApproved {
			t.Fatalf("expected failed ask path not to leave resumable approvals behind, got %#v", approvals)
		}
	}

	plansAfter := mustListPlans(t, rt, attached.SessionID)
	if len(plansAfter) != 1 {
		t.Fatalf("expected one persisted plan revision, got %#v", plansAfter)
	}
	if plansAfter[0].Steps[0].Status == plan.StepBlocked {
		t.Fatalf("expected failed ask path to compensate blocked plan state, got %#v", plansAfter[0])
	}
}

func TestNoRunnerRunStepDenySurfacesTaskUpdateErrors(t *testing.T) {
	boom := errors.New("boom:task.update")
	sessions := session.NewMemoryStore()
	taskStore := &nthFailingTaskUpdateStore{Store: task.NewMemoryStore(), updateErr: boom, failOnUpdateCall: 2}
	plans := plan.NewMemoryStore()
	audits := audit.NewMemoryStore()

	rt := hruntime.New(hruntime.Options{
		Sessions: sessions,
		Tasks:    taskStore,
		Plans:    plans,
		Audit:    audits,
	}).WithPolicyEvaluator(denyAllPolicy{})
	rt.Runner = nil

	sess := mustCreateSession(t, rt, "deny task update failure", "surface deny-path task update failures")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "deny path should surface task update failures"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(attached.SessionID, "deny", []plan.StepSpec{{
		StepID: "step_deny_task_update_fail",
		Title:  "deny task update fail",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo deny", "timeout_ms": 5000}},
		Verify: verify.Spec{},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	if _, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0]); !errors.Is(err, boom) {
		t.Fatalf("expected deny path to surface task update error, got %v", err)
	}
}

func TestNoRunnerRunStepCompleteSurfacesTaskUpdateErrors(t *testing.T) {
	boom := errors.New("boom:task.update")
	sessions := session.NewMemoryStore()
	taskStore := &nthFailingTaskUpdateStore{Store: task.NewMemoryStore(), updateErr: boom, failOnUpdateCall: 2}
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
		Tasks:     taskStore,
		Plans:     plans,
		Tools:     tools,
		Verifiers: verifiers,
		Audit:     audits,
	})
	rt.Runner = nil

	sess := mustCreateSession(t, rt, "complete task update failure", "surface complete-path task update failures")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "complete path should surface task update failures"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(attached.SessionID, "complete", []plan.StepSpec{{
		StepID: "step_complete_task_update_fail",
		Title:  "complete task update fail",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo complete", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
		}},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	if _, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0]); !errors.Is(err, boom) {
		t.Fatalf("expected complete path to surface task update error, got %v", err)
	}
	if handler.calls != 1 {
		t.Fatalf("expected action to execute before task update failure, got %d calls", handler.calls)
	}
}

func TestNoRunnerRunStepDenyStaysSuccessfulWhenAttemptPersistenceFailsAfterStateCommit(t *testing.T) {
	boom := errors.New("boom:attempt.create")
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	audits := audit.NewMemoryStore()
	attempts := &nthFailingAttemptCreateStore{
		AttemptStore:     execution.NewMemoryAttemptStore(),
		createErr:        boom,
		failOnCreateCall: 1,
	}

	rt := hruntime.New(hruntime.Options{
		Sessions: sessions,
		Tasks:    tasks,
		Plans:    plans,
		Attempts: attempts,
		Audit:    audits,
	}).WithPolicyEvaluator(denyAllPolicy{})
	rt.Runner = nil

	sess := mustCreateSession(t, rt, "deny attempt failure", "deny path should not split after state commit")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "deny path should stay successful"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(attached.SessionID, "deny", []plan.StepSpec{{
		StepID: "step_deny_attempt_failure",
		Title:  "deny attempt failure",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo deny", "timeout_ms": 5000}},
		Verify: verify.Spec{},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	out, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("expected deny path to stay successful after post-commit attempt failure, got %v", err)
	}
	if out.Session.Phase != session.PhaseFailed {
		t.Fatalf("expected deny path to commit failed session state, got %#v", out.Session)
	}
	storedSession, err := rt.GetSession(attached.SessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if storedSession.Phase != session.PhaseFailed {
		t.Fatalf("expected stored session to stay failed, got %#v", storedSession)
	}
}

func TestNoRunnerRunStepCompleteStaysSuccessfulWhenAttemptPersistenceFailsAfterStateCommit(t *testing.T) {
	boom := errors.New("boom:attempt.create")
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	audits := audit.NewMemoryStore()
	attempts := &nthFailingAttemptCreateStore{
		AttemptStore:     execution.NewMemoryAttemptStore(),
		createErr:        boom,
		failOnCreateCall: 1,
	}
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
		Attempts:  attempts,
		Tools:     tools,
		Verifiers: verifiers,
		Audit:     audits,
	})
	rt.Runner = nil

	sess := mustCreateSession(t, rt, "complete attempt failure", "post-commit attempt persistence must be best effort")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "complete path should stay successful"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(attached.SessionID, "complete", []plan.StepSpec{{
		StepID: "step_complete_attempt_failure",
		Title:  "complete attempt failure",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo complete", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
		}},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	out, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("expected complete path to stay successful after post-commit attempt failure, got %v", err)
	}
	if out.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected completed session despite attempt persistence failure, got %#v", out.Session)
	}
	if handler.calls != 1 {
		t.Fatalf("expected action to execute exactly once, got %d calls", handler.calls)
	}
	storedSession, err := rt.GetSession(attached.SessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if storedSession.Phase != session.PhaseComplete {
		t.Fatalf("expected stored session to stay complete, got %#v", storedSession)
	}
}

func TestNoRunnerRespondApprovalRejectSurfacesPlanUpdateErrors(t *testing.T) {
	boom := errors.New("boom:task.update")
	sessions := session.NewMemoryStore()
	taskStore := &nthFailingTaskUpdateStore{Store: task.NewMemoryStore(), updateErr: boom, failOnUpdateCall: 2}
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	audits := audit.NewMemoryStore()

	rt := hruntime.New(hruntime.Options{
		Sessions:  sessions,
		Tasks:     taskStore,
		Plans:     plans,
		Tools:     tools,
		Verifiers: verifiers,
		Audit:     audits,
	}).WithPolicyEvaluator(askPolicy{})
	rt.Runner = nil

	sess := mustCreateSession(t, rt, "reject plan update failure", "surface approval reject plan update failures")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "reject path should surface plan update failures"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(attached.SessionID, "reject", []plan.StepSpec{{
		StepID: "step_reject_plan_update_fail",
		Title:  "reject plan update fail",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo reject", "timeout_ms": 5000}},
		Verify: verify.Spec{},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	initial, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	if initial.Execution.PendingApproval == nil {
		t.Fatalf("expected pending approval before reject path")
	}

	if _, _, err := rt.RespondApproval(initial.Execution.PendingApproval.ApprovalID, approval.Response{Reply: approval.ReplyReject}); !errors.Is(err, boom) {
		t.Fatalf("expected reject path to surface task update error, got %v", err)
	}
}

func TestNoRunnerRespondApprovalApproveKeepsApprovalRetryableWhenSessionWriteFails(t *testing.T) {
	boom := errors.New("boom:session.update")
	sessions := &nthFailingSessionUpdateStore{
		Store:            session.NewMemoryStore(),
		updateErr:        boom,
		failOnUpdateCall: 3,
	}
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	audits := audit.NewMemoryStore()

	rt := hruntime.New(hruntime.Options{
		Sessions: sessions,
		Tasks:    tasks,
		Plans:    plans,
		Tools:    tools,
		Audit:    audits,
	}).WithPolicyEvaluator(askPolicy{})
	rt.Runner = nil

	sess := mustCreateSession(t, rt, "approve rollback", "approved responses must remain retryable when no-runner session writes fail")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "approval response should roll back"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(attached.SessionID, "approval", []plan.StepSpec{{
		StepID: "step_approve_session_failure",
		Title:  "approve session failure",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo gated", "timeout_ms": 5000}},
		Verify: verify.Spec{},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	initial, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	if initial.Execution.PendingApproval == nil {
		t.Fatalf("expected pending approval")
	}

	approvalID := initial.Execution.PendingApproval.ApprovalID
	if _, _, err := rt.RespondApproval(approvalID, approval.Response{Reply: approval.ReplyOnce}); !errors.Is(err, boom) {
		t.Fatalf("expected approve path to surface session update error, got %v", err)
	}

	storedApproval, err := rt.GetApproval(approvalID)
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	if storedApproval.Status != approval.StatusPending {
		t.Fatalf("expected failed approve path to leave approval retryable, got %#v", storedApproval)
	}

	storedSession, err := rt.GetSession(attached.SessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if storedSession.PendingApprovalID != approvalID || storedSession.ExecutionState != session.ExecutionAwaitingApproval {
		t.Fatalf("expected failed approve path to preserve session pending state, got %#v", storedSession)
	}
}

func TestNoRunnerRespondApprovalRejectRollsBackApprovalFactsWhenSessionWriteFails(t *testing.T) {
	boom := errors.New("boom:session.update")
	sessions := &nthFailingSessionUpdateStore{
		Store:            session.NewMemoryStore(),
		updateErr:        boom,
		failOnUpdateCall: 3,
	}
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	audits := audit.NewMemoryStore()

	rt := hruntime.New(hruntime.Options{
		Sessions: sessions,
		Tasks:    tasks,
		Plans:    plans,
		Tools:    tools,
		Audit:    audits,
	}).WithPolicyEvaluator(askPolicy{})
	rt.Runner = nil

	sess := mustCreateSession(t, rt, "reject rollback", "reject responses must roll back when no-runner session writes fail")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "reject response should roll back"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(attached.SessionID, "approval", []plan.StepSpec{{
		StepID: "step_reject_session_failure",
		Title:  "reject session failure",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo gated", "timeout_ms": 5000}},
		Verify: verify.Spec{},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	initial, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	if initial.Execution.PendingApproval == nil {
		t.Fatalf("expected pending approval")
	}

	approvalID := initial.Execution.PendingApproval.ApprovalID
	if _, _, err := rt.RespondApproval(approvalID, approval.Response{Reply: approval.ReplyReject}); !errors.Is(err, boom) {
		t.Fatalf("expected reject path to surface session update error, got %v", err)
	}

	storedApproval, err := rt.GetApproval(approvalID)
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	if storedApproval.Status != approval.StatusPending {
		t.Fatalf("expected failed reject path to restore approval to pending, got %#v", storedApproval)
	}

	storedSession, err := rt.GetSession(attached.SessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if storedSession.PendingApprovalID != approvalID || storedSession.Phase == session.PhaseFailed {
		t.Fatalf("expected failed reject path not to terminalize session, got %#v", storedSession)
	}

	plansAfter := mustListPlans(t, rt, attached.SessionID)
	if len(plansAfter) != 1 {
		t.Fatalf("expected one plan after rollback, got %#v", plansAfter)
	}
	if plansAfter[0].Steps[0].Status != plan.StepBlocked {
		t.Fatalf("expected failed reject path to restore blocked plan step, got %#v", plansAfter[0])
	}

	attempts := mustListAttempts(t, rt, attached.SessionID)
	if len(attempts) != 1 || attempts[0].Status != execution.AttemptBlocked {
		t.Fatalf("expected failed reject path to restore blocked attempt, got %#v", attempts)
	}
}

func TestNoRunnerResumeApprovalStaysSuccessfulWhenFinalApprovalPersistenceFailsAfterStateCommit(t *testing.T) {
	boom := errors.New("boom:approval.update")
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	audits := audit.NewMemoryStore()
	approvals := &nthFailingApprovalUpdateStore{
		Store:            approval.NewMemoryStore(),
		updateErr:        boom,
		failOnUpdateCall: 2,
	}
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
		Approvals: approvals,
		Tools:     tools,
		Verifiers: verifiers,
		Audit:     audits,
	}).WithPolicyEvaluator(askPolicy{})
	rt.Runner = nil

	sess := mustCreateSession(t, rt, "approval finalize failure", "approved execution should not split after state commit")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "resume path should stay successful"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(attached.SessionID, "approval", []plan.StepSpec{{
		StepID: "step_approval_finalize_failure",
		Title:  "approval finalize failure",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo approved", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
		}},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	initial, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	if initial.Execution.PendingApproval == nil {
		t.Fatalf("expected pending approval")
	}
	approvalID := initial.Execution.PendingApproval.ApprovalID
	if _, _, err := rt.RespondApproval(approvalID, approval.Response{Reply: approval.ReplyOnce}); err != nil {
		t.Fatalf("respond approval: %v", err)
	}

	resumed, err := rt.ResumePendingApproval(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("expected approval resume to stay successful after final approval persistence failure, got %v", err)
	}
	if resumed.Session.Phase != session.PhaseComplete || resumed.Session.PendingApprovalID != "" {
		t.Fatalf("expected resumed session to commit completion, got %#v", resumed.Session)
	}
	if handler.calls != 1 {
		t.Fatalf("expected approved action to execute once, got %d calls", handler.calls)
	}

	storedSession, err := rt.GetSession(attached.SessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if storedSession.Phase != session.PhaseComplete || storedSession.PendingApprovalID != "" {
		t.Fatalf("expected stored session to stay complete with no pending approval, got %#v", storedSession)
	}
	storedApproval, err := rt.GetApproval(approvalID)
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	if storedApproval.Status != approval.StatusApproved {
		t.Fatalf("expected failed best-effort approval finalization to leave approval approved, got %#v", storedApproval)
	}
}
