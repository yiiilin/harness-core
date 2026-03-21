package evals_test

import (
	"context"
	"testing"
	"time"

	"github.com/yiiilin/harness-core/pkg/harness"
	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

func TestWorkflowEvalApprovalResumeReplay(t *testing.T) {
	ctx := context.Background()
	handler := &evalHandler{
		result: action.Result{
			OK: true,
			Data: map[string]any{
				"status":    "completed",
				"exit_code": 0,
				"stdout":    "approval eval complete",
			},
		},
	}
	rt := newEvalRuntime(t, askAllPolicy{}, handler)
	sessionID := seedEvalSession(t, rt, "approval-eval", "approval flow", "approval eval complete")

	workerHelper, err := harness.NewWorkerHelper(harness.WorkerOptions{
		Runtime:       rt,
		LeaseTTL:      time.Minute,
		RenewInterval: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("new worker helper: %v", err)
	}

	first, err := workerHelper.RunOnce(ctx)
	if err != nil {
		t.Fatalf("first run once: %v", err)
	}
	if !first.ApprovalPending {
		t.Fatalf("expected approval pending on first run, got %#v", first)
	}
	if handler.calls != 0 {
		t.Fatalf("expected tool handler not to run before approval, got %d", handler.calls)
	}

	approvals, err := rt.ListApprovals(sessionID)
	if err != nil {
		t.Fatalf("list approvals: %v", err)
	}
	if len(approvals) != 1 || approvals[0].Status != approval.StatusPending {
		t.Fatalf("expected one pending approval, got %#v", approvals)
	}

	if _, _, err := rt.RespondApproval(approvals[0].ApprovalID, harness.ApprovalResponse{Reply: approval.ReplyOnce}); err != nil {
		t.Fatalf("respond approval: %v", err)
	}

	second, err := workerHelper.RunOnce(ctx)
	if err != nil {
		t.Fatalf("second run once: %v", err)
	}
	if second.ApprovalPending || second.Run.Session.Phase != harness.SessionPhase("complete") {
		t.Fatalf("expected completed session after approval resume, got %#v", second)
	}
	if handler.calls != 1 {
		t.Fatalf("expected tool handler to run once after approval, got %d", handler.calls)
	}

	attempts, err := rt.ListAttempts(sessionID)
	if err != nil {
		t.Fatalf("list attempts: %v", err)
	}
	actions, err := rt.ListActions(sessionID)
	if err != nil {
		t.Fatalf("list actions: %v", err)
	}
	verifications, err := rt.ListVerifications(sessionID)
	if err != nil {
		t.Fatalf("list verifications: %v", err)
	}
	if len(attempts) == 0 || len(actions) == 0 || len(verifications) == 0 {
		t.Fatalf("expected persisted execution facts, got attempts=%d actions=%d verifications=%d", len(attempts), len(actions), len(verifications))
	}

	replayReader := harness.NewReplayReader(rt)
	projection, err := replayReader.SessionProjection(sessionID)
	if err != nil {
		t.Fatalf("session projection: %v", err)
	}
	if len(projection.Cycles) != 1 {
		t.Fatalf("expected one logical execution cycle, got %#v", projection)
	}
	if len(projection.Cycles[0].Events) == 0 || len(projection.Events) == 0 {
		t.Fatalf("expected replay projection to include cycle and session events, got %#v", projection)
	}
}

func TestWorkflowEvalRecoverableClaimRun(t *testing.T) {
	ctx := context.Background()
	handler := &evalHandler{
		result: action.Result{
			OK: true,
			Data: map[string]any{
				"status":    "completed",
				"exit_code": 0,
				"stdout":    "recover eval complete",
			},
		},
	}
	rt := newEvalRuntime(t, nil, handler)
	sessionID := seedEvalSession(t, rt, "recover-eval", "recover flow", "recover eval complete")

	claimed, ok, err := rt.ClaimRunnableSession(ctx, time.Minute)
	if err != nil {
		t.Fatalf("claim runnable session: %v", err)
	}
	if !ok {
		t.Fatal("expected runnable session to be claimable")
	}
	if _, err := rt.MarkClaimedSessionInFlight(ctx, sessionID, claimed.LeaseID, "step_eval"); err != nil {
		t.Fatalf("mark in flight: %v", err)
	}
	if _, err := rt.MarkClaimedSessionInterrupted(ctx, sessionID, claimed.LeaseID); err != nil {
		t.Fatalf("mark interrupted: %v", err)
	}
	if _, err := rt.ReleaseSessionLease(ctx, sessionID, claimed.LeaseID); err != nil {
		t.Fatalf("release lease after interruption: %v", err)
	}

	workerHelper, err := harness.NewWorkerHelper(harness.WorkerOptions{
		Runtime:       rt,
		LeaseTTL:      time.Minute,
		RenewInterval: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("new worker helper: %v", err)
	}

	result, err := workerHelper.RunOnce(ctx)
	if err != nil {
		t.Fatalf("recoverable run once: %v", err)
	}
	if result.NoWork || result.ApprovalPending || result.Run.Session.Phase != harness.SessionPhase("complete") {
		t.Fatalf("expected recovered session to complete, got %#v", result)
	}
	if result.Claimed.SessionID != sessionID || result.Released.LeaseID != "" {
		t.Fatalf("expected recoverable claim and lease release, got %#v", result)
	}
	if handler.calls != 1 {
		t.Fatalf("expected tool handler to run once during recovery, got %d", handler.calls)
	}
}

func TestWorkflowEvalRunLoopStopsAfterHandlingWork(t *testing.T) {
	ctx := context.Background()
	handler := &evalHandler{
		result: action.Result{
			OK: true,
			Data: map[string]any{
				"status":    "completed",
				"exit_code": 0,
				"stdout":    "loop eval complete",
			},
		},
	}
	rt := newEvalRuntime(t, nil, handler)
	sessionID := seedEvalSession(t, rt, "loop-eval", "runloop flow", "loop eval complete")

	workerHelper, err := harness.NewWorkerHelper(harness.WorkerOptions{
		Runtime:       rt,
		LeaseTTL:      time.Minute,
		RenewInterval: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("new worker helper: %v", err)
	}

	iterations := 0
	err = workerHelper.RunLoop(ctx, harness.WorkerLoopOptions{
		IdleWait:  10 * time.Millisecond,
		ErrorWait: 10 * time.Millisecond,
		ShouldStop: func(result harness.WorkerResult, err error) bool {
			iterations++
			return err == nil && !result.NoWork && !result.ApprovalPending
		},
	})
	if err != nil {
		t.Fatalf("run loop: %v", err)
	}
	if iterations != 1 || handler.calls != 1 {
		t.Fatalf("expected run loop to stop after one handled iteration, got iterations=%d calls=%d", iterations, handler.calls)
	}

	stored, err := rt.GetSession(sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if stored.Phase != harness.SessionPhase("complete") {
		t.Fatalf("expected session complete after run loop, got %#v", stored)
	}
}

func newEvalRuntime(t *testing.T, policy permission.Evaluator, handler tool.Handler) *harness.Service {
	t.Helper()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	tools.Register(tool.Definition{
		ToolName:       "eval.tool",
		Version:        "v1",
		CapabilityType: "executor",
		RiskLevel:      tool.RiskLow,
		Enabled:        true,
	}, handler)
	verifiers.Register(
		verify.Definition{Kind: "exit_code", Description: "Verify exit code."},
		verify.ExitCodeChecker{},
	)
	verifiers.Register(
		verify.Definition{Kind: "output_contains", Description: "Verify output contains text."},
		verify.OutputContainsChecker{},
	)

	rt := harness.New(harness.Options{
		Tools:     tools,
		Verifiers: verifiers,
	})
	if policy != nil {
		rt = rt.WithPolicyEvaluator(policy)
	}
	return rt
}

func seedEvalSession(t *testing.T, rt *harness.Service, title, goal, expectedOutput string) string {
	t.Helper()
	sess, err := rt.CreateSession(title, goal)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	tsk, err := rt.CreateTask(task.Spec{TaskType: "eval", Goal: goal})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	sess, err = rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	_, err = rt.CreatePlan(sess.SessionID, title, []plan.StepSpec{{
		StepID: "step_eval",
		Title:  title,
		Action: action.Spec{
			ToolName: "eval.tool",
		},
		Verify: verify.Spec{
			Mode: verify.ModeAll,
			Checks: []verify.Check{
				{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
				{Kind: "output_contains", Args: map[string]any{"text": expectedOutput}},
			},
		},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	return sess.SessionID
}

type evalHandler struct {
	calls  int
	result action.Result
}

func (h *evalHandler) Invoke(_ context.Context, _ map[string]any) (action.Result, error) {
	h.calls++
	return h.result, nil
}

type askAllPolicy struct{}

func (askAllPolicy) Evaluate(_ context.Context, _ harness.SessionState, _ harness.StepSpec) (permission.Decision, error) {
	return permission.Decision{Action: permission.Ask, Reason: "eval approval required", MatchedRule: "eval/ask"}, nil
}
