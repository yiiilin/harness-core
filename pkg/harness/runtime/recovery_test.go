package runtime_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/builtins"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/persistence"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type nthFailingSessionUpdateStore struct {
	session.Store
	updateErr        error
	failOnUpdateCall int
	updateCalls      int
}

func (s *nthFailingSessionUpdateStore) Update(next session.State) error {
	s.updateCalls++
	if s.failOnUpdateCall > 0 && s.updateCalls == s.failOnUpdateCall {
		return s.updateErr
	}
	return s.Store.Update(next)
}

type messageHandler struct{}

func (messageHandler) Invoke(_ context.Context, args map[string]any) (action.Result, error) {
	message, _ := args["message"].(string)
	return action.Result{
		OK: true,
		Data: map[string]any{
			"status":    "completed",
			"exit_code": 0,
			"stdout":    message,
		},
	}, nil
}

func TestRecoveryReadPathAcrossRuntimeReinit(t *testing.T) {
	opts := hruntime.Options{}
	builtins.Register(&opts)
	rt1 := hruntime.New(opts)
	sess := mustCreateSession(t, rt1, "recovery", "mark in-flight and recover later")
	_, err := rt1.MarkSessionInFlight(context.Background(), sess.SessionID, "step_1")
	if err != nil {
		t.Fatalf("mark in-flight: %v", err)
	}

	// Simulate restart by constructing a new runtime with the same backing stores.
	rt2 := hruntime.New(opts)
	items := mustListRecoverableSessions(t, rt2)
	if len(items) != 1 {
		t.Fatalf("expected 1 recoverable session, got %d", len(items))
	}
	if items[0].SessionID != sess.SessionID {
		t.Fatalf("expected session %s, got %s", sess.SessionID, items[0].SessionID)
	}
	if items[0].InFlightStepID != "step_1" {
		t.Fatalf("expected in-flight step step_1, got %s", items[0].InFlightStepID)
	}
}

func TestListRecoverableSessionsIncludesNormalizedRecoveryStateAfterRestart(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()

	rt1 := hruntime.New(hruntime.Options{
		Sessions: sessions,
		Tasks:    tasks,
		Plans:    plans,
	})
	sess := mustCreateSession(t, rt1, "normalized recovery listing", "normalized recovery state must stay discoverable")
	tsk := mustCreateTask(t, rt1, task.Spec{TaskType: "demo", Goal: "normalized recovery state must stay discoverable"})
	attached, err := rt1.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	if _, err := rt1.MarkSessionInFlight(context.Background(), attached.SessionID, "step_normalized_listing"); err != nil {
		t.Fatalf("mark in-flight: %v", err)
	}
	if _, err := rt1.MarkSessionInterrupted(context.Background(), attached.SessionID); err != nil {
		t.Fatalf("mark interrupted: %v", err)
	}

	normalized, err := sessions.Get(attached.SessionID)
	if err != nil {
		t.Fatalf("get interrupted session: %v", err)
	}
	normalized.ExecutionState = session.ExecutionIdle
	normalized.Phase = session.PhaseRecover
	normalized.Version++
	if err := sessions.Update(normalized); err != nil {
		t.Fatalf("persist normalized recovery state: %v", err)
	}

	rt2 := hruntime.New(hruntime.Options{
		Sessions: sessions,
		Tasks:    tasks,
		Plans:    plans,
	})
	items := mustListRecoverableSessions(t, rt2)
	if len(items) != 1 || items[0].SessionID != attached.SessionID {
		t.Fatalf("expected normalized recovery session to stay listed as recoverable, got %#v", items)
	}
}

func TestRecoverSessionUsesOriginPlanRevisionWhenNewerRevisionExists(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	audits := audit.NewMemoryStore()

	tools.Register(
		tool.Definition{ToolName: "demo.echo", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true},
		messageHandler{},
	)
	verifiers.Register(
		verify.Definition{Kind: "exit_code", Description: "Verify that an execution result exit code is in the allowed set."},
		verify.ExitCodeChecker{},
	)
	verifiers.Register(
		verify.Definition{Kind: "output_contains", Description: "Verify output contains substring."},
		verify.OutputContainsChecker{},
	)

	rt := hruntime.New(hruntime.Options{
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Tools:     tools,
		Verifiers: verifiers,
		Audit:     audits,
	})

	sess := mustCreateSession(t, rt, "recovery revision binding", "recovery must resume the originally pinned plan revision")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "resume old plan revision after replan"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	first, err := rt.CreatePlan(attached.SessionID, "revision 1", []plan.StepSpec{{
		StepID: "step_shared",
		Title:  "revision 1 recovery step",
		Action: action.Spec{ToolName: "demo.echo", Args: map[string]any{"message": "old"}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
			{Kind: "output_contains", Args: map[string]any{"text": "old"}},
		}},
	}})
	if err != nil {
		t.Fatalf("create first plan: %v", err)
	}

	if _, err := rt.MarkSessionInFlight(context.Background(), attached.SessionID, first.Steps[0].StepID); err != nil {
		t.Fatalf("mark session in-flight: %v", err)
	}
	if _, err := rt.MarkSessionInterrupted(context.Background(), attached.SessionID); err != nil {
		t.Fatalf("mark session interrupted: %v", err)
	}

	second, err := rt.CreatePlan(attached.SessionID, "revision 2", []plan.StepSpec{{
		StepID: "step_shared",
		Title:  "revision 2 replacement step",
		Action: action.Spec{ToolName: "demo.echo", Args: map[string]any{"message": "new"}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
			{Kind: "output_contains", Args: map[string]any{"text": "new"}},
		}},
	}})
	if err != nil {
		t.Fatalf("create second plan: %v", err)
	}

	out, err := rt.RecoverSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("recover session: %v", err)
	}
	if len(out.Executions) != 1 {
		t.Fatalf("expected one recovered execution, got %#v", out)
	}
	stdout, _ := out.Executions[0].Execution.Action.Data["stdout"].(string)
	if stdout != "old" {
		t.Fatalf("expected recovery to execute the original plan revision, got output %q from %#v", stdout, out.Executions[0].Execution.Action.Data)
	}

	storedFirst := mustPlanByRevision(t, rt, attached.SessionID, first.Revision)
	storedSecond := mustPlanByRevision(t, rt, attached.SessionID, second.Revision)
	if storedFirst.Steps[0].Status != plan.StepCompleted {
		t.Fatalf("expected originating recovery revision to complete, got %#v", storedFirst)
	}
	if storedSecond.Steps[0].Status != plan.StepPending {
		t.Fatalf("expected newer revision to remain pending, got %#v", storedSecond)
	}
}

