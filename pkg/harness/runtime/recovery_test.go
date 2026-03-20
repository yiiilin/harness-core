package runtime_test

import (
	"context"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/persistence"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

func TestRecoveryReadPathAcrossRuntimeReinit(t *testing.T) {
	opts := hruntime.Options{}
	hruntime.RegisterBuiltins(&opts)
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
	hruntime.RegisterBuiltins(&opts)
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
