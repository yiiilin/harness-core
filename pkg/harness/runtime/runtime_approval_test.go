package runtime_test

import (
	"context"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type askPolicy struct{}

func (askPolicy) Evaluate(_ context.Context, _ session.State, _ plan.StepSpec) (permission.Decision, error) {
	return permission.Decision{Action: permission.Ask, Reason: "approval required", MatchedRule: "test/ask"}, nil
}

type countingHandler struct {
	calls int
}

func (h *countingHandler) Invoke(_ context.Context, _ map[string]any) (action.Result, error) {
	h.calls++
	return action.Result{
		OK: true,
		Data: map[string]any{
			"status":    "completed",
			"exit_code": 0,
			"stdout":    "executed",
		},
	}, nil
}

func TestRunStepPolicyAskCreatesApprovalAndDoesNotExecute(t *testing.T) {
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

	rt := hruntime.New(hruntime.Options{
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Tools:     tools,
		Verifiers: verifiers,
		Audit:     audits,
	}).WithPolicyEvaluator(askPolicy{})

	sess := rt.CreateSession("ask session", "approval required path")
	tsk := rt.CreateTask(task.Spec{TaskType: "demo", Goal: "wait for approval before executing"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(attached.SessionID, "ask", []plan.StepSpec{{
		StepID: "step_ask",
		Title:  "approval required shell action",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo hello", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
		}},
		OnFail: plan.OnFailSpec{Strategy: "abort"},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	out, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}

	if out.Execution.Policy.Decision.Action != permission.Ask {
		t.Fatalf("expected ask decision, got %#v", out.Execution.Policy.Decision)
	}
	if handler.calls != 0 {
		t.Fatalf("expected action not to execute before approval, got %d calls", handler.calls)
	}
	if out.Session.Phase != session.PhaseExecute {
		t.Fatalf("expected session to stay blocked before execution, got %s", out.Session.Phase)
	}
	if out.UpdatedTask != nil && out.UpdatedTask.Status != task.StatusRunning {
		t.Fatalf("expected task to stay running while approval is pending, got %#v", out.UpdatedTask)
	}

	foundApprovalRequested := false
	foundToolCall := false
	for _, event := range rt.ListAuditEvents(attached.SessionID) {
		if event.Type == "approval.requested" {
			foundApprovalRequested = true
		}
		if event.Type == audit.EventToolCalled {
			foundToolCall = true
		}
	}
	if !foundApprovalRequested {
		t.Fatalf("expected approval.requested event in audit trail")
	}
	if foundToolCall {
		t.Fatalf("did not expect tool.called event before approval")
	}
}

func TestResumePendingApprovalExecutesStepAfterReplyOnce(t *testing.T) {
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

	rt := hruntime.New(hruntime.Options{
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Tools:     tools,
		Verifiers: verifiers,
		Audit:     audits,
	}).WithPolicyEvaluator(askPolicy{})

	sess := rt.CreateSession("resume once", "approval then resume")
	tsk := rt.CreateTask(task.Spec{TaskType: "demo", Goal: "resume after one-time approval"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(sess.SessionID, "approval", []plan.StepSpec{{
		StepID: "step_resume_once",
		Title:  "resume once",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo approved", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
		}},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	initial, err := rt.RunStep(context.Background(), sess.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	if initial.Execution.PendingApproval == nil {
		t.Fatalf("expected pending approval in execution result")
	}

	approvalRec, stateAfterReply, err := rt.RespondApproval(initial.Execution.PendingApproval.ApprovalID, approval.Response{Reply: approval.ReplyOnce})
	if err != nil {
		t.Fatalf("respond approval: %v", err)
	}
	if approvalRec.Status != approval.StatusApproved {
		t.Fatalf("expected approved approval record, got %#v", approvalRec)
	}
	if stateAfterReply.PendingApprovalID == "" {
		t.Fatalf("expected session to keep approval handle until resume")
	}

	resumed, err := rt.ResumePendingApproval(context.Background(), sess.SessionID)
	if err != nil {
		t.Fatalf("resume pending approval: %v", err)
	}
	if handler.calls != 1 {
		t.Fatalf("expected one tool execution after resume, got %d", handler.calls)
	}
	if resumed.Session.PendingApprovalID != "" {
		t.Fatalf("expected pending approval to clear after resume, got %#v", resumed.Session)
	}
	if resumed.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected session complete after resumed execution, got %s", resumed.Session.Phase)
	}

	storedApproval, err := rt.GetApproval(approvalRec.ApprovalID)
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	if storedApproval.Status != approval.StatusConsumed {
		t.Fatalf("expected one-time approval to be consumed, got %#v", storedApproval)
	}
}

func TestRespondApprovalRejectFailsPendingStepWithoutExecuting(t *testing.T) {
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

	rt := hruntime.New(hruntime.Options{
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Tools:     tools,
		Verifiers: verifiers,
		Audit:     audits,
	}).WithPolicyEvaluator(askPolicy{})

	sess := rt.CreateSession("reject approval", "approval rejected path")
	tsk := rt.CreateTask(task.Spec{TaskType: "demo", Goal: "reject approval and fail safely"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(sess.SessionID, "approval reject", []plan.StepSpec{{
		StepID: "step_reject",
		Title:  "reject pending action",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo should-not-run", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	initial, err := rt.RunStep(context.Background(), sess.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	if initial.Execution.PendingApproval == nil {
		t.Fatalf("expected pending approval in execution result")
	}

	approvalRec, stateAfterReply, err := rt.RespondApproval(initial.Execution.PendingApproval.ApprovalID, approval.Response{Reply: approval.ReplyReject})
	if err != nil {
		t.Fatalf("respond approval: %v", err)
	}
	if approvalRec.Status != approval.StatusRejected {
		t.Fatalf("expected rejected approval record, got %#v", approvalRec)
	}
	if handler.calls != 0 {
		t.Fatalf("expected reject path not to execute tool, got %d calls", handler.calls)
	}
	if stateAfterReply.PendingApprovalID != "" {
		t.Fatalf("expected pending approval to clear after reject, got %#v", stateAfterReply)
	}
	if stateAfterReply.Phase != session.PhaseFailed {
		t.Fatalf("expected session failed after reject, got %s", stateAfterReply.Phase)
	}
}

func TestReplyAlwaysAllowsFutureMatchingToolWithoutAnotherApproval(t *testing.T) {
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

	rt := hruntime.New(hruntime.Options{
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Tools:     tools,
		Verifiers: verifiers,
		Audit:     audits,
	}).WithPolicyEvaluator(askPolicy{})

	sess := rt.CreateSession("always approval", "reuse approval for matching tool")
	tsk := rt.CreateTask(task.Spec{TaskType: "demo", Goal: "reuse approval"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(sess.SessionID, "always approval", []plan.StepSpec{
		{
			StepID: "step_always_1",
			Title:  "first approval-required shell action",
			Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo first", "timeout_ms": 5000}},
			Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
				{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
			}},
		},
		{
			StepID: "step_always_2",
			Title:  "second shell action should reuse approval",
			Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo second", "timeout_ms": 5000}},
			Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
				{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
			}},
		},
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	first, err := rt.RunStep(context.Background(), sess.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run first step: %v", err)
	}
	if first.Execution.PendingApproval == nil {
		t.Fatalf("expected pending approval for first step")
	}

	approvalRec, _, err := rt.RespondApproval(first.Execution.PendingApproval.ApprovalID, approval.Response{Reply: approval.ReplyAlways})
	if err != nil {
		t.Fatalf("respond approval: %v", err)
	}
	if _, err := rt.ResumePendingApproval(context.Background(), sess.SessionID); err != nil {
		t.Fatalf("resume pending approval: %v", err)
	}
	if handler.calls != 1 {
		t.Fatalf("expected first resumed execution to call tool once, got %d", handler.calls)
	}

	second, err := rt.RunStep(context.Background(), sess.SessionID, pl.Steps[1])
	if err != nil {
		t.Fatalf("run second step: %v", err)
	}
	if second.Execution.Policy.Decision.Action != permission.Allow {
		t.Fatalf("expected second step to auto-allow after reply always, got %#v", second.Execution.Policy.Decision)
	}
	if second.Execution.PendingApproval != nil {
		t.Fatalf("did not expect a new pending approval after reply always")
	}
	if handler.calls != 2 {
		t.Fatalf("expected second step to execute immediately, got %d calls", handler.calls)
	}

	storedApproval, err := rt.GetApproval(approvalRec.ApprovalID)
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	if storedApproval.Status != approval.StatusApproved {
		t.Fatalf("expected always approval to remain approved for reuse, got %#v", storedApproval)
	}
}