func TestRecoveryStateTransitionsUseRunnerBoundary(t *testing.T) {
	sessions := session.NewMemoryStore()
	runner := &countingRunner{repos: persistence.RepositorySet{Sessions: sessions}}
	rt := hruntime.New(hruntime.Options{
		Sessions: sessions,
		Runner:   runner,
	})

	sess := mustCreateSession(t, rt, "runner recovery", "recovery updates should use runner")
	baselineCalls := runner.calls

	if _, err := rt.MarkSessionInFlight(context.Background(), sess.SessionID, "step_1"); err != nil {
		t.Fatalf("mark in-flight: %v", err)
	}
	if _, err := rt.MarkSessionInterrupted(context.Background(), sess.SessionID); err != nil {
		t.Fatalf("mark interrupted: %v", err)
	}

	if runner.calls < baselineCalls+2 {
		t.Fatalf("expected recovery writes to use runner, got %d calls from baseline %d", runner.calls, baselineCalls)
	}
}

func TestMarkSessionInFlightDoesNotPersistRuntimeBudgetAnchorWhenMutationFails(t *testing.T) {
	clock := &fakeClock{now: 4242}
	boom := errors.New("boom:session.update")
	sessions := &nthFailingSessionUpdateStore{
		Store:            session.NewMemoryStoreWithClock(clock),
		updateErr:        boom,
		failOnUpdateCall: 1,
	}
	runner := &countingRunner{repos: persistence.RepositorySet{Sessions: sessions}}
	rt := hruntime.New(hruntime.Options{
		Clock:    clock,
		Sessions: sessions,
		Runner:   runner,
	})

	sess := mustCreateSession(t, rt, "runtime anchor rollback", "failed in-flight mutation must not burn runtime budget")

	if _, err := rt.MarkSessionInFlight(context.Background(), sess.SessionID, "step_anchor_fail"); !errors.Is(err, boom) {
		t.Fatalf("expected in-flight mutation failure, got %v", err)
	}

	persisted, err := rt.GetSession(sess.SessionID)
	if err != nil {
		t.Fatalf("get session after failed in-flight mutation: %v", err)
	}
	if persisted.RuntimeStartedAt != 0 {
		t.Fatalf("expected runtime budget anchor to remain unset after failed in-flight mutation, got %#v", persisted)
	}
	if persisted.ExecutionState != session.ExecutionIdle {
		t.Fatalf("expected execution state to remain idle after failed in-flight mutation, got %#v", persisted)
	}
}

func TestRecoveryStateMutationsEmitAuditEventsAndHandleInvalidations(t *testing.T) {
	opts := hruntime.Options{Audit: audit.NewMemoryStore()}
	builtins.Register(&opts)
	rt := hruntime.New(opts)

	sess := mustCreateSession(t, rt, "recovery audit", "recovery state mutations should be audited")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "recover and invalidate stale handles"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	if _, err := rt.CreatePlan(attached.SessionID, "recover audit plan", []plan.StepSpec{{
		StepID: "step_recovery_audit",
		Title:  "recover and audit",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo recovery-audit", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
			{Kind: "output_contains", Args: map[string]any{"text": "recovery-audit"}},
		}},
	}}); err != nil {
		t.Fatalf("create plan: %v", err)
	}

	if _, err := rt.MarkSessionInFlight(context.Background(), attached.SessionID, "step_recovery_audit"); err != nil {
		t.Fatalf("mark in-flight: %v", err)
	}
	if _, err := rt.MarkSessionInterrupted(context.Background(), attached.SessionID); err != nil {
		t.Fatalf("mark interrupted: %v", err)
	}
	if _, err := rt.RuntimeHandles.Create(execution.RuntimeHandle{
		HandleID:  "hdl_recovery_audit",
		SessionID: attached.SessionID,
		TaskID:    attached.TaskID,
		Kind:      "pty",
		Value:     "pty-recovery-audit",
		Status:    execution.RuntimeHandleActive,
	}); err != nil {
		t.Fatalf("seed runtime handle: %v", err)
	}

	out, err := rt.RecoverSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("recover session: %v", err)
	}
	if out.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected recovered session to complete, got %#v", out.Session)
	}

	events := mustListAuditEvents(t, rt, attached.SessionID)
	mutations := map[string]bool{
		"in_flight":   false,
		"interrupted": false,
		"recovered":   false,
	}
	foundHandleInvalidation := false
	for _, event := range events {
		switch event.Type {
		case audit.EventRecoveryStateChanged:
			if mutation, _ := event.Payload["mutation"].(string); mutation != "" {
				if _, ok := mutations[mutation]; ok {
					mutations[mutation] = true
				}
			}
		case audit.EventRuntimeHandleInvalidated:
			if got, _ := event.Payload["handle_id"].(string); got == "hdl_recovery_audit" {
				foundHandleInvalidation = true
			}
		}
	}
	for mutation, found := range mutations {
		if !found {
			t.Fatalf("expected recovery mutation %q in audit trail, got %#v", mutation, events)
		}
	}
	if !foundHandleInvalidation {
		t.Fatalf("expected runtime handle invalidation event for recovery, got %#v", events)
	}
}

