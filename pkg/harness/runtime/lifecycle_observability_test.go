package runtime_test

import (
	"context"
	"testing"
	"time"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/builtins"
	"github.com/yiiilin/harness-core/pkg/harness/observability"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

func TestPlanningLifecycleExportsObservability(t *testing.T) {
	metricsExporter := &recordingMetricsExporter{}
	traceExporter := &recordingTraceExporter{}

	opts := hruntime.Options{}
	builtins.Register(&opts)
	rt := hruntime.New(opts).WithPlanner(sequencePlanner{})
	rt.MetricsExporter = metricsExporter
	rt.TraceExporter = traceExporter

	sess := mustCreateSession(t, rt, "planning observability", "emit planning observability")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "planning observability"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, _, err := rt.CreatePlanFromPlanner(context.Background(), sess.SessionID, "planner derived", 1)
	if err != nil {
		t.Fatalf("create plan from planner: %v", err)
	}

	sample := mustFindMetricSample(t, metricsExporter.samples, "planning.cycle")
	if sample.Labels["session_id"] != sess.SessionID || sample.Labels["task_id"] != tsk.TaskID || sample.Labels["planning_id"] == "" || sample.Labels["plan_id"] != pl.PlanID {
		t.Fatalf("expected correlated planning metric sample, got %#v", sample)
	}
	if sample.Fields["success"] != true || sample.Fields["step_count"] != 1 {
		t.Fatalf("expected successful planning fields, got %#v", sample)
	}

	span := mustFindTraceSpan(t, traceExporter.spans, "planning.cycle")
	if span.SessionID != sess.SessionID || span.TaskID != tsk.TaskID || span.PlanningID == "" {
		t.Fatalf("expected correlated planning trace span, got %#v", span)
	}
	if span.Attributes["plan_id"] != pl.PlanID || span.Attributes["plan_revision"] != pl.Revision {
		t.Fatalf("expected plan attributes on planning trace span, got %#v", span)
	}

	snap := rt.MetricsSnapshot()
	if snap.PlanningRuns != 1 || snap.PlanningFailure != 0 {
		t.Fatalf("expected planning counters to increment, got %#v", snap)
	}
}

func TestApprovalLifecycleExportsObservability(t *testing.T) {
	metricsExporter := &recordingMetricsExporter{}
	traceExporter := &recordingTraceExporter{}

	opts := hruntime.Options{}
	builtins.Register(&opts)
	opts.Policy = askPolicy{}
	rt := hruntime.New(opts)
	rt.MetricsExporter = metricsExporter
	rt.TraceExporter = traceExporter

	sess := mustCreateSession(t, rt, "approval observability", "emit approval observability")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "approval observability"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(sess.SessionID, "approval observability", []plan.StepSpec{{
		StepID: "step_approval_observability",
		Title:  "approval observability",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo approval", "timeout_ms": 5000}},
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
	if _, _, err := rt.RespondApproval(initial.Execution.PendingApproval.ApprovalID, approval.Response{Reply: approval.ReplyOnce}); err != nil {
		t.Fatalf("respond approval: %v", err)
	}

	requestSample := mustFindMetricSample(t, metricsExporter.samples, "approval.request")
	if requestSample.Labels["session_id"] != sess.SessionID || requestSample.Labels["approval_id"] == "" || requestSample.Labels["attempt_id"] == "" || requestSample.Labels["cycle_id"] == "" {
		t.Fatalf("expected correlated approval request metric sample, got %#v", requestSample)
	}
	cycleID := requestSample.Labels["cycle_id"]

	responseSample := mustFindMetricSample(t, metricsExporter.samples, "approval.response")
	if responseSample.Labels["approval_id"] != initial.Execution.PendingApproval.ApprovalID || responseSample.Fields["reply"] != string(approval.ReplyOnce) || responseSample.Labels["cycle_id"] != cycleID {
		t.Fatalf("expected approval response metric sample, got %#v", responseSample)
	}

	requestSpan := mustFindTraceSpan(t, traceExporter.spans, "approval.request")
	if requestSpan.ApprovalID == "" || requestSpan.AttemptID == "" || requestSpan.StepID != pl.Steps[0].StepID || requestSpan.CycleID != cycleID {
		t.Fatalf("expected correlated approval request trace span, got %#v", requestSpan)
	}
	responseSpan := mustFindTraceSpan(t, traceExporter.spans, "approval.response")
	if responseSpan.ApprovalID != initial.Execution.PendingApproval.ApprovalID || responseSpan.Attributes["reply"] != string(approval.ReplyOnce) || responseSpan.CycleID != cycleID {
		t.Fatalf("expected correlated approval response trace span, got %#v", responseSpan)
	}

	snap := rt.MetricsSnapshot()
	if snap.ApprovalRequested != 1 || snap.ApprovalApproved != 1 || snap.ApprovalRejected != 0 {
		t.Fatalf("expected approval counters to increment, got %#v", snap)
	}
}

func TestRecoveryAbortAndLeaseLifecycleExportObservability(t *testing.T) {
	metricsExporter := &recordingMetricsExporter{}
	traceExporter := &recordingTraceExporter{}

	opts := hruntime.Options{}
	builtins.Register(&opts)
	rt := hruntime.New(opts)
	rt.MetricsExporter = metricsExporter
	rt.TraceExporter = traceExporter

	recoverable := mustCreateSession(t, rt, "recover observability", "emit recovery observability")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "recovery observability"})
	recoverable, err := rt.AttachTaskToSession(recoverable.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(recoverable.SessionID, "recoverable", []plan.StepSpec{{
		StepID: "step_recover_observability",
		Title:  "recover observability",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo recovered", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
			{Kind: "output_contains", Args: map[string]any{"text": "recovered"}},
		}},
	}})
	if err != nil {
		t.Fatalf("create recovery plan: %v", err)
	}
	if _, err := rt.MarkSessionInFlight(context.Background(), recoverable.SessionID, pl.Steps[0].StepID); err != nil {
		t.Fatalf("mark in-flight: %v", err)
	}
	if _, err := rt.MarkSessionInterrupted(context.Background(), recoverable.SessionID); err != nil {
		t.Fatalf("mark interrupted: %v", err)
	}
	if _, err := rt.RecoverSession(context.Background(), recoverable.SessionID); err != nil {
		t.Fatalf("recover session: %v", err)
	}

	abortable := mustCreateSession(t, rt, "abort observability", "emit abort observability")
	abortTask := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "abort observability"})
	abortable, err = rt.AttachTaskToSession(abortable.SessionID, abortTask.TaskID)
	if err != nil {
		t.Fatalf("attach abort task: %v", err)
	}
	if _, err := rt.AbortSession(context.Background(), abortable.SessionID, hruntime.AbortRequest{
		Code:   "operator.abort",
		Reason: "stop execution",
	}); err != nil {
		t.Fatalf("abort session: %v", err)
	}

	leaseTarget := mustCreateSession(t, rt, "lease observability", "emit lease observability")
	claimed, ok, err := rt.ClaimRunnableSession(context.Background(), 30*time.Second)
	if err != nil {
		t.Fatalf("claim runnable session: %v", err)
	}
	if !ok || claimed.SessionID != leaseTarget.SessionID {
		t.Fatalf("expected to claim lease target session, got ok=%v state=%#v", ok, claimed)
	}
	if _, err := rt.RenewSessionLease(context.Background(), claimed.SessionID, claimed.LeaseID, 30*time.Second); err != nil {
		t.Fatalf("renew lease: %v", err)
	}
	if _, err := rt.ReleaseSessionLease(context.Background(), claimed.SessionID, claimed.LeaseID); err != nil {
		t.Fatalf("release lease: %v", err)
	}

	recoverSample := mustFindMetricSample(t, metricsExporter.samples, "session.recover")
	if recoverSample.Labels["session_id"] != recoverable.SessionID || recoverSample.Fields["success"] != true {
		t.Fatalf("expected recovery metric sample, got %#v", recoverSample)
	}
	abortSample := mustFindMetricSample(t, metricsExporter.samples, "session.abort")
	if abortSample.Labels["session_id"] != abortable.SessionID || abortSample.Fields["code"] != "operator.abort" {
		t.Fatalf("expected abort metric sample, got %#v", abortSample)
	}
	claimSample := mustFindMetricSample(t, metricsExporter.samples, "lease.claim")
	if claimSample.Labels["session_id"] != claimed.SessionID || claimSample.Labels["lease_id"] != claimed.LeaseID || claimSample.Fields["claimed"] != true {
		t.Fatalf("expected lease claim metric sample, got %#v", claimSample)
	}
	renewSample := mustFindMetricSample(t, metricsExporter.samples, "lease.renew")
	if renewSample.Labels["lease_id"] != claimed.LeaseID {
		t.Fatalf("expected lease renew metric sample, got %#v", renewSample)
	}
	releaseSample := mustFindMetricSample(t, metricsExporter.samples, "lease.release")
	if releaseSample.Labels["lease_id"] != claimed.LeaseID {
		t.Fatalf("expected lease release metric sample, got %#v", releaseSample)
	}

	recoverSpan := mustFindTraceSpan(t, traceExporter.spans, "session.recover")
	if recoverSpan.SessionID != recoverable.SessionID {
		t.Fatalf("expected recovery trace span, got %#v", recoverSpan)
	}
	abortSpan := mustFindTraceSpan(t, traceExporter.spans, "session.abort")
	if abortSpan.SessionID != abortable.SessionID {
		t.Fatalf("expected abort trace span, got %#v", abortSpan)
	}
	claimSpan := mustFindTraceSpan(t, traceExporter.spans, "lease.claim")
	if claimSpan.LeaseID != claimed.LeaseID || claimSpan.SessionID != claimed.SessionID {
		t.Fatalf("expected lease claim trace span, got %#v", claimSpan)
	}
	renewSpan := mustFindTraceSpan(t, traceExporter.spans, "lease.renew")
	if renewSpan.LeaseID != claimed.LeaseID {
		t.Fatalf("expected lease renew trace span, got %#v", renewSpan)
	}
	releaseSpan := mustFindTraceSpan(t, traceExporter.spans, "lease.release")
	if releaseSpan.LeaseID != claimed.LeaseID {
		t.Fatalf("expected lease release trace span, got %#v", releaseSpan)
	}

	snap := rt.MetricsSnapshot()
	if snap.RecoveryRuns != 1 || snap.SessionAborts != 1 || snap.LeaseClaims != 1 || snap.LeaseRenewals != 1 || snap.LeaseReleases != 1 {
		t.Fatalf("expected lifecycle counters to increment, got %#v", snap)
	}
}

func mustFindMetricSample(t *testing.T, samples []observability.MetricSample, name string) observability.MetricSample {
	t.Helper()
	for _, sample := range samples {
		if sample.Name == name {
			return sample
		}
	}
	t.Fatalf("expected metric sample %q, got %#v", name, samples)
	return observability.MetricSample{}
}

func mustFindTraceSpan(t *testing.T, spans []observability.TraceSpan, name string) observability.TraceSpan {
	t.Helper()
	for _, span := range spans {
		if span.Name == name {
			return span
		}
	}
	t.Fatalf("expected trace span %q, got %#v", name, spans)
	return observability.TraceSpan{}
}
