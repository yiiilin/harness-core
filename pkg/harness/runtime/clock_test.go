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

func TestRunStepExtractsRuntimeHandlesUsingInjectedClock(t *testing.T) {
	clock := &fakeClock{now: 1000}
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.handle", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, runtimeHandleHandler{})

	rt := hruntime.New(hruntime.Options{
		Clock:    clock,
		Sessions: session.NewMemoryStoreWithClock(clock),
		Tools:    tools,
	})

	sess := mustCreateSession(t, rt, "handle extraction clock", "runtime handle extraction should use injected clock")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "runtime handle extraction should use injected clock"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(attached.SessionID, "runtime handle extraction clock", []plan.StepSpec{{
		StepID: "step_runtime_handle_clock",
		Title:  "runtime handle extraction clock",
		Action: action.Spec{ToolName: "demo.handle", Args: map[string]any{"mode": "interactive"}},
		Verify: verify.Spec{},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	if _, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0]); err != nil {
		t.Fatalf("run step: %v", err)
	}

	handles, err := rt.ListRuntimeHandles(attached.SessionID)
	if err != nil {
		t.Fatalf("list runtime handles: %v", err)
	}
	if len(handles) != 1 {
		t.Fatalf("expected one runtime handle, got %#v", handles)
	}
	if handles[0].CreatedAt != 1000 || handles[0].UpdatedAt != 1000 {
		t.Fatalf("expected runtime handle timestamps from injected clock, got %#v", handles[0])
	}
}

func TestPlanningLifecycleUsesInjectedClockForRecordsAndObservability(t *testing.T) {
	clock := &fakeClock{now: 1000}
	metricsExporter := &recordingMetricsExporter{}
	traceExporter := &recordingTraceExporter{}

	opts := hruntime.Options{
		Clock:    clock,
		Sessions: session.NewMemoryStoreWithClock(clock),
	}
	builtins.Register(&opts)
	rt := hruntime.New(opts).WithPlanner(sequencePlanner{})
	rt.MetricsExporter = metricsExporter
	rt.TraceExporter = traceExporter

	sess := mustCreateSession(t, rt, "planning clock", "planning should use injected clock")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "planning should use injected clock"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	if _, _, err := rt.CreatePlanFromPlanner(context.Background(), sess.SessionID, "clocked planning", 1); err != nil {
		t.Fatalf("create plan from planner: %v", err)
	}

	records := mustListPlanningRecords(t, rt, sess.SessionID)
	if len(records) != 1 {
		t.Fatalf("expected one planning record, got %#v", records)
	}
	if records[0].StartedAt != 1000 || records[0].FinishedAt != 1000 {
		t.Fatalf("expected planning record timestamps from injected clock, got %#v", records[0])
	}

	sample := mustFindMetricSample(t, metricsExporter.samples, "planning.cycle")
	if sample.RecordedAt != 1000 {
		t.Fatalf("expected planning metric sample to use injected clock, got %#v", sample)
	}
	span := mustFindTraceSpan(t, traceExporter.spans, "planning.cycle")
	if span.StartedAt != 1000 || span.FinishedAt != 1000 {
		t.Fatalf("expected planning trace span timestamps from injected clock, got %#v", span)
	}
}

func TestApprovalResponseUsesInjectedClockForStateAndObservability(t *testing.T) {
	clock := &fakeClock{now: 1000}
	metricsExporter := &recordingMetricsExporter{}
	traceExporter := &recordingTraceExporter{}
	audits := audit.NewMemoryStore()

	opts := hruntime.Options{
		Clock:    clock,
		Sessions: session.NewMemoryStoreWithClock(clock),
		Audit:    audits,
	}
	builtins.Register(&opts)
	opts.Policy = askPolicy{}
	rt := hruntime.New(opts)
	rt.MetricsExporter = metricsExporter
	rt.TraceExporter = traceExporter

	sess := mustCreateSession(t, rt, "approval clock", "approval response should use injected clock")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "approval response should use injected clock"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(sess.SessionID, "approval clock", []plan.StepSpec{{
		StepID: "step_approval_clock",
		Title:  "approval clock",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo approval clock", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}}}},
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

	clock.Advance(2500)
	rec, _, err := rt.RespondApproval(initial.Execution.PendingApproval.ApprovalID, approval.Response{Reply: approval.ReplyOnce})
	if err != nil {
		t.Fatalf("respond approval: %v", err)
	}
	if rec.RespondedAt != 3500 {
		t.Fatalf("expected approval response timestamp from injected clock, got %#v", rec)
	}

	responseSample := mustFindMetricSample(t, metricsExporter.samples, "approval.response")
	if responseSample.RecordedAt != 3500 {
		t.Fatalf("expected approval response metric sample to use injected clock, got %#v", responseSample)
	}
	responseSpan := mustFindTraceSpan(t, traceExporter.spans, "approval.response")
	if responseSpan.StartedAt != 3500 || responseSpan.FinishedAt != 3500 {
		t.Fatalf("expected approval response trace span timestamps from injected clock, got %#v", responseSpan)
	}

	events := mustListAuditEvents(t, rt, sess.SessionID)
	approvalEvent := mustFindAuditEventType(t, events, audit.EventApprovalApproved)
	if approvalEvent.CreatedAt != 3500 {
		t.Fatalf("expected approval audit event to use injected clock, got %#v", approvalEvent)
	}
}

func TestRecoveryStateAndObservabilityUseInjectedClock(t *testing.T) {
	clock := &fakeClock{now: 1000}
	metricsExporter := &recordingMetricsExporter{}
	traceExporter := &recordingTraceExporter{}

	opts := hruntime.Options{
		Clock:    clock,
		Sessions: session.NewMemoryStoreWithClock(clock),
	}
	builtins.Register(&opts)
	rt := hruntime.New(opts)
	rt.MetricsExporter = metricsExporter
	rt.TraceExporter = traceExporter

	sess := mustCreateSession(t, rt, "recovery clock", "recovery state transitions should use injected clock")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "recovery should use injected clock"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(attached.SessionID, "recovery clock", []plan.StepSpec{{
		StepID: "step_recovery_clock",
		Title:  "recovery clock",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo recover", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
			{Kind: "output_contains", Args: map[string]any{"text": "recover"}},
		}},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	clock.Advance(1000)
	inFlight, err := rt.MarkSessionInFlight(context.Background(), attached.SessionID, pl.Steps[0].StepID)
	if err != nil {
		t.Fatalf("mark in-flight: %v", err)
	}
	if inFlight.LastHeartbeatAt != 2000 {
		t.Fatalf("expected in-flight heartbeat from injected clock, got %#v", inFlight)
	}

	clock.Advance(500)
	interrupted, err := rt.MarkSessionInterrupted(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("mark interrupted: %v", err)
	}
	if interrupted.InterruptedAt != 2500 {
		t.Fatalf("expected interrupted timestamp from injected clock, got %#v", interrupted)
	}

	clock.Advance(500)
	if _, err := rt.RecoverSession(context.Background(), attached.SessionID); err != nil {
		t.Fatalf("recover session: %v", err)
	}

	recoverSample := mustFindMetricSample(t, metricsExporter.samples, "session.recover")
	if recoverSample.RecordedAt != 3000 {
		t.Fatalf("expected recovery metric sample to use injected clock, got %#v", recoverSample)
	}
	recoverSpan := mustFindTraceSpan(t, traceExporter.spans, "session.recover")
	if recoverSpan.StartedAt != 3000 || recoverSpan.FinishedAt != 3000 {
		t.Fatalf("expected recovery trace span timestamps from injected clock, got %#v", recoverSpan)
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
