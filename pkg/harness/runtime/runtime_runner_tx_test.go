package runtime_test

import (
	"context"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	shellexec "github.com/yiiilin/harness-core/pkg/harness/executor/shell"
	"github.com/yiiilin/harness-core/pkg/harness/persistence"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type countingRunner struct {
	repos persistence.RepositorySet
	calls int
}

func (r *countingRunner) Within(ctx context.Context, fn func(repos persistence.RepositorySet) error) error {
	r.calls++
	_ = ctx
	return fn(r.repos)
}

func TestRunStepUsesRunnerBoundary(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	audits := audit.NewMemoryStore()
	runner := &countingRunner{repos: persistence.RepositorySet{Sessions: sessions, Tasks: tasks, Plans: plans, Audits: audits}}

	tools.Register(tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true}, shellexec.PipeExecutor{})
	verifiers.Register(verify.Definition{Kind: "exit_code", Description: "Verify exit code."}, verify.ExitCodeChecker{})
	verifiers.Register(verify.Definition{Kind: "output_contains", Description: "Verify output contains substring."}, verify.OutputContainsChecker{})

	rt := hruntime.New(hruntime.Options{Sessions: sessions, Tasks: tasks, Plans: plans, Tools: tools, Verifiers: verifiers, Audit: audits, Runner: runner})
	sess := rt.CreateSession("tx", "runner boundary")
	tsk := rt.CreateTask(task.Spec{TaskType: "demo", Goal: "runner boundary"})
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
	if runner.calls == 0 {
		t.Fatalf("expected runner to be used")
	}
}