func TestRecoveryAuditFailuresAreBestEffortWithoutRunnerAndSurfaceWithRunner(t *testing.T) {
	t.Run("without runner recovery mutation stays successful", func(t *testing.T) {
		rt := hruntime.New(hruntime.Options{
			EventSink: selectiveFailingEventSink{failures: map[string]error{audit.EventRecoveryStateChanged: errors.New("boom:recovery.state_changed")}},
		})
		rt.Runner = nil
		sess := mustCreateSession(t, rt, "recovery best effort", "recovery mutation should stay successful without runner")

		updated, err := rt.MarkSessionInFlight(context.Background(), sess.SessionID, "step_best_effort")
		if err != nil {
			t.Fatalf("expected recovery mutation to stay successful without runner, got %v", err)
		}
		if updated.ExecutionState != session.ExecutionInFlight {
			t.Fatalf("expected in-flight state despite emit failure, got %#v", updated)
		}
	})

	t.Run("with runner recovery mutation surfaces emit failure", func(t *testing.T) {
		sessions := session.NewMemoryStore()
		audits := audit.NewMemoryStore()
		runner := &countingRunner{repos: persistence.RepositorySet{
			Sessions: sessions,
			Audits:   audits,
		}}
		boom := errors.New("boom:recovery.state_changed")
		rt := hruntime.New(hruntime.Options{
			Sessions: sessions,
			Audit:    audits,
			Runner:   runner,
			EventSink: selectiveFailingEventSink{failures: map[string]error{
				audit.EventRecoveryStateChanged: boom,
			}},
		})
		sess := mustCreateSession(t, rt, "recovery runner failure", "recovery mutation should surface emit failure with runner")

		if _, err := rt.MarkSessionInFlight(context.Background(), sess.SessionID, "step_runner_failure"); !errors.Is(err, boom) {
			t.Fatalf("expected runner-backed recovery mutation to surface emit error, got %v", err)
		}
	})
}

func TestRunSessionStopsOnPendingApprovalAndRecoverSessionResumesToCompletion(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
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
	}).WithPolicyEvaluator(askPolicy{})

	sess := mustCreateSession(t, rt, "approval-driven session", "driver should pause for approval and recover later")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "approval-gated driver path"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	if _, err := rt.CreatePlan(sess.SessionID, "approval plan", []plan.StepSpec{{
		StepID: "step_approval_driver",
		Title:  "approval-gated step",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo gated", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
		}},
	}}); err != nil {
		t.Fatalf("create plan: %v", err)
	}

	blocked, err := rt.RunSession(context.Background(), sess.SessionID)
	if err != nil {
		t.Fatalf("run session: %v", err)
	}
	if blocked.Session.PendingApprovalID == "" {
		t.Fatalf("expected session driver to stop on pending approval, got %#v", blocked.Session)
	}
	if handler.calls != 0 {
		t.Fatalf("expected no tool call before approval, got %d", handler.calls)
	}

	if _, _, err := rt.RespondApproval(blocked.Session.PendingApprovalID, approval.Response{Reply: approval.ReplyOnce}); err != nil {
		t.Fatalf("respond approval: %v", err)
	}

	resumed, err := rt.RecoverSession(context.Background(), sess.SessionID)
	if err != nil {
		t.Fatalf("recover session: %v", err)
	}
	if resumed.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected recovered session to complete, got %#v", resumed.Session)
	}
	if handler.calls != 1 {
		t.Fatalf("expected one tool call after recovery resume, got %d", handler.calls)
	}
}

