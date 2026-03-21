package runtime_test

import (
	"context"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

func TestExecutionCycleReadSurfaceGroupsExecutionFacts(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.handle", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, runtimeHandleHandler{})

	rt := hruntime.New(hruntime.Options{Tools: tools}).WithPolicyEvaluator(askPolicy{})

	sess := mustCreateSession(t, rt, "execution cycle reads", "group execution facts by logical cycle")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "read one logical execution cycle"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(attached.SessionID, "execution cycle reads", []plan.StepSpec{{
		StepID: "step_execution_cycle",
		Title:  "approval gated runtime handle step",
		Action: action.Spec{ToolName: "demo.handle", Args: map[string]any{"mode": "interactive"}},
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
		t.Fatalf("expected pending approval")
	}
	if _, _, err := rt.RespondApproval(initial.Execution.PendingApproval.ApprovalID, approval.Response{Reply: approval.ReplyOnce}); err != nil {
		t.Fatalf("respond approval: %v", err)
	}
	if _, err := rt.ResumePendingApproval(context.Background(), attached.SessionID); err != nil {
		t.Fatalf("resume approval: %v", err)
	}

	attempts := mustListAttempts(t, rt, attached.SessionID)
	if len(attempts) != 1 || attempts[0].CycleID == "" {
		t.Fatalf("expected one execution attempt with cycle_id, got %#v", attempts)
	}
	cycleID := attempts[0].CycleID

	cycle, err := rt.GetExecutionCycle(attached.SessionID, cycleID)
	if err != nil {
		t.Fatalf("get execution cycle: %v", err)
	}
	if cycle.CycleID != cycleID || cycle.SessionID != attached.SessionID || cycle.TaskID != attached.TaskID {
		t.Fatalf("expected execution cycle identity to be preserved, got %#v", cycle)
	}
	if len(cycle.Attempts) != 1 || len(cycle.Actions) != 1 || len(cycle.Verifications) != 1 {
		t.Fatalf("expected cycle to group one attempt, one action, and one verification, got %#v", cycle)
	}
	if len(cycle.Artifacts) == 0 || len(cycle.RuntimeHandles) != 1 {
		t.Fatalf("expected cycle to expose artifacts and runtime handles, got %#v", cycle)
	}
	if cycle.RuntimeHandles[0].CycleID != cycleID {
		t.Fatalf("expected runtime handle cycle_id to match execution cycle %q, got %#v", cycleID, cycle.RuntimeHandles)
	}

	cycles, err := rt.ListExecutionCycles(attached.SessionID)
	if err != nil {
		t.Fatalf("list execution cycles: %v", err)
	}
	if len(cycles) != 1 {
		t.Fatalf("expected one execution cycle bundle, got %#v", cycles)
	}
	if cycles[0].CycleID != cycleID || len(cycles[0].RuntimeHandles) != 1 {
		t.Fatalf("expected listed execution cycle to retain runtime handles, got %#v", cycles[0])
	}
}
