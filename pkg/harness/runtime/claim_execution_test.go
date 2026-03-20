package runtime_test

import (
	"context"
	"errors"
	"testing"
	"time"

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

func newClaimExecutionRuntime(t *testing.T, policy any) (*hruntime.Service, *countingHandler) {
	t.Helper()

	sessions := session.NewMemoryStore()
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
	verifiers.Register(
		verify.Definition{Kind: "output_contains", Description: "Verify that output contains text."},
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
	switch p := policy.(type) {
	case askPolicy:
		rt = rt.WithPolicyEvaluator(p)
	case nil:
	default:
		t.Fatalf("unsupported policy type %T", policy)
	}
	return rt, handler
}

func createClaimedExecutionPlan(t *testing.T, rt *hruntime.Service, title, goal, stepID, command string) (session.State, plan.Spec) {
	t.Helper()

	sess := mustCreateSession(t, rt, title, goal)
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: goal})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(attached.SessionID, "claim execution", []plan.StepSpec{{
		StepID: stepID,
		Title:  title,
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": command, "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
		}},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	return attached, pl
}

func TestRunClaimedStepRequiresLeaseOwnership(t *testing.T) {
	rt, handler := newClaimExecutionRuntime(t, nil)
	sess, pl := createClaimedExecutionPlan(t, rt, "claimed step", "require lease for step execution", "step_claimed_step", "echo claimed-step")

	claimed, ok, err := rt.ClaimRunnableSession(context.Background(), time.Minute)
	if err != nil {
		t.Fatalf("claim runnable session: %v", err)
	}
	if !ok || claimed.SessionID != sess.SessionID {
		t.Fatalf("expected claimed session %s, got %#v ok=%v", sess.SessionID, claimed, ok)
	}

	if _, err := rt.RunStep(context.Background(), sess.SessionID, pl.Steps[0]); !errors.Is(err, session.ErrSessionLeaseNotHeld) {
		t.Fatalf("expected direct run step to fail under active lease, got %v", err)
	}

	out, err := rt.RunClaimedStep(context.Background(), sess.SessionID, claimed.LeaseID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run claimed step: %v", err)
	}
	if handler.calls != 1 {
		t.Fatalf("expected one tool execution, got %d", handler.calls)
	}
	if out.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected session complete, got %#v", out.Session)
	}
}

func TestRunClaimedSessionRequiresLeaseOwnership(t *testing.T) {
	rt, handler := newClaimExecutionRuntime(t, nil)
	sess, _ := createClaimedExecutionPlan(t, rt, "claimed session", "require lease for session execution", "step_claimed_session", "echo claimed-session")

	claimed, ok, err := rt.ClaimRunnableSession(context.Background(), time.Minute)
	if err != nil {
		t.Fatalf("claim runnable session: %v", err)
	}
	if !ok || claimed.SessionID != sess.SessionID {
		t.Fatalf("expected claimed session %s, got %#v ok=%v", sess.SessionID, claimed, ok)
	}

	if _, err := rt.RunSession(context.Background(), sess.SessionID); !errors.Is(err, session.ErrSessionLeaseNotHeld) {
		t.Fatalf("expected direct run session to fail under active lease, got %v", err)
	}

	out, err := rt.RunClaimedSession(context.Background(), sess.SessionID, claimed.LeaseID)
	if err != nil {
		t.Fatalf("run claimed session: %v", err)
	}
	if handler.calls != 1 {
		t.Fatalf("expected one tool execution, got %d", handler.calls)
	}
	if out.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected session complete, got %#v", out.Session)
	}
}

func TestClaimedApprovalResumeRequiresLeaseOwnership(t *testing.T) {
	rt, handler := newClaimExecutionRuntime(t, askPolicy{})
	sess, pl := createClaimedExecutionPlan(t, rt, "claimed approval", "resume approved work under claim", "step_claimed_approval", "echo claimed-approval")

	initial, err := rt.RunStep(context.Background(), sess.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	if initial.Execution.PendingApproval == nil {
		t.Fatalf("expected pending approval")
	}
	if _, _, err := rt.RespondApproval(initial.Execution.PendingApproval.ApprovalID, approval.Response{Reply: approval.ReplyOnce}); err != nil {
		t.Fatalf("respond approval: %v", err)
	}

	claimed, ok, err := rt.ClaimRunnableSession(context.Background(), time.Minute)
	if err != nil {
		t.Fatalf("claim runnable session: %v", err)
	}
	if !ok || claimed.SessionID != sess.SessionID {
		t.Fatalf("expected claimed session %s, got %#v ok=%v", sess.SessionID, claimed, ok)
	}

	if _, err := rt.ResumePendingApproval(context.Background(), sess.SessionID); !errors.Is(err, session.ErrSessionLeaseNotHeld) {
		t.Fatalf("expected direct approval resume to fail under active lease, got %v", err)
	}

	resumed, err := rt.ResumeClaimedApproval(context.Background(), sess.SessionID, claimed.LeaseID)
	if err != nil {
		t.Fatalf("resume claimed approval: %v", err)
	}
	if handler.calls != 1 {
		t.Fatalf("expected one tool execution after claimed resume, got %d", handler.calls)
	}
	if resumed.Session.PendingApprovalID != "" || resumed.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected resumed session to complete and clear approval, got %#v", resumed.Session)
	}
}

