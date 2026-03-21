// Command postgres-embedded shows the public durable Postgres bootstrap without any adapter layer.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/builtins"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hpostgres "github.com/yiiilin/harness-core/pkg/harness/postgres"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type DemoResult struct {
	StorageMode  string
	Session      session.State
	Output       string
	AttemptCount int
}

func main() {
	dsn := strings.TrimSpace(os.Getenv("HARNESS_POSTGRES_DSN"))
	if dsn == "" {
		panic("HARNESS_POSTGRES_DSN is required")
	}

	result, err := RunEmbeddedDemo(context.Background(), dsn)
	if err != nil {
		panic(err)
	}

	fmt.Printf("storage: %s\n", result.StorageMode)
	fmt.Printf("session: %s (%s)\n", result.Session.SessionID, result.Session.Phase)
	fmt.Printf("attempts: %d\n", result.AttemptCount)
	fmt.Printf("output: %s\n", strings.TrimSpace(result.Output))
}

// RunEmbeddedDemo opens a durable runtime, seeds one verified shell step, executes it,
// and reads back persisted attempts through the public Postgres bootstrap path.
func RunEmbeddedDemo(ctx context.Context, dsn string) (DemoResult, error) {
	var opts hruntime.Options
	builtins.Register(&opts)

	rt, db, err := hpostgres.OpenService(ctx, dsn, opts)
	if err != nil {
		return DemoResult{}, err
	}
	defer db.Close()

	sess, err := rt.CreateSession("postgres-embedded", "run a durable public bootstrap demo")
	if err != nil {
		return DemoResult{}, err
	}
	tsk, err := rt.CreateTask(task.Spec{
		TaskType: "demo",
		Goal:     "echo hello from durable runtime",
	})
	if err != nil {
		return DemoResult{}, err
	}
	sess, err = rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		return DemoResult{}, err
	}

	step := plan.StepSpec{
		StepID: "step_postgres_embedded_demo",
		Title:  "run durable shell step",
		Action: action.Spec{
			ToolName: "shell.exec",
			Args: map[string]any{
				"mode":       "pipe",
				"command":    "echo hello from durable runtime",
				"timeout_ms": 5000,
			},
		},
		Verify: verify.Spec{
			Mode: verify.ModeAll,
			Checks: []verify.Check{
				{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
				{Kind: "output_contains", Args: map[string]any{"text": "hello from durable runtime"}},
			},
		},
	}
	pl, err := rt.CreatePlan(sess.SessionID, "public durable bootstrap demo", []plan.StepSpec{step})
	if err != nil {
		return DemoResult{}, err
	}

	out, err := rt.RunStep(ctx, sess.SessionID, pl.Steps[0])
	if err != nil {
		return DemoResult{}, err
	}
	attempts, err := rt.ListAttempts(sess.SessionID)
	if err != nil {
		return DemoResult{}, err
	}

	stdout, _ := out.Execution.Action.Data["stdout"].(string)
	return DemoResult{
		StorageMode:  rt.StorageMode,
		Session:      out.Session,
		Output:       stdout,
		AttemptCount: len(attempts),
	}, nil
}
