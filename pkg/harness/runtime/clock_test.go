package runtime_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

func TestRunStepUsesInjectedClockForRetryBackoff(t *testing.T) {
	clock := &fakeClock{now: 1000}
	rt, handler := newVerificationFailureRuntimeWithClock(clock)

	sess := mustCreateSession(t, rt, "clock backoff", "backoff should use injected clock")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "backoff should use injected clock"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(attached.SessionID, "clock backoff", []plan.StepSpec{{
		StepID: "step_clock_backoff",
		Title:  "retry with injected clock",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo retry", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{1}}},
		}},
		OnFail: plan.OnFailSpec{Strategy: "retry", BackoffMS: 60000},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	first, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run first step: %v", err)
	}
	retryNotBefore, ok := metadataInt64(first.Execution.Step.Metadata, "retry_not_before")
	if !ok || retryNotBefore != 61000 {
		t.Fatalf("expected retry_not_before to equal injected clock + backoff, got %#v", first.Execution.Step.Metadata)
	}

	clock.Advance(59999)
	if _, err := rt.RunStep(context.Background(), attached.SessionID, first.Execution.Step); !errors.Is(err, hruntime.ErrStepBackoffActive) {
		t.Fatalf("expected backoff gate before retry_not_before, got %v", err)
	}

	clock.Advance(1)
	if _, err := rt.RunStep(context.Background(), attached.SessionID, first.Execution.Step); err != nil {
		t.Fatalf("expected retry to unblock exactly at retry_not_before, got %v", err)
	}
	if handler.calls != 2 {
		t.Fatalf("expected two tool calls after backoff expiry, got %d", handler.calls)
	}
}

func TestLeaseOperationsUseInjectedClock(t *testing.T) {
	clock := &fakeClock{now: 1000}
	rt := hruntime.New(hruntime.Options{
		Clock:    clock,
		Sessions: session.NewMemoryStoreWithClock(clock),
	})

	mustCreateSession(t, rt, "lease clock", "lease timestamps should use injected clock")

	claimed, ok, err := rt.ClaimRunnableSession(context.Background(), time.Minute)
	if err != nil {
		t.Fatalf("claim runnable session: %v", err)
	}
	if !ok {
		t.Fatalf("expected runnable session to be claimed")
	}
	if claimed.LeaseClaimedAt != 1000 || claimed.LeaseExpiresAt != 61000 || claimed.LastHeartbeatAt != 1000 {
		t.Fatalf("expected exact injected lease timestamps, got %#v", claimed)
	}

	clock.Advance(10000)
	renewed, err := rt.RenewSessionLease(context.Background(), claimed.SessionID, claimed.LeaseID, time.Minute)
	if err != nil {
		t.Fatalf("renew session lease: %v", err)
	}
	if renewed.LeaseExpiresAt != 71000 || renewed.LastHeartbeatAt != 11000 {
		t.Fatalf("expected exact injected renew timestamps, got %#v", renewed)
	}

	clock.Advance(5000)
	released, err := rt.ReleaseSessionLease(context.Background(), claimed.SessionID, claimed.LeaseID)
	if err != nil {
		t.Fatalf("release session lease: %v", err)
	}
	if released.LeaseID != "" || released.UpdatedAt != 16000 {
		t.Fatalf("expected exact injected release timestamp, got %#v", released)
	}
}

func TestRuntimeHandleLifecycleUsesInjectedClock(t *testing.T) {
	clock := &fakeClock{now: 1000}
	rt := hruntime.New(hruntime.Options{Clock: clock})

	sess := mustCreateSession(t, rt, "handle clock", "runtime handle timestamps should use injected clock")
	if _, err := rt.RuntimeHandles.Create(execution.RuntimeHandle{
		HandleID:  "hdl_clock",
		SessionID: sess.SessionID,
		CycleID:   "cyc_clock",
		Kind:      "pty",
		Value:     "pty-clock",
		Status:    execution.RuntimeHandleActive,
		CreatedAt: 1000,
		UpdatedAt: 1000,
	}); err != nil {
		t.Fatalf("seed runtime handle: %v", err)
	}

	nextValue := "pty-clock-updated"
	clock.Advance(1000)
	updated, err := rt.UpdateRuntimeHandle(context.Background(), "hdl_clock", hruntime.RuntimeHandleUpdate{Value: &nextValue})
	if err != nil {
		t.Fatalf("update runtime handle: %v", err)
	}
	if updated.UpdatedAt != 2000 {
		t.Fatalf("expected update timestamp from injected clock, got %#v", updated)
	}

	clock.Advance(1000)
	closed, err := rt.CloseRuntimeHandle(context.Background(), "hdl_clock", hruntime.RuntimeHandleCloseRequest{Reason: "clock close"})
	if err != nil {
		t.Fatalf("close runtime handle: %v", err)
	}
	if closed.ClosedAt != 3000 || closed.UpdatedAt != 3000 {
		t.Fatalf("expected close timestamps from injected clock, got %#v", closed)
	}
}

func TestRuntimeBudgetUsesInjectedClock(t *testing.T) {
	clock := &fakeClock{now: 1000}
	sessions := session.NewMemoryStoreWithClock(clock)
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	handler := &countingHandler{}

	tools.Register(tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true}, handler)
	verifiers.Register(verify.Definition{Kind: "exit_code", Description: "Verify exit code."}, verify.ExitCodeChecker{})

	rt := hruntime.New(hruntime.Options{
		Clock:     clock,
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Tools:     tools,
		Verifiers: verifiers,
		LoopBudgets: hruntime.LoopBudgets{
			MaxTotalRuntimeMS: 60000,
		},
	})

	sess := mustCreateSession(t, rt, "budget clock", "runtime budget should use injected clock")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "budget should use injected clock"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	step := plan.StepSpec{
		StepID: "step_budget_clock",
		Title:  "budgeted",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo budget", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
		}},
	}

	clock.Advance(60001)
	if _, err := rt.RunStep(context.Background(), attached.SessionID, step); !errors.Is(err, hruntime.ErrRuntimeBudgetExceeded) {
		t.Fatalf("expected runtime budget to use injected clock, got %v", err)
	}
	if handler.calls != 0 {
		t.Fatalf("expected runtime budget rejection to block tool execution, got %d calls", handler.calls)
	}
}

func newVerificationFailureRuntimeWithClock(clock *fakeClock) (*hruntime.Service, *countingHandler) {
	sessions := session.NewMemoryStoreWithClock(clock)
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	handler := &countingHandler{}

	tools.Register(tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true}, handler)
	verifiers.Register(verify.Definition{Kind: "exit_code", Description: "Verify exit code."}, verify.ExitCodeChecker{})

	return hruntime.New(hruntime.Options{
		Clock:     clock,
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Tools:     tools,
		Verifiers: verifiers,
	}), handler
}

type fakeClock struct {
	now int64
}

func (c *fakeClock) NowMilli() int64 {
	return c.now
}

func (c *fakeClock) Advance(delta int64) {
	c.now += delta
}
