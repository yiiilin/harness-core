package runtime

import (
	"context"
	goruntime "runtime"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

func TestExecuteFanoutPreparedStepsDoesNotLetBlockedSiblingConsumeProgramSlot(t *testing.T) {
	previousMaxProcs := goruntime.GOMAXPROCS(1)
	defer goruntime.GOMAXPROCS(previousMaxProcs)

	handler := newInternalLabeledBlockingHandler()
	tools := tool.NewRegistry()
	tools.Register(
		tool.Definition{ToolName: "demo.internal-slot-test", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true},
		handler,
	)

	rt := New(Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	defer handler.releaseAll()

	prepared := []fanoutPreparedStep{
		internalPreparedStep("step_host_a", "host-a", "agg", 1),
		internalPreparedStep("step_host_b", "host-b", "agg", 1),
		internalPreparedStep("step_collect", "collect", "step_collect", 1),
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		rt.executeFanoutPreparedSteps(ctx, session.State{Phase: session.PhaseExecute}, prepared, 2)
	}()

	first := handler.waitForStart(t, time.Second)
	second := handler.waitForStart(t, 100*time.Millisecond)
	started := map[string]bool{
		first:  true,
		second: true,
	}
	if started["host-b"] {
		t.Fatalf("expected throttled sibling target not to consume the second program slot before collect, got %q then %q", first, second)
	}
	if !started["collect"] {
		t.Fatalf("expected collect to start within the first two slots, got %q then %q", first, second)
	}

	handler.releaseAll()
	<-done
}

func internalPreparedStep(stepID, label, group string, limit int) fanoutPreparedStep {
	step := plan.StepSpec{
		StepID: stepID,
		Action: action.Spec{
			ToolName: "demo.internal-slot-test",
			Args:     map[string]any{"label": label},
		},
	}
	return fanoutPreparedStep{
		Original: step,
		Step:     step,
		Decision: permission.Decision{Action: permission.Allow, Reason: "allow"},
		Attempt: execution.Attempt{
			AttemptID: "att_" + uuid.NewString(),
			SessionID: "sess",
			TaskID:    "task",
			StepID:    stepID,
			TraceID:   "trc_" + uuid.NewString(),
			CycleID:   "cyc_" + uuid.NewString(),
			Step:      step,
			StartedAt: time.Now().UnixMilli(),
		},
		ConcurrencyGroup: group,
		ConcurrencyLimit: limit,
	}
}

type internalLabeledBlockingHandler struct {
	started chan string
	release chan struct{}
	once    sync.Once
}

func newInternalLabeledBlockingHandler() *internalLabeledBlockingHandler {
	return &internalLabeledBlockingHandler{
		started: make(chan string, 8),
		release: make(chan struct{}),
	}
}

func (h *internalLabeledBlockingHandler) Invoke(ctx context.Context, args map[string]any) (action.Result, error) {
	label, _ := args["label"].(string)
	h.started <- label
	select {
	case <-h.release:
	case <-ctx.Done():
		return action.Result{
			OK: false,
			Error: &action.Error{
				Code:    "CONCURRENCY_TIMEOUT",
				Message: ctx.Err().Error(),
			},
		}, ctx.Err()
	}
	return action.Result{OK: true, Data: map[string]any{"label": label}}, nil
}

func (h *internalLabeledBlockingHandler) waitForStart(t *testing.T, timeout time.Duration) string {
	t.Helper()
	select {
	case label := <-h.started:
		return label
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for start after %s", timeout)
		return ""
	}
}

func (h *internalLabeledBlockingHandler) releaseAll() {
	h.once.Do(func() {
		close(h.release)
	})
}
