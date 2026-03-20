package runtime_test

import (
	"context"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type failingHandler struct{}

func (failingHandler) Invoke(_ context.Context, _ map[string]any) (action.Result, error) {
	return action.Result{
		OK: false,
		Data: map[string]any{
			"status":    "failed",
			"exit_code": 1,
			"stderr":    "simulated failure",
		},
		Error: &action.Error{Code: "SIMULATED_FAILURE", Message: "simulated failure"},
	}, nil
}

func BenchmarkRunStepActionFailure(b *testing.B) {
	for i := 0; i < b.N; i++ {
		sessions := session.NewMemoryStore()
		tasks := task.NewMemoryStore()
		plans := plan.NewMemoryStore()
		tools := tool.NewRegistry()
		verifiers := verify.NewRegistry()
		audits := audit.NewMemoryStore()

		tools.Register(tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true}, failingHandler{})
		verifiers.Register(verify.Definition{Kind: "exit_code", Description: "Verify exit code."}, verify.ExitCodeChecker{})

		rt := hruntime.New(hruntime.Options{Sessions: sessions, Tasks: tasks, Plans: plans, Tools: tools, Verifiers: verifiers, Audit: audits})
		sess := mustCreateSession(b, rt, "action-fail", "action failure path")
		tsk := mustCreateTask(b, rt, task.Spec{TaskType: "bench", Goal: "action failure path"})
		sess, _ = rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
		step := plan.StepSpec{
			StepID: "step_fail_action",
			Title:  "simulated action failure",
			Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "false"}},
			Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}}}},
		}
		_, err := rt.RunStep(context.Background(), sess.SessionID, step)
		if err != nil {
			b.Fatal(err)
		}
	}
}