func TestRecoverSessionResumesApprovedMidProgramApprovalAfterRestart(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	approvals := approval.NewMemoryStore()
	attempts := execution.NewMemoryAttemptStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	handler := &countingHandler{}

	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		handler,
	)
	verifiers.Register(
		verify.Definition{Kind: "exit_code", Description: "Verify that an execution result exit code is in the allowed set."},
		verify.ExitCodeChecker{},
	)

	opts := hruntime.Options{
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Approvals: approvals,
		Attempts:  attempts,
		Tools:     tools,
		Verifiers: verifiers,
	}
	rt1 := hruntime.New(opts).WithPolicyEvaluator(askPolicy{})

	sess := mustCreateSession(t, rt1, "mid-program approval restart", "resume approved mid-program approval after restart")
	tsk := mustCreateTask(t, rt1, task.Spec{TaskType: "demo", Goal: "mid-program approval should recover after restart"})
	attached, err := rt1.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	if _, err := rt1.CreatePlan(attached.SessionID, "approval plan", []plan.StepSpec{{
		StepID: "step_mid_program_restart",
		Title:  "approval-gated step across restart",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo restart gated", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
		}},
	}}); err != nil {
		t.Fatalf("create plan: %v", err)
	}

	blocked, err := rt1.RunSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("run session: %v", err)
	}
	if blocked.Session.PendingApprovalID == "" {
		t.Fatalf("expected pending approval before restart, got %#v", blocked.Session)
	}
	if _, _, err := rt1.RespondApproval(blocked.Session.PendingApprovalID, approval.Response{Reply: approval.ReplyOnce}); err != nil {
		t.Fatalf("respond approval: %v", err)
	}

	rt2 := hruntime.New(opts).WithPolicyEvaluator(askPolicy{})
	out, err := rt2.RecoverSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("recover session after restart: %v", err)
	}
	if out.Session.Phase != session.PhaseComplete || len(out.Executions) != 1 {
		t.Fatalf("expected restarted recovery to resume mid-program approval to completion, got %#v", out)
	}
	if handler.calls != 1 {
		t.Fatalf("expected exactly one real execution after restarted recovery, got %d", handler.calls)
	}
}

func TestRecoverSessionContinuesAfterApprovedSessionApprovalGate(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	approvals := approval.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	handler := &countingHandler{}

	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		handler,
	)
	verifiers.Register(
		verify.Definition{Kind: "exit_code", Description: "Verify that an execution result exit code is in the allowed set."},
		verify.ExitCodeChecker{},
	)

	rt1 := hruntime.New(hruntime.Options{
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Approvals: approvals,
		Tools:     tools,
		Verifiers: verifiers,
	})

	sess := mustCreateSession(t, rt1, "recover session gate", "recover an approved session-entry approval")
	tsk := mustCreateTask(t, rt1, task.Spec{TaskType: "demo", Goal: "session-entry approval must resume after restart"})
	attached, err := rt1.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	if _, err := rt1.CreatePlan(attached.SessionID, "session gate", []plan.StepSpec{{
		StepID: "step_after_recovery_gate",
		Title:  "execute after recovery gate",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo recovered gate", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
		}},
	}}); err != nil {
		t.Fatalf("create plan: %v", err)
	}

	approvalRec, blockedState, err := rt1.RequestSessionApproval(context.Background(), attached.SessionID, hruntime.SessionApprovalRequest{
		Reason:      "approve session before recovery",
		MatchedRule: "test/session-gate",
	})
	if err != nil {
		t.Fatalf("request session approval: %v", err)
	}
	if blockedState.PendingApprovalID != approvalRec.ApprovalID {
		t.Fatalf("expected pending approval to be recorded on the session, got %#v", blockedState)
	}
	if _, _, err := rt1.RespondApproval(approvalRec.ApprovalID, approval.Response{Reply: approval.ReplyOnce}); err != nil {
		t.Fatalf("respond approval: %v", err)
	}

	rt2 := hruntime.New(hruntime.Options{
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Approvals: approvals,
		Tools:     tools,
		Verifiers: verifiers,
	})

	out, err := rt2.RecoverSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("recover session: %v", err)
	}
	if out.Session.Phase != session.PhaseComplete || out.Session.PendingApprovalID != "" {
		t.Fatalf("expected recovery to clear the approved session gate and complete, got %#v", out.Session)
	}
	if len(out.Executions) != 1 || out.Executions[0].Execution.Step.StepID != "step_after_recovery_gate" {
		t.Fatalf("expected recovery to continue with the first real plan step, got %#v", out.Executions)
	}
	if handler.calls != 1 {
		t.Fatalf("expected one real tool execution after recovery, got %d", handler.calls)
	}
}

