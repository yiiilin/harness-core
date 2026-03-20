package runtime_test

import (
	"context"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

func BenchmarkRunStepTimeoutPath(b *testing.B) {
	for i := 0; i < b.N; i++ {
		rt, sess, _ := newHappyRuntime(b)
		step := plan.StepSpec{
			StepID: "bench_timeout",
			Title:  "sleep past timeout",
			Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "sleep 0.05", "timeout_ms": 1}},
			Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}}}},
			OnFail: plan.OnFailSpec{Strategy: "abort"},
		}
		_, err := rt.RunStep(context.Background(), sess.SessionID, step)
		if err != nil {
			b.Fatal(err)
		}
	}
}
