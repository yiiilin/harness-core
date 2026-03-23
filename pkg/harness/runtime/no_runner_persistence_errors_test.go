package runtime_test

import (
	"context"
	"errors"
	"testing"

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

type failingPlanUpdateStore struct {
	plan.Store
	updateErr error
}

func (s failingPlanUpdateStore) Update(plan.Spec) error {
	return s.updateErr
}

type nthFailingTaskUpdateStore struct {
	task.Store
	updateErr       error
	failOnUpdateCall int
	updateCalls      int
}

func (s *nthFailingTaskUpdateStore) Update(next task.Record) error {
	s.updateCalls++
	if s.failOnUpdateCall > 0 && s.updateCalls == s.failOnUpdateCall {
		return s.updateErr
	}
	return s.Store.Update(next)
}

func TestNoRunnerRunStepAskSurfacesPlanUpdateErrors(t *testing.T) {
	boom := errors.New("boom:plan.update")
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := failingPlanUpdateStore{Store: plan.NewMemoryStore(), updateErr: boom}
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	audits := audit.NewMemoryStore()

	rt := hruntime.New(hruntime.Options{
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Tools:     tools,
		Verifiers: verifiers,
		Audit:     audits,
	}).WithPolicyEvaluator(askPolicy{})
	rt.Runner = nil

	sess := mustCreateSession(t, rt, "ask plan update failure", "surface blocked plan update failures")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "ask path should surface plan update failures"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(attached.SessionID, "ask", []plan.StepSpec{{
		StepID: "step_ask_plan_update_fail",
		Title:  "ask plan update fail",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo ask", "timeout_ms": 5000}},
		Verify: verify.Spec{},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	if _, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0]); !errors.Is(err, boom) {
		t.Fatalf("expected ask path to surface plan update error, got %v", err)
	}
}

func TestNoRunnerRunStepDenySurfacesTaskUpdateErrors(t *testing.T) {
	boom := errors.New("boom:task.update")
	sessions := session.NewMemoryStore()
	taskStore := &nthFailingTaskUpdateStore{Store: task.NewMemoryStore(), updateErr: boom, failOnUpdateCall: 2}
	plans := plan.NewMemoryStore()
	audits := audit.NewMemoryStore()

	rt := hruntime.New(hruntime.Options{
		Sessions: sessions,
		Tasks:    taskStore,
		Plans:    plans,
		Audit:    audits,
	}).WithPolicyEvaluator(denyAllPolicy{})
	rt.Runner = nil

	sess := mustCreateSession(t, rt, "deny task update failure", "surface deny-path task update failures")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "deny path should surface task update failures"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(attached.SessionID, "deny", []plan.StepSpec{{
		StepID: "step_deny_task_update_fail",
		Title:  "deny task update fail",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo deny", "timeout_ms": 5000}},
		Verify: verify.Spec{},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	if _, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0]); !errors.Is(err, boom) {
		t.Fatalf("expected deny path to surface task update error, got %v", err)
	}
}

func TestNoRunnerRunStepCompleteSurfacesTaskUpdateErrors(t *testing.T) {
	boom := errors.New("boom:task.update")
	sessions := session.NewMemoryStore()
	taskStore := &nthFailingTaskUpdateStore{Store: task.NewMemoryStore(), updateErr: boom, failOnUpdateCall: 2}
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

	rt := hruntime.New(hruntime.Options{
		Sessions:  sessions,
		Tasks:     taskStore,
		Plans:     plans,
		Tools:     tools,
		Verifiers: verifiers,
		Audit:     audits,
	})
	rt.Runner = nil

	sess := mustCreateSession(t, rt, "complete task update failure", "surface complete-path task update failures")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "complete path should surface task update failures"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(attached.SessionID, "complete", []plan.StepSpec{{
		StepID: "step_complete_task_update_fail",
		Title:  "complete task update fail",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo complete", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
		}},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	if _, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0]); !errors.Is(err, boom) {
		t.Fatalf("expected complete path to surface task update error, got %v", err)
	}
	if handler.calls != 1 {
		t.Fatalf("expected action to execute before task update failure, got %d calls", handler.calls)
	}
}

func TestNoRunnerRespondApprovalRejectSurfacesPlanUpdateErrors(t *testing.T) {
	boom := errors.New("boom:task.update")
	sessions := session.NewMemoryStore()
	taskStore := &nthFailingTaskUpdateStore{Store: task.NewMemoryStore(), updateErr: boom, failOnUpdateCall: 2}
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	audits := audit.NewMemoryStore()

	rt := hruntime.New(hruntime.Options{
		Sessions:  sessions,
		Tasks:     taskStore,
		Plans:     plans,
		Tools:     tools,
		Verifiers: verifiers,
		Audit:     audits,
	}).WithPolicyEvaluator(askPolicy{})
	rt.Runner = nil

	sess := mustCreateSession(t, rt, "reject plan update failure", "surface approval reject plan update failures")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "reject path should surface plan update failures"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(attached.SessionID, "reject", []plan.StepSpec{{
		StepID: "step_reject_plan_update_fail",
		Title:  "reject plan update fail",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo reject", "timeout_ms": 5000}},
		Verify: verify.Spec{},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	initial, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	if initial.Execution.PendingApproval == nil {
		t.Fatalf("expected pending approval before reject path")
	}

	if _, _, err := rt.RespondApproval(initial.Execution.PendingApproval.ApprovalID, approval.Response{Reply: approval.ReplyReject}); !errors.Is(err, boom) {
		t.Fatalf("expected reject path to surface task update error, got %v", err)
	}
}
