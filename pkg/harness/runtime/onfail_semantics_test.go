package runtime_test

import (
	"context"
	"testing"
	"time"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

func TestRunStepReinspectStrategyReturnsPreparePhaseOnVerificationFailure(t *testing.T) {
	rt, handler := newVerificationFailureRuntime()

	sess := mustCreateSession(t, rt, "reinspect", "reinspect after failed verification")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "reinspect after failed verification"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(attached.SessionID, "reinspect", []plan.StepSpec{{
		StepID: "step_reinspect",
		Title:  "reinspect failure",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo reinspect", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{1}}},
		}},
		OnFail: plan.OnFailSpec{Strategy: "reinspect"},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	out, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	if handler.calls != 1 {
		t.Fatalf("expected exactly one tool call, got %d", handler.calls)
	}
	if out.Session.Phase != session.PhasePrepare {
		t.Fatalf("expected reinspect strategy to re-enter prepare phase, got %#v", out.Session)
	}
}

func TestRunStepRetryBackoffPersistsRetryNotBeforeAndBlocksImmediateRetry(t *testing.T) {
	rt, handler := newVerificationFailureRuntime()

	sess := mustCreateSession(t, rt, "retry backoff", "persist retry backoff")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "persist retry backoff"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(attached.SessionID, "retry backoff", []plan.StepSpec{{
		StepID: "step_retry_backoff",
		Title:  "retry with backoff",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo retry", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{1}}},
		}},
		OnFail: plan.OnFailSpec{Strategy: "retry", BackoffMS: 60000},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	startedAt := time.Now().UnixMilli()
	first, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run first step attempt: %v", err)
	}
	if handler.calls != 1 {
		t.Fatalf("expected one tool call after first attempt, got %d", handler.calls)
	}
	if first.Session.Phase != session.PhaseRecover {
		t.Fatalf("expected retry strategy to stay recoverable, got %#v", first.Session)
	}
	retryNotBefore, ok := metadataInt64(first.Execution.Step.Metadata, "retry_not_before")
	if !ok {
		t.Fatalf("expected retry_not_before metadata after backoff failure, got %#v", first.Execution.Step.Metadata)
	}
	if retryNotBefore < startedAt+50000 {
		t.Fatalf("expected retry_not_before to reflect configured backoff, got %d started %d", retryNotBefore, startedAt)
	}

	if _, err := rt.RunStep(context.Background(), attached.SessionID, first.Execution.Step); err == nil {
		t.Fatalf("expected immediate retry during backoff to fail")
	}
	if handler.calls != 1 {
		t.Fatalf("expected backoff to block a second tool call, got %d", handler.calls)
	}
	if attempts := mustListAttempts(t, rt, attached.SessionID); len(attempts) != 1 {
		t.Fatalf("expected immediate backoff rejection not to create a new attempt, got %#v", attempts)
	}
}

func TestRunSessionStopsWhenRetryBackoffIsActive(t *testing.T) {
	rt, handler := newVerificationFailureRuntime()

	sess := mustCreateSession(t, rt, "driver backoff", "session driver should stop when retry backoff is active")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "session driver should stop when retry backoff is active"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	if _, err := rt.CreatePlan(attached.SessionID, "driver backoff", []plan.StepSpec{{
		StepID: "step_driver_backoff",
		Title:  "driver retry with backoff",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo driver", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{1}}},
		}},
		OnFail: plan.OnFailSpec{Strategy: "retry", BackoffMS: 60000},
	}}); err != nil {
		t.Fatalf("create plan: %v", err)
	}

	out, err := rt.RunSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("run session: %v", err)
	}
	if handler.calls != 1 {
		t.Fatalf("expected session driver to stop after one failed execution under backoff, got %d", handler.calls)
	}
	if len(out.Executions) != 1 {
		t.Fatalf("expected one execution before backoff stop, got %#v", out.Executions)
	}
	if out.Session.Phase != session.PhaseRecover {
		t.Fatalf("expected session to remain recoverable under backoff, got %#v", out.Session)
	}
}

func newVerificationFailureRuntime() (*hruntime.Service, *countingHandler) {
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
		verify.Definition{Kind: "exit_code", Description: "Verify exit code."},
		verify.ExitCodeChecker{},
	)

	return hruntime.New(hruntime.Options{
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Tools:     tools,
		Verifiers: verifiers,
		Audit:     audits,
	}), handler
}

func metadataInt64(metadata map[string]any, key string) (int64, bool) {
	if len(metadata) == 0 {
		return 0, false
	}
	switch value := metadata[key].(type) {
	case int64:
		return value, true
	case int:
		return int64(value), true
	case float64:
		return int64(value), true
	default:
		return 0, false
	}
}