func TestRecoverSessionAfterRejectedSessionApprovalGateStaysFailedWithoutExecution(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	approvals := approval.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	handler := &countingHandler{}

	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		handler,
	)
	verifiers.Register(
		verify.Definition{Kind: "exit_code", Description: "Verify that an execution result exit code is in the allowed set."},
		verify.ExitCodeChecker{},
	)

	opts := hruntime.Options{
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Approvals: approvals,
		Tools:     tools,
		Verifiers: verifiers,
	}
	rt1 := hruntime.New(opts)

	sess := mustCreateSession(t, rt1, "reject request-level approval restart", "request-level rejection should stay failed after restart")
	tsk := mustCreateTask(t, rt1, task.Spec{TaskType: "demo", Goal: "rejected session gate must not re-execute after restart"})
	attached, err := rt1.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	if _, err := rt1.CreatePlan(attached.SessionID, "session gate", []plan.StepSpec{{
		StepID: "step_after_rejected_gate",
		Title:  "must never execute after rejected gate",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo should not run", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
		}},
	}}); err != nil {
		t.Fatalf("create plan: %v", err)
	}

	approvalRec, _, err := rt1.RequestSessionApproval(context.Background(), attached.SessionID, hruntime.SessionApprovalRequest{
		Reason: "reject the whole request before execution",
	})
	if err != nil {
		t.Fatalf("request session approval: %v", err)
	}
	if _, stateAfterReply, err := rt1.RespondApproval(approvalRec.ApprovalID, approval.Response{Reply: approval.ReplyReject}); err != nil {
		t.Fatalf("respond approval reject: %v", err)
	} else if stateAfterReply.Phase != session.PhaseFailed {
		t.Fatalf("expected rejected gate to fail the session before restart, got %#v", stateAfterReply)
	}

	rt2 := hruntime.New(opts)
	out, err := rt2.RecoverSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("recover rejected gate session: %v", err)
	}
	if out.Session.Phase != session.PhaseFailed || len(out.Executions) != 0 {
		t.Fatalf("expected rejected request-level approval to stay failed without execution after restart, got %#v", out)
	}
	if handler.calls != 0 {
		t.Fatalf("expected no tool execution after rejected request-level recovery, got %d", handler.calls)
	}
}

func TestResumePendingApprovalRejectsRepeatedResumeAfterRestart(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	approvals := approval.NewMemoryStore()
	attempts := execution.NewMemoryAttemptStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	handler := &countingHandler{}

	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		handler,
	)
	verifiers.Register(
		verify.Definition{Kind: "exit_code", Description: "Verify that an execution result exit code is in the allowed set."},
		verify.ExitCodeChecker{},
	)

	opts := hruntime.Options{
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Approvals: approvals,
		Attempts:  attempts,
		Tools:     tools,
		Verifiers: verifiers,
	}
	rt1 := hruntime.New(opts).WithPolicyEvaluator(askPolicy{})

	sess := mustCreateSession(t, rt1, "repeated resume restart", "second restart-time resume must not replay execution")
	tsk := mustCreateTask(t, rt1, task.Spec{TaskType: "demo", Goal: "resume once, then reject repeated resume attempts"})
	attached, err := rt1.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt1.CreatePlan(attached.SessionID, "approval", []plan.StepSpec{{
		StepID: "step_resume_once_after_restart",
		Title:  "approval-gated step resumed once",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo resume once", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
		}},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	initial, err := rt1.RunStep(context.Background(), attached.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	if _, _, err := rt1.RespondApproval(initial.Execution.PendingApproval.ApprovalID, approval.Response{Reply: approval.ReplyOnce}); err != nil {
		t.Fatalf("respond approval: %v", err)
	}

	rt2 := hruntime.New(opts).WithPolicyEvaluator(askPolicy{})
	firstResume, err := rt2.ResumePendingApproval(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("first resume after restart: %v", err)
	}
	if firstResume.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected first resumed execution to complete, got %#v", firstResume.Session)
	}
	if handler.calls != 1 {
		t.Fatalf("expected one real execution after first restarted resume, got %d", handler.calls)
	}

	rt3 := hruntime.New(opts).WithPolicyEvaluator(askPolicy{})
	if _, err := rt3.ResumePendingApproval(context.Background(), attached.SessionID); !errors.Is(err, hruntime.ErrNoPendingApproval) {
		t.Fatalf("expected repeated restart-time resume to fail with ErrNoPendingApproval, got %v", err)
	}
	if handler.calls != 1 {
		t.Fatalf("expected repeated restart-time resume not to replay execution, got %d calls", handler.calls)
	}
}

