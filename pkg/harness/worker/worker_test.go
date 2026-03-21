package worker_test

import (
	"context"
	"testing"
	"time"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	workerpkg "github.com/yiiilin/harness-core/pkg/harness/worker"
)

func TestWorkerRunOnceClaimsRunsAndReleases(t *testing.T) {
	ctx := context.Background()
	handler := &sleepHandler{delay: 100 * time.Millisecond}
	rt := newTestRuntime(t, handler)
	sess := seedRunnableSession(t, rt)

	w, err := workerpkg.New(workerpkg.Options{
		Runtime:       rt,
		LeaseTTL:      time.Minute,
		RenewInterval: 25 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	res, err := w.RunOnce(ctx)
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if res.NoWork {
		t.Fatalf("expected work, got no work result")
	}
	if res.Mode != session.ClaimModeRunnable {
		t.Fatalf("expected runnable claim mode, got %q", res.Mode)
	}
	if res.Run.Session.SessionID != sess.SessionID {
		t.Fatalf("run session mismatch: want %q got %q", sess.SessionID, res.Run.Session.SessionID)
	}
	if len(res.Run.Executions) != 1 {
		t.Fatalf("expected one execution, got %d", len(res.Run.Executions))
	}
	if res.RenewalCount < 1 {
		t.Fatalf("expected at least one renewal, got %d", res.RenewalCount)
	}
	if res.Released.LeaseID != "" {
		t.Fatalf("expected released lease cleared, got %q", res.Released.LeaseID)
	}
	if handler.calls != 1 {
		t.Fatalf("expected tool handler called once, got %d", handler.calls)
	}
}

func TestWorkerRunOnceResumesRecoverableSession(t *testing.T) {
	ctx := context.Background()
	handler := &sleepHandler{delay: 50 * time.Millisecond}
	rt := newTestRuntime(t, handler)
	sess := seedRunnableSession(t, rt)
	if _, err := rt.MarkSessionInterrupted(ctx, sess.SessionID); err != nil {
		t.Fatalf("mark interrupted: %v", err)
	}

	w, err := workerpkg.New(workerpkg.Options{
		Runtime:       rt,
		LeaseTTL:      time.Minute,
		RenewInterval: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	res, err := w.RunOnce(ctx)
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if res.Mode != session.ClaimModeRecoverable {
		t.Fatalf("expected recoverable claim mode, got %q", res.Mode)
	}
	if len(res.Run.Executions) == 0 {
		t.Fatalf("expected executions from recoverable session")
	}
}

func TestWorkerRunOnceReportsApprovalPause(t *testing.T) {
	ctx := context.Background()
	handler := &sleepHandler{}
	rt := newTestRuntime(t, handler)
	rt.WithPolicyEvaluator(askPolicy{})
	_ = seedRunnableSession(t, rt)

	w, err := workerpkg.New(workerpkg.Options{
		Runtime:       rt,
		LeaseTTL:      time.Minute,
		RenewInterval: 25 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	res, err := w.RunOnce(ctx)
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if !res.ApprovalPending {
		t.Fatalf("expected approval pending result")
	}
	if res.Run.Session.PendingApprovalID == "" {
		t.Fatalf("expected pending approval id, got empty")
	}
	if handler.calls != 0 {
		t.Fatalf("expected tool not invoked before approval, got %d calls", handler.calls)
	}
}

func TestWorkerRunOnceReportsNoWork(t *testing.T) {
	ctx := context.Background()
	rt := hruntime.New(hruntime.Options{})
	w, err := workerpkg.New(workerpkg.Options{
		Runtime:       rt,
		LeaseTTL:      time.Minute,
		RenewInterval: 25 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	res, err := w.RunOnce(ctx)
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if !res.NoWork {
		t.Fatalf("expected no work result when nothing to claim")
	}
}

func newTestRuntime(t *testing.T, handler tool.Handler) *hruntime.Service {
	t.Helper()
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{
		ToolName:       "test.tool",
		Version:        "v1",
		CapabilityType: "executor",
		RiskLevel:      tool.RiskLow,
		Enabled:        true,
	}, handler)
	return hruntime.New(hruntime.Options{Tools: tools})
}

func seedRunnableSession(t *testing.T, rt *hruntime.Service) session.State {
	t.Helper()
	sess, err := rt.CreateSession("worker session", "run a single step")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	tsk, err := rt.CreateTask(task.Spec{TaskType: "demo", Goal: "runnable"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID); err != nil {
		t.Fatalf("attach task: %v", err)
	}
	if _, err := rt.CreatePlan(sess.SessionID, "test", []plan.StepSpec{{
		StepID: "run",
		Title:  "run test tool",
		Action: action.Spec{ToolName: "test.tool"},
		OnFail: plan.OnFailSpec{Strategy: "abort"},
	}}); err != nil {
		t.Fatalf("create plan: %v", err)
	}
	return sess
}

type sleepHandler struct {
	delay time.Duration
	calls int
}

func (h *sleepHandler) Invoke(ctx context.Context, args map[string]any) (action.Result, error) {
	if h.delay > 0 {
		select {
		case <-ctx.Done():
			return action.Result{}, ctx.Err()
		case <-time.After(h.delay):
		}
	}
	h.calls++
	return action.Result{OK: true, Data: map[string]any{"status": "done"}}, nil
}

type askPolicy struct{}

func (askPolicy) Evaluate(ctx context.Context, _ session.State, _ plan.StepSpec) (permission.Decision, error) {
	_ = ctx
	return permission.Decision{Action: permission.Ask, Reason: "approval required", MatchedRule: "test/ask"}, nil
}
