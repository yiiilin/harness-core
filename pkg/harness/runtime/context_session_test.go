package runtime_test

import (
	"context"
	"errors"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/builtins"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

func TestRunSessionPersistsExecutionCompactionSummaryOutsidePlannerPath(t *testing.T) {
	compactor := &lifecycleCompactor{}
	summaries := hruntime.NewMemoryContextSummaryStore()
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.echo", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, &countingHandler{})

	rt := hruntime.New(hruntime.Options{
		Tools:            tools,
		Compactor:        compactor,
		ContextSummaries: summaries,
	})

	sess := mustCreateSession(t, rt, "execution compaction", "persist execution-phase summaries")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "run session execution compaction"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	if _, err := rt.CreatePlan(attached.SessionID, "manual plan", []plan.StepSpec{{
		StepID: "step_exec_compaction",
		Title:  "execute with compaction",
		Action: action.Spec{ToolName: "demo.echo", Args: map[string]any{"message": "hello"}},
		Verify: verify.Spec{},
	}}); err != nil {
		t.Fatalf("create plan: %v", err)
	}

	out, err := rt.RunSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("run session: %v", err)
	}
	if out.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected session to complete, got %#v", out.Session)
	}

	items, err := summaries.List(attached.SessionID)
	if err != nil {
		t.Fatalf("list summaries: %v", err)
	}
	foundExecution := false
	for _, item := range items {
		if item.Trigger == hruntime.CompactionTriggerExecute {
			foundExecution = true
			break
		}
	}
	if !foundExecution {
		t.Fatalf("expected execution-phase summary outside planner path, got %#v", items)
	}
}

func TestRecoverSessionPersistsRecoveryCompactionSummary(t *testing.T) {
	compactor := &lifecycleCompactor{}
	summaries := hruntime.NewMemoryContextSummaryStore()
	opts := hruntime.Options{
		Compactor:        compactor,
		ContextSummaries: summaries,
	}
	builtins.Register(&opts)
	rt := hruntime.New(opts)

	sess := mustCreateSession(t, rt, "recovery compaction", "persist recovery-phase summaries")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "recover and compact context"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	if _, _, err := rt.CompactSessionContext(context.Background(), attached.SessionID, hruntime.CompactionTriggerExecute); err != nil {
		t.Fatalf("seed execution compaction summary: %v", err)
	}
	if _, err := rt.CreatePlan(attached.SessionID, "recover plan", []plan.StepSpec{{
		StepID: "step_recover_compaction",
		Title:  "recover with compaction",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo recovered", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
		}},
	}}); err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := rt.MarkSessionInterrupted(context.Background(), attached.SessionID); err != nil {
		t.Fatalf("mark interrupted: %v", err)
	}

	out, err := rt.RecoverSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("recover session: %v", err)
	}
	if out.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected recovered session to complete, got %#v", out.Session)
	}

	items, err := summaries.List(attached.SessionID)
	if err != nil {
		t.Fatalf("list summaries: %v", err)
	}
	var latest hruntime.ContextSummary
	foundRecovery := false
	for _, item := range items {
		if item.Trigger == hruntime.CompactionTriggerRecover {
			foundRecovery = true
			latest = item
		}
	}
	if !foundRecovery {
		t.Fatalf("expected recovery-phase summary, got %#v", items)
	}
	if latest.SupersedesSummaryID == "" {
		t.Fatalf("expected recovery summary to supersede prior summary, got %#v", latest)
	}
}

type phaseFailingCompactor struct {
	failPhase session.Phase
}

func (c phaseFailingCompactor) Compact(_ context.Context, pkg hruntime.ContextPackage, state session.State, _ task.Spec, _ hruntime.LoopBudgets) (hruntime.ContextPackage, *hruntime.ContextSummary, error) {
	if state.Phase == c.failPhase {
		return hruntime.ContextPackage{}, nil, errors.New("boom:compaction")
	}
	return pkg, &hruntime.ContextSummary{
		SessionID:      state.SessionID,
		TaskID:         pkg.Task.TaskID,
		Strategy:       "noop",
		OriginalBytes:  1,
		CompactedBytes: 1,
		Summary:        map[string]any{"phase": state.Phase},
	}, nil
}