func TestRecoverSessionDoesNotReplayApprovedExecutionAfterSecondRestart(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	approvals := approval.NewMemoryStore()
	attempts := execution.NewMemoryAttemptStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	handler := &countingHandler{}

	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		handler,
	)
	verifiers.Register(
		verify.Definition{Kind: "exit_code", Description: "Verify that an execution result exit code is in the allowed set."},
		verify.ExitCodeChecker{},
	)

	opts := hruntime.Options{
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Approvals: approvals,
		Attempts:  attempts,
		Tools:     tools,
		Verifiers: verifiers,
	}
	rt1 := hruntime.New(opts).WithPolicyEvaluator(askPolicy{})

	sess := mustCreateSession(t, rt1, "recovery approval replay once", "recover should consume approval exactly once across restarts")
	tsk := mustCreateTask(t, rt1, task.Spec{TaskType: "demo", Goal: "recover should not replay approved work twice"})
	attached, err := rt1.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	if _, err := rt1.CreatePlan(attached.SessionID, "approval plan", []plan.StepSpec{{
		StepID: "step_recover_approval_once",
		Title:  "approval-gated step recovered once",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo approval-once", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
		}},
	}}); err != nil {
		t.Fatalf("create plan: %v", err)
	}

	blocked, err := rt1.RunSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("run session: %v", err)
	}
	if blocked.Session.PendingApprovalID == "" {
		t.Fatalf("expected pending approval before restart, got %#v", blocked.Session)
	}
	if _, _, err := rt1.RespondApproval(blocked.Session.PendingApprovalID, approval.Response{Reply: approval.ReplyOnce}); err != nil {
		t.Fatalf("respond approval: %v", err)
	}

	rt2 := hruntime.New(opts).WithPolicyEvaluator(askPolicy{})
	first, err := rt2.RecoverSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("first recover session: %v", err)
	}
	if first.Session.Phase != session.PhaseComplete || len(first.Executions) != 1 {
		t.Fatalf("expected first recovery to execute the approved step once, got %#v", first)
	}
	if handler.calls != 1 {
		t.Fatalf("expected one real execution after first recovery, got %d", handler.calls)
	}
	attemptsAfterFirst := mustListAttempts(t, rt2, attached.SessionID)
	if len(attemptsAfterFirst) != 1 || attemptsAfterFirst[0].Status != execution.AttemptCompleted {
		t.Fatalf("expected one completed attempt after first recovery, got %#v", attemptsAfterFirst)
	}

	rt3 := hruntime.New(opts).WithPolicyEvaluator(askPolicy{})
	second, err := rt3.RecoverSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("second recover session: %v", err)
	}
	if second.Session.Phase != session.PhaseComplete || len(second.Executions) != 0 {
		t.Fatalf("expected second recovery to stay terminal without replay, got %#v", second)
	}
	if handler.calls != 1 {
		t.Fatalf("expected second recovery not to replay approved execution, got %d calls", handler.calls)
	}
	attemptsAfterSecond := mustListAttempts(t, rt3, attached.SessionID)
	if len(attemptsAfterSecond) != len(attemptsAfterFirst) {
		t.Fatalf("expected second recovery not to create extra attempts, got before=%#v after=%#v", attemptsAfterFirst, attemptsAfterSecond)
	}

	storedApproval, err := rt3.GetApproval(blocked.Session.PendingApprovalID)
	if err != nil {
		t.Fatalf("get approval after second recovery: %v", err)
	}
	if storedApproval.Status != approval.StatusConsumed {
		t.Fatalf("expected approval to remain consumed after second recovery, got %#v", storedApproval)
	}
}

func TestRecoverSessionDoesNotRepeatRuntimeHandleInvalidationAfterSecondRestart(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	runtimeHandles := execution.NewMemoryRuntimeHandleStore()
	audits := audit.NewMemoryStore()

	opts := hruntime.Options{
		Sessions:       sessions,
		Tasks:          tasks,
		Plans:          plans,
		RuntimeHandles: runtimeHandles,
		Audit:          audits,
	}
	builtins.Register(&opts)

	rt1 := hruntime.New(opts)
	sess := mustCreateSession(t, rt1, "recover handles once", "recovery should invalidate stale handles only once")
	tsk := mustCreateTask(t, rt1, task.Spec{TaskType: "demo", Goal: "recovery invalidation should be idempotent across restarts"})
	attached, err := rt1.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	if _, err := rt1.CreatePlan(attached.SessionID, "recover runtime handle", []plan.StepSpec{{
		StepID: "step_recover_runtime_handle_once",
		Title:  "recover after stale handle once",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo recovered-once", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
			{Kind: "output_contains", Args: map[string]any{"text": "recovered-once"}},
		}},
	}}); err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := rt1.MarkSessionInterrupted(context.Background(), attached.SessionID); err != nil {
		t.Fatalf("mark interrupted: %v", err)
	}
	if _, err := runtimeHandles.Create(execution.RuntimeHandle{
		HandleID:  "hdl_recover_runtime_once",
		SessionID: attached.SessionID,
		TaskID:    attached.TaskID,
		Kind:      "pty",
		Value:     "pty-recover-once",
		Status:    execution.RuntimeHandleActive,
	}); err != nil {
		t.Fatalf("seed runtime handle: %v", err)
	}

	rt2 := hruntime.New(opts)
	first, err := rt2.RecoverSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("first recover session: %v", err)
	}
	if first.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected first recovery to complete, got %#v", first.Session)
	}
	handleAfterFirst, err := rt2.GetRuntimeHandle("hdl_recover_runtime_once")
	if err != nil {
		t.Fatalf("get runtime handle after first recovery: %v", err)
	}
	if handleAfterFirst.Status != execution.RuntimeHandleInvalidated || handleAfterFirst.InvalidatedAt == 0 || handleAfterFirst.StatusReason != "session recovered" {
		t.Fatalf("expected first recovery to invalidate the stale handle, got %#v", handleAfterFirst)
	}
	eventsAfterFirst := mustListAuditEvents(t, rt2, attached.SessionID)
	invalidationsAfterFirst := 0
	for _, event := range eventsAfterFirst {
		if event.Type == audit.EventRuntimeHandleInvalidated && event.CausationID == "hdl_recover_runtime_once" {
			invalidationsAfterFirst++
		}
	}
	if invalidationsAfterFirst != 1 {
		t.Fatalf("expected exactly one invalidation event after first recovery, got %#v", eventsAfterFirst)
	}

	rt3 := hruntime.New(opts)
	second, err := rt3.RecoverSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("second recover session: %v", err)
	}
	if second.Session.Phase != session.PhaseComplete || len(second.Executions) != 0 {
		t.Fatalf("expected second recovery to stay terminal without new executions, got %#v", second)
	}
	handleAfterSecond, err := rt3.GetRuntimeHandle("hdl_recover_runtime_once")
	if err != nil {
		t.Fatalf("get runtime handle after second recovery: %v", err)
	}
	if handleAfterSecond.Version != handleAfterFirst.Version || handleAfterSecond.InvalidatedAt != handleAfterFirst.InvalidatedAt || handleAfterSecond.Status != execution.RuntimeHandleInvalidated {
		t.Fatalf("expected second recovery not to re-invalidate the stale handle, got before=%#v after=%#v", handleAfterFirst, handleAfterSecond)
	}
	eventsAfterSecond := mustListAuditEvents(t, rt3, attached.SessionID)
	invalidationsAfterSecond := 0
	for _, event := range eventsAfterSecond {
		if event.Type == audit.EventRuntimeHandleInvalidated && event.CausationID == "hdl_recover_runtime_once" {
			invalidationsAfterSecond++
		}
	}
	if invalidationsAfterSecond != invalidationsAfterFirst {
		t.Fatalf("expected second recovery not to emit extra invalidation events, got before=%d after=%d", invalidationsAfterFirst, invalidationsAfterSecond)
	}
}

