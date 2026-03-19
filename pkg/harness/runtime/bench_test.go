package runtime_test

import (
	"context"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	shellexec "github.com/yiiilin/harness-core/pkg/harness/executor/shell"
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type denyBenchPolicy struct{}

func (denyBenchPolicy) Evaluate(_ context.Context, _ session.State, _ plan.StepSpec) (permission.Decision, error) {
	return permission.Decision{Action: permission.Deny, Reason: "bench deny", MatchedRule: "bench/*"}, nil
}

func newHappyRuntime() (*hruntime.Service, session.State, plan.StepSpec) {
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
	sess := rt.CreateSession("bench", "benchmark run")
	tsk := rt.CreateTask(task.Spec{TaskType: "bench", Goal: "run benchmark shell step"})
	sess, _ = rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	step := plan.StepSpec{
		StepID: "bench_step",
		Title:  "echo hello",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo hello", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}}, {Kind: "output_contains", Args: map[string]any{"text": "hello"}}}},
		OnFail: plan.OnFailSpec{Strategy: "abort"},
	}
	return rt, sess, step
}

func BenchmarkRunStepHappyPath(b *testing.B) {
	for i := 0; i < b.N; i++ {
		rt, sess, step := newHappyRuntime()
		_, err := rt.RunStep(context.Background(), sess.SessionID, step)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRunStepPolicyDenied(b *testing.B) {
	for i := 0; i < b.N; i++ {
		sessions := session.NewMemoryStore()
		tasks := task.NewMemoryStore()
		plans := plan.NewMemoryStore()
		tools := tool.NewRegistry()
		verifiers := verify.NewRegistry()
		audits := audit.NewMemoryStore()
		rt := hruntime.New(hruntime.Options{Sessions: sessions, Tasks: tasks, Plans: plans, Tools: tools, Verifiers: verifiers, Audit: audits}).WithPolicyEvaluator(denyBenchPolicy{})
		sess := rt.CreateSession("deny", "deny path")
		tsk := rt.CreateTask(task.Spec{TaskType: "bench", Goal: "deny path"})
		sess, _ = rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
		step := plan.StepSpec{StepID: "deny", Title: "deny", Action: action.Spec{ToolName: "windows.native", Args: map[string]any{"action": "click"}}, Verify: verify.Spec{Mode: verify.ModeAll}}
		_, err := rt.RunStep(context.Background(), sess.SessionID, step)
		if err != nil {
			b.Fatal(err)
		}
	}
}