func TestRunSessionPersistsExecuteCompactionAfterApprovalResume(t *testing.T) {
	compactor := &lifecycleCompactor{}
	summaries := hruntime.NewMemoryContextSummaryStore()
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.echo", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, &countingHandler{})

	rt := hruntime.New(hruntime.Options{
		Tools:            tools,
		Compactor:        compactor,
		ContextSummaries: summaries,
	}).WithPolicyEvaluator(askPolicy{})

	sess := mustCreateSession(t, rt, "approval compaction", "persist execute compaction after approval resume")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "resume a blocked step and compact execute context"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	if _, err := rt.CreatePlan(attached.SessionID, "manual plan", []plan.StepSpec{{
		StepID: "step_exec_after_approval",
		Title:  "execute after approval",
		Action: action.Spec{ToolName: "demo.echo", Args: map[string]any{"message": "hello"}},
		Verify: verify.Spec{},
	}}); err != nil {
		t.Fatalf("create plan: %v", err)
	}

	blocked, err := rt.RunSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("run blocked session: %v", err)
	}
	if len(blocked.Executions) != 1 || blocked.Executions[0].Execution.PendingApproval == nil {
		t.Fatalf("expected blocked approval execution, got %#v", blocked.Executions)
	}

	if _, _, err := rt.RespondApproval(blocked.Executions[0].Execution.PendingApproval.ApprovalID, approval.Response{Reply: approval.ReplyOnce}); err != nil {
		t.Fatalf("respond approval: %v", err)
	}

	resumed, err := rt.RunSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("resume session: %v", err)
	}
	if resumed.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected resumed session to complete, got %#v", resumed.Session)
	}

	items, err := summaries.List(attached.SessionID)
	if err != nil {
		t.Fatalf("list summaries: %v", err)
	}
	foundExecution := false
	for _, item := range items {
		if item.Trigger == hruntime.CompactionTriggerExecute {
			foundExecution = true
			break
		}
	}
	if !foundExecution {
		t.Fatalf("expected execute compaction summary after approval resume, got %#v", items)
	}
}

func TestRunSessionTreatsExecuteCompactionAsBestEffort(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.echo", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, &countingHandler{})

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Compactor: phaseFailingCompactor{failPhase: session.PhaseComplete},
	})

	sess := mustCreateSession(t, rt, "execute compaction best effort", "do not fail after a committed execution")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "keep run session successful after compaction failure"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	if _, err := rt.CreatePlan(attached.SessionID, "manual plan", []plan.StepSpec{{
		StepID: "step_best_effort_execute_compaction",
		Title:  "complete despite compaction error",
		Action: action.Spec{ToolName: "demo.echo", Args: map[string]any{"message": "hello"}},
		Verify: verify.Spec{},
	}}); err != nil {
		t.Fatalf("create plan: %v", err)
	}

	out, err := rt.RunSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("expected committed execution to stay successful, got %v", err)
	}
	if out.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected completed session, got %#v", out.Session)
	}
	if len(out.Executions) != 1 {
		t.Fatalf("expected one execution result, got %#v", out.Executions)
	}
	stored, err := rt.GetSession(attached.SessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if stored.Phase != session.PhaseComplete {
		t.Fatalf("expected stored session to remain complete, got %#v", stored)
	}
}

func TestRecoverSessionTreatsRecoveryCompactionAsBestEffort(t *testing.T) {
	opts := hruntime.Options{
		Compactor: phaseFailingCompactor{failPhase: session.PhaseRecover},
	}
	builtins.Register(&opts)
	rt := hruntime.New(opts)

	sess := mustCreateSession(t, rt, "recover compaction best effort", "do not fail recovery after compaction error")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "keep recover session successful after compaction failure"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	if _, err := rt.CreatePlan(attached.SessionID, "recover plan", []plan.StepSpec{{
		StepID: "step_best_effort_recover_compaction",
		Title:  "recover despite compaction error",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo recovered", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
		}},
	}}); err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := rt.MarkSessionInterrupted(context.Background(), attached.SessionID); err != nil {
		t.Fatalf("mark interrupted: %v", err)
	}

	out, err := rt.RecoverSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("expected committed recovery to stay successful, got %v", err)
	}
	if out.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected recovered session to complete, got %#v", out.Session)
	}
	if len(out.Executions) != 1 {
		t.Fatalf("expected one recovered execution, got %#v", out.Executions)
	}
	stored, err := rt.GetSession(attached.SessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if stored.Phase != session.PhaseComplete {
		t.Fatalf("expected stored session to remain complete after recovery, got %#v", stored)
	}
}