func TestRecoverSessionDoesNotRepeatInteractiveCloseAfterSecondRestart(t *testing.T) {
	controller := &stubInteractiveController{}
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	approvals := approval.NewMemoryStore()
	attempts := execution.NewMemoryAttemptStore()
	actions := execution.NewMemoryActionStore()
	verifications := execution.NewMemoryVerificationStore()
	artifacts := execution.NewMemoryArtifactStore()
	runtimeHandles := execution.NewMemoryRuntimeHandleStore()

	opts := hruntime.Options{
		Sessions:              sessions,
		Tasks:                 tasks,
		Plans:                 plans,
		Approvals:             approvals,
		Attempts:              attempts,
		Actions:               actions,
		Verifications:         verifications,
		Artifacts:             artifacts,
		RuntimeHandles:        runtimeHandles,
		InteractiveController: controller,
		Verifiers:             verify.NewRegistry(),
	}

	rt1 := hruntime.New(opts)
	sess := mustCreateSession(t, rt1, "recover interactive close once", "recovery should close native interactive handles only once")
	tsk := mustCreateTask(t, rt1, task.Spec{TaskType: "demo", Goal: "second recovery should not repeat native interactive close"})
	attached, err := rt1.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	if _, err := rt1.CreatePlanFromProgram(attached.SessionID, "", execution.Program{
		ProgramID: "prog_recover_interactive_close_once",
		Nodes: []execution.ProgramNode{
			{
				NodeID: "node_start",
				Action: action.Spec{
					ToolName: hruntime.ProgramInteractiveStartToolName,
					Args: map[string]any{
						"handle_id": "hdl_recover_interactive_close_once",
						"kind":      "stub",
					},
				},
			},
			{
				NodeID:    "node_close",
				Action:    action.Spec{ToolName: hruntime.ProgramInteractiveCloseToolName, Args: map[string]any{"reason": "approved"}},
				DependsOn: []string{"node_start"},
				InputBinds: []execution.ProgramInputBinding{{
					Name: "handle",
					Kind: execution.ProgramInputBindingRuntimeHandleRef,
					RuntimeHandle: &execution.RuntimeHandleRef{
						StepID: "node_start",
					},
				}},
			},
		},
	}); err != nil {
		t.Fatalf("create plan from program: %v", err)
	}

	approvalRec, blockedState, err := rt1.RequestSessionApproval(context.Background(), attached.SessionID, hruntime.SessionApprovalRequest{
		Reason:      "approve native interactive recovery once",
		MatchedRule: "test/recover-interactive-close-once",
	})
	if err != nil {
		t.Fatalf("request session approval: %v", err)
	}
	if blockedState.PendingApprovalID != approvalRec.ApprovalID {
		t.Fatalf("expected pending approval on the session gate, got %#v", blockedState)
	}
	if _, _, err := rt1.RespondApproval(approvalRec.ApprovalID, approval.Response{Reply: approval.ReplyOnce}); err != nil {
		t.Fatalf("respond approval: %v", err)
	}

	rt2 := hruntime.New(opts)
	first, err := rt2.RecoverSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("first recover session: %v", err)
	}
	if first.Session.Phase != session.PhaseComplete || len(first.Executions) != 2 {
		t.Fatalf("expected first recovery to execute native interactive start and close, got %#v", first)
	}
	if controller.startCalls != 1 || controller.closeCalls != 1 {
		t.Fatalf("expected first recovery to perform one native interactive start/close, got start=%d close=%d", controller.startCalls, controller.closeCalls)
	}
	handleAfterFirst, err := rt2.GetRuntimeHandle("hdl_recover_interactive_close_once")
	if err != nil {
		t.Fatalf("get runtime handle after first recovery: %v", err)
	}
	if handleAfterFirst.Status != execution.RuntimeHandleClosed || handleAfterFirst.Version != 2 {
		t.Fatalf("expected first recovery to persist one closed handle, got %#v", handleAfterFirst)
	}
	attemptsAfterFirst := mustListAttempts(t, rt2, attached.SessionID)
	if len(attemptsAfterFirst) != 2 {
		t.Fatalf("expected two attempts after first recovery, got %#v", attemptsAfterFirst)
	}

	rt3 := hruntime.New(opts)
	second, err := rt3.RecoverSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("second recover session: %v", err)
	}
	if second.Session.Phase != session.PhaseComplete || len(second.Executions) != 0 {
		t.Fatalf("expected second recovery to stay terminal without re-closing the handle, got %#v", second)
	}
	if controller.startCalls != 1 || controller.closeCalls != 1 {
		t.Fatalf("expected second recovery not to repeat native interactive start/close, got start=%d close=%d", controller.startCalls, controller.closeCalls)
	}
	handleAfterSecond, err := rt3.GetRuntimeHandle("hdl_recover_interactive_close_once")
	if err != nil {
		t.Fatalf("get runtime handle after second recovery: %v", err)
	}
	if handleAfterSecond.Version != handleAfterFirst.Version || handleAfterSecond.Status != execution.RuntimeHandleClosed || handleAfterSecond.ClosedAt != handleAfterFirst.ClosedAt {
		t.Fatalf("expected second recovery not to mutate the closed handle, got before=%#v after=%#v", handleAfterFirst, handleAfterSecond)
	}
	attemptsAfterSecond := mustListAttempts(t, rt3, attached.SessionID)
	if len(attemptsAfterSecond) != len(attemptsAfterFirst) {
		t.Fatalf("expected second recovery not to create extra interactive attempts, got before=%#v after=%#v", attemptsAfterFirst, attemptsAfterSecond)
	}
}

