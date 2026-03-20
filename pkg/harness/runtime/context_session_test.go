package runtime_test

import (
	"context"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/action"
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
	hruntime.RegisterBuiltins(&opts)
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
