package runtime_test

import (
	"context"
	"errors"
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

type scopedAskPolicy struct{}

func (scopedAskPolicy) Evaluate(_ context.Context, _ session.State, step plan.StepSpec) (permission.Decision, error) {
	path, _ := step.Action.Args["path"].(string)
	return permission.Decision{
		Action:      permission.Ask,
		Reason:      "approval required",
		MatchedRule: "test/ask:" + path,
	}, nil
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

	sess := mustCreateSession(t, rt, "ask session", "approval required path")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "wait for approval before executing"})
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
	for _, event := range mustListAuditEvents(t, rt, attached.SessionID) {
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

	sess := mustCreateSession(t, rt, "resume once", "approval then resume")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "resume after one-time approval"})
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
	attempts := mustListAttempts(t, rt, sess.SessionID)
	if len(attempts) != 1 || attempts[0].Status != "blocked" {
		t.Fatalf("expected one blocked attempt before approval resume, got %#v", attempts)
	}
	if attempts[0].FinishedAt != 0 {
		t.Fatalf("expected blocked attempt to stay open until resume, got %#v", attempts[0])
	}
	blockedAttemptID := attempts[0].AttemptID

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
	attempts = mustListAttempts(t, rt, sess.SessionID)
	if len(attempts) != 1 || attempts[0].AttemptID != blockedAttemptID || attempts[0].Status != "blocked" {
		t.Fatalf("expected approval response to keep the original blocked attempt pending, got %#v", attempts)
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
	attempts = mustListAttempts(t, rt, sess.SessionID)
	if len(attempts) != 1 {
		t.Fatalf("expected resume to reuse the original attempt, got %#v", attempts)
	}
	if attempts[0].AttemptID != blockedAttemptID || attempts[0].Status != "completed" || attempts[0].Step.Status != plan.StepCompleted {
		t.Fatalf("expected original blocked attempt to become the completed execution attempt, got %#v", attempts[0])
	}

	storedApproval, err := rt.GetApproval(approvalRec.ApprovalID)
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	if storedApproval.Status != approval.StatusConsumed {
		t.Fatalf("expected one-time approval to be consumed, got %#v", storedApproval)
	}
}

func TestRespondApprovalBestEffortEventEmissionWithoutRunner(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	audits := audit.NewMemoryStore()
	handler := &countingHandler{}

	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		handler,
	)

	rt := hruntime.New(hruntime.Options{
		Sessions: sessions,
		Tasks:    tasks,
		Plans:    plans,
		Tools:    tools,
		Audit:    audits,
		EventSink: selectiveFailingEventSink{failures: map[string]error{
			audit.EventApprovalApproved: errors.New("boom:approval.approved"),
		}},
	}).WithPolicyEvaluator(askPolicy{})
	rt.Runner = nil

	sess := mustCreateSession(t, rt, "best effort approval", "approval responses should stay successful without runner")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "approve without transactional sink"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(sess.SessionID, "approval", []plan.StepSpec{{
		StepID: "step_best_effort_approval",
		Title:  "best effort approval",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo approved", "timeout_ms": 5000}},
		Verify: verify.Spec{},
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

	rec, stateAfterReply, err := rt.RespondApproval(initial.Execution.PendingApproval.ApprovalID, approval.Response{Reply: approval.ReplyOnce})
	if err != nil {
		t.Fatalf("expected approval response to stay successful without runner compensation, got %v", err)
	}
	if rec.Status != approval.StatusApproved {
		t.Fatalf("expected approved record, got %#v", rec)
	}
	if stateAfterReply.PendingApprovalID == "" {
		t.Fatalf("expected session to retain pending approval until resume, got %#v", stateAfterReply)
	}

	storedApproval, err := rt.GetApproval(rec.ApprovalID)
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	if storedApproval.Status != approval.StatusApproved {
		t.Fatalf("expected stored approval to remain approved, got %#v", storedApproval)
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

	sess := mustCreateSession(t, rt, "reject approval", "approval rejected path")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "reject approval and fail safely"})
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

	sess := mustCreateSession(t, rt, "always approval", "reuse approval for matching tool")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "reuse approval"})
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
			Title:  "second identical shell action should reuse approval",
			Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo first", "timeout_ms": 5000}},
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