func TestRecoverSessionContinuesInterruptedSessionFromPersistedPlan(t *testing.T) {
	opts := hruntime.Options{}
	builtins.Register(&opts)
	rt := hruntime.New(opts)

	sess := mustCreateSession(t, rt, "recover driver", "recover interrupted planned session")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "resume interrupted execution"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	if _, err := rt.CreatePlan(sess.SessionID, "recoverable plan", []plan.StepSpec{{
		StepID: "step_recover_driver",
		Title:  "recoverable step",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo recovered", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
			{Kind: "output_contains", Args: map[string]any{"text": "recovered"}},
		}},
		OnFail: plan.OnFailSpec{Strategy: "abort"},
	}}); err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := rt.MarkSessionInterrupted(context.Background(), sess.SessionID); err != nil {
		t.Fatalf("mark interrupted: %v", err)
	}

	out, err := rt.RecoverSession(context.Background(), sess.SessionID)
	if err != nil {
		t.Fatalf("recover session: %v", err)
	}
	if out.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected recovered session to complete, got %#v", out.Session)
	}
	if len(out.Executions) != 1 {
		t.Fatalf("expected one recovered step execution, got %#v", out.Executions)
	}
}

func TestRecoverSessionRejectsActiveRecoverableLeaseWithoutLeaseID(t *testing.T) {
	opts := hruntime.Options{}
	builtins.Register(&opts)
	rt := hruntime.New(opts)

	sess := mustCreateSession(t, rt, "recover claimed", "direct recovery should not bypass an active lease")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "claim before recovery"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	if _, err := rt.CreatePlan(sess.SessionID, "claimed recovery plan", []plan.StepSpec{{
		StepID: "step_claimed_recover",
		Title:  "recover after claim",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo claimed", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
		}},
	}}); err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := rt.MarkSessionInterrupted(context.Background(), sess.SessionID); err != nil {
		t.Fatalf("mark interrupted: %v", err)
	}

	claimed, ok, err := rt.ClaimRecoverableSession(context.Background(), time.Minute)
	if err != nil {
		t.Fatalf("claim recoverable session: %v", err)
	}
	if !ok {
		t.Fatalf("expected recoverable session to be claimed")
	}
	if claimed.SessionID != sess.SessionID {
		t.Fatalf("expected claimed session %s, got %#v", sess.SessionID, claimed)
	}

	if _, err := rt.RecoverSession(context.Background(), sess.SessionID); !errors.Is(err, session.ErrSessionLeaseNotHeld) {
		t.Fatalf("expected direct recovery without lease to fail, got %v", err)
	}
}

func TestRecoverClaimedSessionContinuesInterruptedSession(t *testing.T) {
	opts := hruntime.Options{}
	builtins.Register(&opts)
	rt := hruntime.New(opts)

	sess := mustCreateSession(t, rt, "recover claimed", "claimed recovery should resume work")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "claim before recovery"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	if _, err := rt.CreatePlan(sess.SessionID, "claimed recovery plan", []plan.StepSpec{{
		StepID: "step_claimed_recover",
		Title:  "recover after claim",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo claimed", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
			{Kind: "output_contains", Args: map[string]any{"text": "claimed"}},
		}},
	}}); err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := rt.MarkSessionInterrupted(context.Background(), sess.SessionID); err != nil {
		t.Fatalf("mark interrupted: %v", err)
	}

	claimed, ok, err := rt.ClaimRecoverableSession(context.Background(), time.Minute)
	if err != nil {
		t.Fatalf("claim recoverable session: %v", err)
	}
	if !ok {
		t.Fatalf("expected recoverable session to be claimed")
	}

	out, err := rt.RecoverClaimedSession(context.Background(), sess.SessionID, claimed.LeaseID)
	if err != nil {
		t.Fatalf("recover claimed session: %v", err)
	}
	if out.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected recovered session to complete, got %#v", out.Session)
	}
	if len(out.Executions) != 1 {
		t.Fatalf("expected one recovered step execution, got %#v", out.Executions)
	}
}
