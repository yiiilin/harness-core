package runtime_test

import (
	"context"
	"errors"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
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
