package runtime_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

func TestGetBlockedRuntimeProjectsPendingApproval(t *testing.T) {
	rt := newBlockedRuntimeTestService()

	attached, initial := seedApprovalBlockedSession(t, rt, "blocked runtime session", "project current approval-backed blocked runtime")
	attempts := mustListAttempts(t, rt, attached.SessionID)
	if len(attempts) != 1 {
		t.Fatalf("expected one blocked attempt, got %#v", attempts)
	}

	blocked, err := rt.GetBlockedRuntime(attached.SessionID)
	if err != nil {
		t.Fatalf("get blocked runtime: %v", err)
	}
	if blocked.BlockedRuntimeID != initial.Execution.PendingApproval.ApprovalID {
		t.Fatalf("expected blocked runtime id %q, got %#v", initial.Execution.PendingApproval.ApprovalID, blocked)
	}
	if blocked.Kind != execution.BlockedRuntimeApproval || blocked.Status != execution.BlockedRuntimePending {
		t.Fatalf("unexpected blocked runtime kind/status: %#v", blocked)
	}
	if blocked.WaitingFor != "approval" {
		t.Fatalf("expected waiting_for approval, got %#v", blocked)
	}
	if blocked.SessionID != attached.SessionID || blocked.TaskID != attached.TaskID || blocked.StepID != initial.Execution.PendingApproval.StepID {
		t.Fatalf("unexpected blocked runtime identity fields: %#v", blocked)
	}
	if blocked.ApprovalID != initial.Execution.PendingApproval.ApprovalID || blocked.AttemptID != attempts[0].AttemptID || blocked.CycleID != attempts[0].CycleID {
		t.Fatalf("unexpected blocked runtime linkage fields: %#v", blocked)
	}
	if blocked.Step.StepID != initial.Execution.PendingApproval.Step.StepID {
		t.Fatalf("expected blocked step to come from approval record, got %#v", blocked.Step)
	}
	if blocked.Approval.ApprovalID != initial.Execution.PendingApproval.ApprovalID || blocked.Approval.Status != approval.StatusPending {
		t.Fatalf("expected pending approval payload, got %#v", blocked.Approval)
	}
}

func TestGetBlockedRuntimeByApprovalLooksUpCurrentBlockedRuntime(t *testing.T) {
	rt := newBlockedRuntimeTestService()

	attached, initial := seedApprovalBlockedSession(t, rt, "blocked runtime approval lookup", "look up blocked runtime by approval id")

	bySession, err := rt.GetBlockedRuntime(attached.SessionID)
	if err != nil {
		t.Fatalf("get blocked runtime by session: %v", err)
	}
	byApproval, err := rt.GetBlockedRuntimeByApproval(initial.Execution.PendingApproval.ApprovalID)
	if err != nil {
		t.Fatalf("get blocked runtime by approval: %v", err)
	}
	if byApproval.BlockedRuntimeID != bySession.BlockedRuntimeID || byApproval.SessionID != bySession.SessionID || byApproval.CycleID != bySession.CycleID {
		t.Fatalf("expected blocked runtime lookup shapes to agree, got session=%#v approval=%#v", bySession, byApproval)
	}
}

func TestGetBlockedRuntimeReturnsNotFoundForNonBlockedSession(t *testing.T) {
	rt := newBlockedRuntimeTestService()
	sess := mustCreateSession(t, rt, "not blocked", "no pending approval")

	_, err := rt.GetBlockedRuntime(sess.SessionID)
	if !errors.Is(err, execution.ErrBlockedRuntimeNotFound) {
		t.Fatalf("expected ErrBlockedRuntimeNotFound, got %v", err)
	}
}

func TestGetBlockedRuntimeRejectsMismatchedApprovalSessionProjection(t *testing.T) {
	rt := newBlockedRuntimeTestService()

	firstSession, firstInitial := seedApprovalBlockedSession(t, rt, "blocked first mismatch", "first blocked runtime")
	secondSession, _ := seedApprovalBlockedSession(t, rt, "blocked second mismatch", "second blocked runtime")

	secondStored, err := rt.GetSession(secondSession.SessionID)
	if err != nil {
		t.Fatalf("get second session: %v", err)
	}
	secondStored.PendingApprovalID = firstInitial.Execution.PendingApproval.ApprovalID
	secondStored.Version++
	if err := rt.Sessions.Update(secondStored); err != nil {
		t.Fatalf("corrupt second session pending approval: %v", err)
	}

	_, err = rt.GetBlockedRuntime(secondSession.SessionID)
	if !errors.Is(err, execution.ErrBlockedRuntimeNotFound) {
		t.Fatalf("expected ErrBlockedRuntimeNotFound for mismatched session/approval projection, got %v", err)
	}

	blocked, err := rt.GetBlockedRuntime(firstSession.SessionID)
	if err != nil {
		t.Fatalf("get first blocked runtime: %v", err)
	}
	if blocked.SessionID != firstSession.SessionID {
		t.Fatalf("expected original session blocked runtime to stay intact, got %#v", blocked)
	}
}