func TestRunClaimedSessionResumesApprovedPendingApproval(t *testing.T) {
	rt, handler := newClaimExecutionRuntime(t, askPolicy{})
	sess, pl := createClaimedExecutionPlan(t, rt, "claimed approval session", "claimed session driver should resume approved work", "step_claimed_approval_session", "echo claimed-approval-session")

	initial, err := rt.RunStep(context.Background(), sess.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	if initial.Execution.PendingApproval == nil {
		t.Fatalf("expected pending approval")
	}
	if _, _, err := rt.RespondApproval(initial.Execution.PendingApproval.ApprovalID, approval.Response{Reply: approval.ReplyOnce}); err != nil {
		t.Fatalf("respond approval: %v", err)
	}

	claimed, ok, err := rt.ClaimRunnableSession(context.Background(), time.Minute)
	if err != nil {
		t.Fatalf("claim runnable session: %v", err)
	}
	if !ok || claimed.SessionID != sess.SessionID {
		t.Fatalf("expected claimed session %s, got %#v ok=%v", sess.SessionID, claimed, ok)
	}

	out, err := rt.RunClaimedSession(context.Background(), sess.SessionID, claimed.LeaseID)
	if err != nil {
		t.Fatalf("run claimed session: %v", err)
	}
	if handler.calls != 1 {
		t.Fatalf("expected one tool execution after claimed session resume, got %d", handler.calls)
	}
	if out.Session.PendingApprovalID != "" || out.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected claimed session run to consume approval and complete, got %#v", out.Session)
	}
}

func TestClaimedRecoveryStateMutationsRequireLeaseOwnership(t *testing.T) {
	rt, _ := newClaimExecutionRuntime(t, nil)
	sess, _ := createClaimedExecutionPlan(t, rt, "claimed recovery state", "require lease for recovery state writes", "step_claimed_recovery", "echo claimed-recovery")

	claimed, ok, err := rt.ClaimRunnableSession(context.Background(), time.Minute)
	if err != nil {
		t.Fatalf("claim runnable session: %v", err)
	}
	if !ok || claimed.SessionID != sess.SessionID {
		t.Fatalf("expected claimed session %s, got %#v ok=%v", sess.SessionID, claimed, ok)
	}

	if _, err := rt.MarkSessionInFlight(context.Background(), sess.SessionID, "step_claimed_recovery"); !errors.Is(err, session.ErrSessionLeaseNotHeld) {
		t.Fatalf("expected direct mark in-flight to fail under active lease, got %v", err)
	}

	inFlight, err := rt.MarkClaimedSessionInFlight(context.Background(), sess.SessionID, claimed.LeaseID, "step_claimed_recovery")
	if err != nil {
		t.Fatalf("mark claimed session in-flight: %v", err)
	}
	if inFlight.ExecutionState != session.ExecutionInFlight {
		t.Fatalf("expected in-flight execution state, got %#v", inFlight)
	}

	if _, err := rt.MarkSessionInterrupted(context.Background(), sess.SessionID); !errors.Is(err, session.ErrSessionLeaseNotHeld) {
		t.Fatalf("expected direct mark interrupted to fail under active lease, got %v", err)
	}

	interrupted, err := rt.MarkClaimedSessionInterrupted(context.Background(), sess.SessionID, claimed.LeaseID)
	if err != nil {
		t.Fatalf("mark claimed session interrupted: %v", err)
	}
	if interrupted.ExecutionState != session.ExecutionInterrupted || interrupted.Phase != session.PhaseRecover {
		t.Fatalf("expected interrupted recoverable session, got %#v", interrupted)
	}
}
