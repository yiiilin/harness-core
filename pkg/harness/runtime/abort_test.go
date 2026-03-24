package runtime_test

import (
	"context"
	"errors"
	"testing"
	"time"

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

func TestAbortSessionPendingApprovalTransitionsToAborted(t *testing.T) {
	sessions := session.NewMemoryStore()
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
		Sessions: sessions,
		Tasks:    tasks,
		Plans:    plans,
		Tools:    tools,
		Audit:    audits,
	}).WithPolicyEvaluator(askPolicy{})

	sess := mustCreateSession(t, rt, "abort pending", "abort pending approval")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "abort pending approval"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(sess.SessionID, "abort pending", []plan.StepSpec{{
		StepID: "step_abort_pending",
		Title:  "abort pending approval",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo gated", "timeout_ms": 5000}},
		Verify: verify.Spec{},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	initial, err := rt.RunStep(context.Background(), sess.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	if initial.Execution.PendingApproval == nil {
		t.Fatalf("expected pending approval")
	}

	aborted, err := rt.AbortSession(context.Background(), sess.SessionID, hruntime.AbortRequest{
		Code:   "operator.abort",
		Reason: "manual stop",
	})
	if err != nil {
		t.Fatalf("abort session: %v", err)
	}
	if aborted.Session.Phase != session.PhaseAborted {
		t.Fatalf("expected aborted phase, got %#v", aborted.Session)
	}
	if aborted.Session.PendingApprovalID != "" || aborted.Session.ExecutionState != session.ExecutionIdle {
		t.Fatalf("expected abort to clear pending/in-flight markers, got %#v", aborted.Session)
	}
	if aborted.UpdatedTask == nil || aborted.UpdatedTask.Status != task.StatusAborted {
		t.Fatalf("expected task to be marked aborted, got %#v", aborted.UpdatedTask)
	}
	if handler.calls != 0 {
		t.Fatalf("expected no tool execution, got %d", handler.calls)
	}

	events := mustListAuditEvents(t, rt, sess.SessionID)
	foundAbortedEvent := false
	for _, event := range events {
		if event.Type == audit.EventSessionAborted {
			foundAbortedEvent = true
			break
		}
	}
	if !foundAbortedEvent {
		t.Fatalf("expected session.aborted event, got %#v", events)
	}

	if _, err := rt.ResumePendingApproval(context.Background(), sess.SessionID); !errors.Is(err, hruntime.ErrNoPendingApproval) {
		t.Fatalf("expected abort to prevent approval resume, got %v", err)
	}
}

func TestAbortSessionFinalizesPendingApprovalAndBlockedAttempt(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	audits := audit.NewMemoryStore()

	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		&countingHandler{},
	)

	rt := hruntime.New(hruntime.Options{
		Sessions: sessions,
		Tasks:    tasks,
		Plans:    plans,
		Tools:    tools,
		Audit:    audits,
	}).WithPolicyEvaluator(askPolicy{})

	sess := mustCreateSession(t, rt, "abort approval facts", "abort should finalize pending approval facts")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "abort pending approval facts"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(attached.SessionID, "abort approval facts", []plan.StepSpec{{
		StepID: "step_abort_pending_facts",
		Title:  "abort pending approval facts",
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
	attemptsBefore := mustListAttempts(t, rt, attached.SessionID)
	if len(attemptsBefore) != 1 || attemptsBefore[0].Status != execution.AttemptBlocked {
		t.Fatalf("expected one blocked attempt before abort, got %#v", attemptsBefore)
	}

	if _, err := rt.AbortSession(context.Background(), attached.SessionID, hruntime.AbortRequest{
		Code:   "operator.abort",
		Reason: "abort pending approval facts",
	}); err != nil {
		t.Fatalf("abort session: %v", err)
	}

	storedApproval, err := rt.GetApproval(initial.Execution.PendingApproval.ApprovalID)
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	if storedApproval.Status != approval.StatusRejected {
		t.Fatalf("expected abort to terminalize pending approval, got %#v", storedApproval)
	}

	attemptsAfter := mustListAttempts(t, rt, attached.SessionID)
	if len(attemptsAfter) != 1 {
		t.Fatalf("expected one attempt after abort, got %#v", attemptsAfter)
	}
	if attemptsAfter[0].Status != execution.AttemptFailed || attemptsAfter[0].FinishedAt == 0 {
		t.Fatalf("expected abort to finalize blocked attempt, got %#v", attemptsAfter[0])
	}
}

func TestAbortSessionConsumesApprovedPendingApprovalBeforeResume(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	audits := audit.NewMemoryStore()

	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		&countingHandler{},
	)

	rt := hruntime.New(hruntime.Options{
		Sessions: sessions,
		Tasks:    tasks,
		Plans:    plans,
		Tools:    tools,
		Audit:    audits,
	}).WithPolicyEvaluator(askPolicy{})

	sess := mustCreateSession(t, rt, "abort approved approval", "abort should terminalize approved pending approvals")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "abort approved approval"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(attached.SessionID, "abort approved approval", []plan.StepSpec{{
		StepID: "step_abort_approved_pending",
		Title:  "abort approved pending approval",
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
	if _, _, err := rt.RespondApproval(approvalID, approval.Response{Reply: approval.ReplyAlways}); err != nil {
		t.Fatalf("respond approval: %v", err)
	}

	if _, err := rt.AbortSession(context.Background(), attached.SessionID, hruntime.AbortRequest{
		Code:   "operator.abort",
		Reason: "abort approved pending approval",
	}); err != nil {
		t.Fatalf("abort session: %v", err)
	}

	storedApproval, err := rt.GetApproval(approvalID)
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	if storedApproval.Status != approval.StatusConsumed || storedApproval.ConsumedAt == 0 {
		t.Fatalf("expected abort to consume previously-approved pending approval, got %#v", storedApproval)
	}

	attempts := mustListAttempts(t, rt, attached.SessionID)
	if len(attempts) != 1 || attempts[0].Status != execution.AttemptFailed || attempts[0].FinishedAt == 0 {
		t.Fatalf("expected abort to finalize blocked attempt after approved reply, got %#v", attempts)
	}
}

func TestAbortSessionNoRunnerRollsBackApprovalFactsWhenSessionWriteFails(t *testing.T) {
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

	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		&countingHandler{},
	)

	rt := hruntime.New(hruntime.Options{
		Sessions: sessions,
		Tasks:    tasks,
		Plans:    plans,
		Tools:    tools,
		Audit:    audits,
	}).WithPolicyEvaluator(askPolicy{})
	rt.Runner = nil

	sess := mustCreateSession(t, rt, "abort rollback", "abort should roll back no-runner partial writes")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "abort rollback"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(attached.SessionID, "abort rollback", []plan.StepSpec{{
		StepID: "step_abort_rollback",
		Title:  "abort rollback",
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

	if _, err := rt.AbortSession(context.Background(), attached.SessionID, hruntime.AbortRequest{
		Code:   "operator.abort",
		Reason: "abort rollback",
	}); !errors.Is(err, boom) {
		t.Fatalf("expected abort to surface session update error, got %v", err)
	}

	storedSession, err := rt.GetSession(attached.SessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if storedSession.Phase == session.PhaseAborted || storedSession.PendingApprovalID != approvalID {
		t.Fatalf("expected failed abort not to persist aborted session state, got %#v", storedSession)
	}

	storedApproval, err := rt.GetApproval(approvalID)
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	if storedApproval.Status != approval.StatusPending {
		t.Fatalf("expected failed abort to restore approval to pending, got %#v", storedApproval)
	}

	attempts := mustListAttempts(t, rt, attached.SessionID)
	if len(attempts) != 1 || attempts[0].Status != execution.AttemptBlocked {
		t.Fatalf("expected failed abort to restore blocked attempt, got %#v", attempts)
	}

	plansAfter := mustListPlans(t, rt, attached.SessionID)
	if len(plansAfter) != 1 || plansAfter[0].Steps[0].Status != plan.StepBlocked {
		t.Fatalf("expected failed abort to restore blocked plan step, got %#v", plansAfter)
	}

	storedTask, err := rt.GetTask(tsk.TaskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if storedTask.Status == task.StatusAborted {
		t.Fatalf("expected failed abort not to persist aborted task state, got %#v", storedTask)
	}
}

func TestAbortSessionMarksRecoverableSessionTerminal(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	rt := hruntime.New(hruntime.Options{
		Sessions: sessions,
		Tasks:    tasks,
		Plans:    plans,
	})

	sess := mustCreateSession(t, rt, "abort recoverable", "abort recoverable session")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "abort recoverable session"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	if _, err := rt.MarkSessionInterrupted(context.Background(), sess.SessionID); err != nil {
		t.Fatalf("mark interrupted: %v", err)
	}

	aborted, err := rt.AbortSession(context.Background(), sess.SessionID, hruntime.AbortRequest{
		Code:   "operator.abort",
		Reason: "stop recovery",
	})
	if err != nil {
		t.Fatalf("abort session: %v", err)
	}
	if aborted.Session.Phase != session.PhaseAborted {
		t.Fatalf("expected aborted phase, got %#v", aborted.Session)
	}
	if aborted.Session.ExecutionState != session.ExecutionIdle || aborted.Session.InFlightStepID != "" {
		t.Fatalf("expected abort to clear recovery execution markers, got %#v", aborted.Session)
	}

	recovered, err := rt.RecoverSession(context.Background(), sess.SessionID)
	if err != nil {
		t.Fatalf("recover aborted session: %v", err)
	}
	if recovered.Session.Phase != session.PhaseAborted {
		t.Fatalf("expected aborted session to stay terminal under recovery, got %#v", recovered.Session)
	}
}

func TestAbortSessionMarksInFlightSessionTerminal(t *testing.T) {
	rt := hruntime.New(hruntime.Options{})

	sess := mustCreateSession(t, rt, "abort in-flight", "abort in-flight session")
	if _, err := rt.MarkSessionInFlight(context.Background(), sess.SessionID, "step_abort_inflight"); err != nil {
		t.Fatalf("mark in-flight: %v", err)
	}

	aborted, err := rt.AbortSession(context.Background(), sess.SessionID, hruntime.AbortRequest{
		Code:   "operator.abort",
		Reason: "stop in-flight execution",
	})
	if err != nil {
		t.Fatalf("abort session: %v", err)
	}
	if aborted.Session.Phase != session.PhaseAborted {
		t.Fatalf("expected aborted phase, got %#v", aborted.Session)
	}
	if aborted.Session.ExecutionState != session.ExecutionIdle || aborted.Session.InFlightStepID != "" {
		t.Fatalf("expected abort to clear in-flight markers, got %#v", aborted.Session)
	}
}

func TestAbortSessionMarksInFlightPlanStepFailed(t *testing.T) {
	rt := hruntime.New(hruntime.Options{})

	sess := mustCreateSession(t, rt, "abort plan step", "abort should reconcile in-flight plan step")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "abort should reconcile in-flight plan step"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(attached.SessionID, "abort plan step", []plan.StepSpec{{
		StepID: "step_abort_plan_reconcile",
		Title:  "abort plan step reconcile",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo step", "timeout_ms": 5000}},
		Verify: verify.Spec{},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := rt.MarkSessionInFlight(context.Background(), attached.SessionID, pl.Steps[0].StepID); err != nil {
		t.Fatalf("mark in-flight: %v", err)
	}

	if _, err := rt.AbortSession(context.Background(), attached.SessionID, hruntime.AbortRequest{
		Code:   "operator.abort",
		Reason: "abort in-flight plan step",
	}); err != nil {
		t.Fatalf("abort session: %v", err)
	}

	plansAfter := mustListPlans(t, rt, attached.SessionID)
	if len(plansAfter) != 1 {
		t.Fatalf("expected one plan, got %#v", plansAfter)
	}
	if plansAfter[0].Steps[0].Status != plan.StepFailed || plansAfter[0].Steps[0].FinishedAt == 0 {
		t.Fatalf("expected abort to reconcile in-flight step as failed, got %#v", plansAfter[0].Steps[0])
	}
}

func TestAbortSessionRevokesClaimedLeaseAndPreservesAbortedState(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	audits := audit.NewMemoryStore()
	handler := &blockingHandler{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}

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

	sess := mustCreateSession(t, rt, "abort claimed run", "abort should preempt claimed execution")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "abort should preempt claimed execution"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	if _, err := rt.CreatePlan(attached.SessionID, "abort claimed run", []plan.StepSpec{{
		StepID: "step_abort_claimed_preempt",
		Title:  "abort claimed run",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo claimed", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
		}},
	}}); err != nil {
		t.Fatalf("create plan: %v", err)
	}

	claimed, ok, err := rt.ClaimRunnableSession(context.Background(), time.Minute)
	if err != nil {
		t.Fatalf("claim runnable session: %v", err)
	}
	if !ok || claimed.SessionID != attached.SessionID {
		t.Fatalf("expected claimed session %s, got %#v ok=%v", attached.SessionID, claimed, ok)
	}

	runErrCh := make(chan error, 1)
	go func() {
		_, err := rt.RunClaimedSession(context.Background(), attached.SessionID, claimed.LeaseID)
		runErrCh <- err
	}()

	select {
	case <-handler.started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for claimed execution to start")
	}

	aborted, err := rt.AbortSession(context.Background(), attached.SessionID, hruntime.AbortRequest{
		Code:   "operator.abort",
		Reason: "abort claimed execution",
	})
	if err != nil {
		t.Fatalf("abort session: %v", err)
	}
	if aborted.Session.LeaseID != "" || aborted.Session.LeaseExpiresAt != 0 {
		t.Fatalf("expected abort to revoke active lease, got %#v", aborted.Session)
	}

	close(handler.release)

	select {
	case err := <-runErrCh:
		if !errors.Is(err, session.ErrSessionLeaseNotHeld) {
			t.Fatalf("expected claimed run to fail after abort lease revocation, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for claimed run to observe abort revocation")
	}

	persisted, err := rt.GetSession(attached.SessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if persisted.Phase != session.PhaseAborted || persisted.LeaseID != "" {
		t.Fatalf("expected session to remain aborted after claimed run exits, got %#v", persisted)
	}
}

func TestAbortSessionRejectsCompletedSession(t *testing.T) {
	rt := hruntime.New(hruntime.Options{})

	sess := mustCreateSession(t, rt, "abort complete", "cannot abort complete")
	sess.Phase = session.PhaseComplete
	sess.Version++
	if err := rt.Sessions.Update(sess); err != nil {
		t.Fatalf("update complete session: %v", err)
	}

	if _, err := rt.AbortSession(context.Background(), sess.SessionID, hruntime.AbortRequest{
		Code:   "operator.abort",
		Reason: "too late",
	}); !errors.Is(err, hruntime.ErrSessionTerminal) {
		t.Fatalf("expected terminal session error, got %v", err)
	}
}

func TestAbortedSessionCannotRunStep(t *testing.T) {
	rt := hruntime.New(hruntime.Options{})

	sess := mustCreateSession(t, rt, "abort rerun", "aborted sessions stay terminal")
	aborted, err := rt.AbortSession(context.Background(), sess.SessionID, hruntime.AbortRequest{
		Code:   "operator.abort",
		Reason: "stop rerun",
	})
	if err != nil {
		t.Fatalf("abort session: %v", err)
	}

	_, err = rt.RunStep(context.Background(), aborted.Session.SessionID, plan.StepSpec{
		StepID: "step_abort_rerun",
		Title:  "should not run",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo nope", "timeout_ms": 5000}},
		Verify: verify.Spec{},
	})
	if !errors.Is(err, hruntime.ErrSessionTerminal) {
		t.Fatalf("expected terminal session error, got %v", err)
	}
}