func TestReplyAlwaysDoesNotReuseApprovalAcrossDifferentArgsOrVersion(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	audits := audit.NewMemoryStore()
	handlerV1 := &countingHandler{}
	handlerV2 := &countingHandler{}

	tools.Register(
		tool.Definition{ToolName: "fs.write", Version: "v1", CapabilityType: "filesystem", RiskLevel: tool.RiskMedium, Enabled: true},
		handlerV1,
	)
	tools.Register(
		tool.Definition{ToolName: "fs.write", Version: "v2", CapabilityType: "filesystem", RiskLevel: tool.RiskMedium, Enabled: true},
		handlerV2,
	)

	rt := hruntime.New(hruntime.Options{
		Sessions: sessions,
		Tasks:    tasks,
		Plans:    plans,
		Tools:    tools,
		Audit:    audits,
	}).WithPolicyEvaluator(scopedAskPolicy{})

	sess := mustCreateSession(t, rt, "always scoped", "approval scope must stay narrow")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "avoid overly broad always approvals"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(sess.SessionID, "scoped always", []plan.StepSpec{
		{
			StepID: "step_first",
			Title:  "write alpha with v1",
			Action: action.Spec{ToolName: "fs.write", ToolVersion: "v1", Args: map[string]any{"path": "/tmp/alpha.txt", "content": "alpha"}},
			Verify: verify.Spec{},
		},
		{
			StepID: "step_second",
			Title:  "write beta with v2",
			Action: action.Spec{ToolName: "fs.write", ToolVersion: "v2", Args: map[string]any{"path": "/tmp/beta.txt", "content": "beta"}},
			Verify: verify.Spec{},
		},
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	first, err := rt.RunStep(context.Background(), sess.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step 1: %v", err)
	}
	if first.Execution.PendingApproval == nil {
		t.Fatalf("expected pending approval on first step")
	}
	if _, _, err := rt.RespondApproval(first.Execution.PendingApproval.ApprovalID, approval.Response{Reply: approval.ReplyAlways}); err != nil {
		t.Fatalf("respond approval: %v", err)
	}
	if _, err := rt.ResumePendingApproval(context.Background(), sess.SessionID); err != nil {
		t.Fatalf("resume pending approval: %v", err)
	}
	if handlerV1.calls != 1 {
		t.Fatalf("expected v1 handler to run once after approved resume, got %d", handlerV1.calls)
	}

	second, err := rt.RunStep(context.Background(), sess.SessionID, pl.Steps[1])
	if err != nil {
		t.Fatalf("run step 2: %v", err)
	}
	if second.Execution.PendingApproval == nil {
		t.Fatalf("expected second step to require a fresh approval instead of reusing reply-always approval")
	}
	if handlerV2.calls != 0 {
		t.Fatalf("expected v2 handler not to execute before fresh approval, got %d", handlerV2.calls)
	}
}

func TestRejectApprovalFinalizesBlockedAttempt(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	audits := audit.NewMemoryStore()
	handler := &countingHandler{}

	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		handler,
	)

	rt := hruntime.New(hruntime.Options{
		Sessions: sessions,
		Tasks:    tasks,
		Plans:    plans,
		Tools:    tools,
		Audit:    audits,
	}).WithPolicyEvaluator(askPolicy{})

	sess := mustCreateSession(t, rt, "reject blocked attempt", "blocked attempt should be finalized on reject")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "reject approval and reconcile attempt"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(sess.SessionID, "reject blocked attempt", []plan.StepSpec{{
		StepID: "step_blocked",
		Title:  "blocked",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo blocked", "timeout_ms": 5000}},
		Verify: verify.Spec{},
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
	attempts := mustListAttempts(t, rt, sess.SessionID)
	if len(attempts) != 1 || attempts[0].Status != "blocked" {
		t.Fatalf("expected one blocked attempt after ask path, got %#v", attempts)
	}

	if _, _, err := rt.RespondApproval(initial.Execution.PendingApproval.ApprovalID, approval.Response{Reply: approval.ReplyReject}); err != nil {
		t.Fatalf("respond approval: %v", err)
	}
	attempts = mustListAttempts(t, rt, sess.SessionID)
	if len(attempts) != 1 {
		t.Fatalf("expected one attempt after reject, got %#v", attempts)
	}
	if attempts[0].Status == "blocked" || attempts[0].FinishedAt == 0 {
		t.Fatalf("expected blocked attempt to be finalized after reject, got %#v", attempts[0])
	}
}

func TestRespondApprovalRejectsInvalidReply(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	audits := audit.NewMemoryStore()
	handler := &countingHandler{}

	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		handler,
	)

	rt := hruntime.New(hruntime.Options{
		Sessions: sessions,
		Tasks:    tasks,
		Plans:    plans,
		Tools:    tools,
		Audit:    audits,
	}).WithPolicyEvaluator(askPolicy{})

	sess := mustCreateSession(t, rt, "invalid reply", "reject unknown approval replies")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "approval replies must be validated"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(sess.SessionID, "invalid reply", []plan.StepSpec{{
		StepID: "step_invalid_reply",
		Title:  "pending approval",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo invalid", "timeout_ms": 5000}},
		Verify: verify.Spec{},
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

	approvalID := initial.Execution.PendingApproval.ApprovalID
	if _, _, err := rt.RespondApproval(approvalID, approval.Response{Reply: approval.Reply("bogus")}); !errors.Is(err, approval.ErrInvalidReply) {
		t.Fatalf("expected ErrInvalidReply, got %v", err)
	}

	storedApproval, err := rt.GetApproval(approvalID)
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	if storedApproval.Status != approval.StatusPending || storedApproval.Reply != "" {
		t.Fatalf("expected pending approval to remain unchanged, got %#v", storedApproval)
	}
	if handler.calls != 0 {
		t.Fatalf("did not expect tool execution on invalid reply, got %d calls", handler.calls)
	}
}

func TestRespondApprovalRejectsNonPendingApprovalAndCannotBroadenConsumedApproval(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	audits := audit.NewMemoryStore()
	handler := &countingHandler{}

	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		handler,
	)

	rt := hruntime.New(hruntime.Options{
		Sessions: sessions,
		Tasks:    tasks,
		Plans:    plans,
		Tools:    tools,
		Audit:    audits,
	}).WithPolicyEvaluator(askPolicy{})

	sess := mustCreateSession(t, rt, "non-pending reply", "consumed approvals must stay immutable")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "old approvals must not be broadened"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(sess.SessionID, "consumed approval", []plan.StepSpec{
		{
			StepID: "step_first",
			Title:  "first gated step",
			Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo first", "timeout_ms": 5000}},
			Verify: verify.Spec{},
		},
		{
			StepID: "step_second",
			Title:  "second identical step",
			Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo first", "timeout_ms": 5000}},
			Verify: verify.Spec{},
		},
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	initial, err := rt.RunStep(context.Background(), sess.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run first step: %v", err)
	}
	if initial.Execution.PendingApproval == nil {
		t.Fatalf("expected pending approval")
	}

	approvalID := initial.Execution.PendingApproval.ApprovalID
	if _, _, err := rt.RespondApproval(approvalID, approval.Response{Reply: approval.ReplyOnce}); err != nil {
		t.Fatalf("respond approval once: %v", err)
	}
	if _, err := rt.ResumePendingApproval(context.Background(), sess.SessionID); err != nil {
		t.Fatalf("resume pending approval: %v", err)
	}

	storedApproval, err := rt.GetApproval(approvalID)
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	if storedApproval.Status != approval.StatusConsumed {
		t.Fatalf("expected consumed approval, got %#v", storedApproval)
	}

	if _, _, err := rt.RespondApproval(approvalID, approval.Response{Reply: approval.ReplyAlways}); !errors.Is(err, approval.ErrApprovalNotPending) {
		t.Fatalf("expected ErrApprovalNotPending when re-responding consumed approval, got %v", err)
	}

	second, err := rt.RunStep(context.Background(), sess.SessionID, pl.Steps[1])
	if err != nil {
		t.Fatalf("run second step: %v", err)
	}
	if second.Execution.PendingApproval == nil {
		t.Fatalf("expected second step to require fresh approval")
	}
	if second.Execution.Policy.Decision.Action != permission.Ask {
		t.Fatalf("expected second step to stay on ask path, got %#v", second.Execution.Policy.Decision)
	}
	if handler.calls != 1 {
		t.Fatalf("expected only the resumed first step to execute, got %d calls", handler.calls)
	}
}
