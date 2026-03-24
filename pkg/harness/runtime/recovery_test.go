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
