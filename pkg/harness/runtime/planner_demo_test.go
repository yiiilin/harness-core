package runtime_test

import (
	"context"
	"testing"

	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

func TestDemoPlannerDerivesShellStep(t *testing.T) {
	planner := hruntime.DemoPlanner{}
	state := session.State{SessionID: "s1", Phase: session.PhasePlan}
	spec := task.Spec{TaskID: "t1", TaskType: "demo", Goal: "echo hello"}
	assembled := hruntime.ContextPackage{Task: hruntime.ContextTask{Goal: "echo hello"}}
	step, err := planner.PlanNext(context.Background(), state, spec, assembled)
	if err != nil {
		t.Fatalf("plan next: %v", err)
	}
	if step.Action.ToolName != "shell.exec" {
		t.Fatalf("expected shell.exec, got %s", step.Action.ToolName)
	}
	if step.Verify.Mode == "" {
		t.Fatalf("expected verify mode to be set")
	}
}
