package runtime_test

import (
	"context"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	shellexec "github.com/yiiilin/harness-core/pkg/harness/executor/shell"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

func BenchmarkRunStepVerifyFailure(b *testing.B) {
	for i := 0; i < b.N; i++ {
		sessions := session.NewMemoryStore()
		tasks := task.NewMemoryStore()
		plans := plan.NewMemoryStore()
		tools := tool.NewRegistry()
		verifiers := verify.NewRegistry()
		audits := audit.NewMemoryStore()

		tools.Register(tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true}, shellexec.PipeExecutor{})
		verifiers.Register(verify.Definition{Kind: "exit_code", Description: "Verify exit code."}, verify.ExitCodeChecker{})
		verifiers.Register(verify.Definition{Kind: "output_contains", Description: "Verify output contains substring."}, verify.OutputContainsChecker{})

		rt := hruntime.New(hruntime.Options{Sessions: sessions, Tasks: tasks, Plans: plans, Tools: tools, Verifiers: verifiers, Audit: audits})
		sess := rt.CreateSession("verify-fail", "verify failure path")
		tsk := rt.CreateTask(task.Spec{TaskType: "bench", Goal: "verify failure path"})
		sess, _ = rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
		step := plan.StepSpec{
			StepID: "step_fail",
			Title:  "echo hello but expect missing text",
			Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo hello", "timeout_ms": 5000}},
			Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}}, {Kind: "output_contains", Args: map[string]any{"text": "goodbye"}}}},
		}
		_, err := rt.RunStep(context.Background(), sess.SessionID, step)
		if err != nil {
			b.Fatal(err)
		}
	}
}