func TestListBlockedRuntimesUsesStableOrderingAndOnlyCurrentPendingApprovals(t *testing.T) {
	rt := newBlockedRuntimeTestService()

	firstSession, firstBlocked := seedApprovalBlockedSession(t, rt, "blocked first", "first blocked runtime")
	time.Sleep(2 * time.Millisecond)
	secondSession, secondBlocked := seedApprovalBlockedSession(t, rt, "blocked second", "second blocked runtime")

	thirdSession, thirdInitial := seedApprovalBlockedSession(t, rt, "blocked then resumed", "should disappear after resume")
	if _, _, err := rt.RespondApproval(thirdInitial.Execution.PendingApproval.ApprovalID, approval.Response{Reply: approval.ReplyOnce}); err != nil {
		t.Fatalf("respond approval: %v", err)
	}
	if _, err := rt.ResumePendingApproval(context.Background(), thirdSession.SessionID); err != nil {
		t.Fatalf("resume approval: %v", err)
	}

	items, err := rt.ListBlockedRuntimes()
	if err != nil {
		t.Fatalf("list blocked runtimes: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected two currently blocked runtimes, got %#v", items)
	}
	if items[0].SessionID != firstSession.SessionID || items[1].SessionID != secondSession.SessionID {
		t.Fatalf("expected stable requested_at ordering, got %#v", items)
	}
	for i := 1; i < len(items); i++ {
		prev := items[i-1]
		next := items[i]
		if prev.RequestedAt > next.RequestedAt {
			t.Fatalf("expected requested_at ascending order, got %#v", items)
		}
		if prev.RequestedAt == next.RequestedAt && prev.BlockedRuntimeID > next.BlockedRuntimeID {
			t.Fatalf("expected approval id tie-break ordering, got %#v", items)
		}
	}
	if items[0].ApprovalID != firstBlocked.Execution.PendingApproval.ApprovalID || items[1].ApprovalID != secondBlocked.Execution.PendingApproval.ApprovalID {
		t.Fatalf("unexpected blocked runtime approval ids: %#v", items)
	}
}

func newBlockedRuntimeTestService() *hruntime.Service {
	tools := tool.NewRegistry()
	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		&countingHandler{},
	)
	verifiers := verify.NewRegistry()
	verifiers.Register(
		verify.Definition{Kind: "exit_code", Description: "Verify that an execution result exit code is in the allowed set."},
		verify.ExitCodeChecker{},
	)
	return hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verifiers,
	}).WithPolicyEvaluator(askPolicy{})
}

func seedApprovalBlockedSession(tb testing.TB, rt *hruntime.Service, title, goal string) (taskAttachedSession, hruntime.StepRunOutput) {
	tb.Helper()
	sess := mustCreateSession(tb, rt, title, goal)
	tsk := mustCreateTask(tb, rt, task.Spec{TaskType: "demo", Goal: goal})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		tb.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(attached.SessionID, "approval blocked", planStepSpecForBlockedRuntime())
	if err != nil {
		tb.Fatalf("create plan: %v", err)
	}
	out, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0])
	if err != nil {
		tb.Fatalf("run step: %v", err)
	}
	if out.Execution.PendingApproval == nil {
		tb.Fatalf("expected pending approval, got %#v", out)
	}
	return taskAttachedSession{SessionID: attached.SessionID, TaskID: attached.TaskID}, out
}

type taskAttachedSession struct {
	SessionID string
	TaskID    string
}

func planStepSpecForBlockedRuntime() []plan.StepSpec {
	return []plan.StepSpec{{
		StepID: "step_blocked_runtime",
		Title:  "blocked runtime step",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo blocked runtime", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
		}},
	}}
}
