package runtime_test

import (
	"context"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/builtins"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

func TestMetricsSnapshotForHappyPath(t *testing.T) {
	opts := hruntime.Options{}
	builtins.Register(&opts)
	rt := hruntime.New(opts)

	sess := mustCreateSession(t, rt, "metrics", "happy path metrics")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "metrics path"})
	sess, _ = rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	pl, _ := rt.CreatePlan(sess.SessionID, "initial", []plan.StepSpec{{
		StepID: "step_1",
		Title:  "echo hello",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo hello", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}}, {Kind: "output_contains", Args: map[string]any{"text": "hello"}}}},
	}})
	_, err := rt.RunStep(context.Background(), sess.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	snap := rt.MetricsSnapshot()
	if snap.StepRuns < 1 {
		t.Fatalf("expected at least one step run, got %#v", snap)
	}
	if snap.StepSuccess < 1 {
		t.Fatalf("expected at least one successful step, got %#v", snap)
	}
}
