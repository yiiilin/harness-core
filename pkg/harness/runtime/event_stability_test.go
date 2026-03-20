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

func TestRunStepAssignsStableEventIDs(t *testing.T) {
	rt, sess, step := newHappyRuntime()

	out, err := rt.RunStep(context.Background(), sess.SessionID, step)
	if err != nil {
		t.Fatalf("run step: %v", err)
	}

	seen := map[string]struct{}{}
	for _, event := range out.Events {
		if event.EventID == "" {
			t.Fatalf("expected non-empty event id in output events, got %#v", out.Events)
		}
		if _, exists := seen[event.EventID]; exists {
			t.Fatalf("expected unique event ids, got duplicate %s", event.EventID)
		}
		seen[event.EventID] = struct{}{}
	}

	stored := rt.ListAuditEvents(sess.SessionID)
	if len(stored) == 0 {
		t.Fatalf("expected stored audit events")
	}
	for _, event := range stored {
		if event.EventID == "" {
			t.Fatalf("expected stored event id, got %#v", stored)
		}
	}
}

func TestRunStepEventOrderingHappyPath(t *testing.T) {
	rt, sess, step := newHappyRuntime()

	out, err := rt.RunStep(context.Background(), sess.SessionID, step)
	if err != nil {
		t.Fatalf("run step: %v", err)
	}

	assertOrderedEventTypes(t, out.Events,
		"step.started",
		"tool.called",
		"tool.completed",
		"verify.completed",
		"state.changed",
	)
}

func TestRunStepEventOrderingPolicyDenied(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()

	rt := hruntime.New(hruntime.Options{
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Tools:     tools,
		Verifiers: verifiers,
	}).WithPolicyEvaluator(denyAllPolicy{})

	sess := rt.CreateSession("deny-ordering", "deny path ordering")
	tsk := rt.CreateTask(task.Spec{TaskType: "demo", Goal: "deny ordering"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	out, err := rt.RunStep(context.Background(), sess.SessionID, plan.StepSpec{
		StepID: "deny_order",
		Title:  "deny order",
		Action: action.Spec{ToolName: "windows.native", Args: map[string]any{"action": "click"}},
		Verify: verify.Spec{Mode: verify.ModeAll},
	})
	if err != nil {
		t.Fatalf("run step: %v", err)
	}

	assertOrderedEventTypes(t, out.Events,
		"step.started",
		"policy.denied",
		"state.changed",
	)
}

func TestRunStepEventOrderingVerifyFailure(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()

	tools.Register(tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true}, shellexec.PipeExecutor{})
	verifiers.Register(verify.Definition{Kind: "exit_code", Description: "Verify exit code."}, verify.ExitCodeChecker{})
	verifiers.Register(verify.Definition{Kind: "output_contains", Description: "Verify output contains substring."}, verify.OutputContainsChecker{})

	rt := hruntime.New(hruntime.Options{
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Tools:     tools,
		Verifiers: verifiers,
	})

	sess := rt.CreateSession("verify-ordering", "verify failure ordering")
	tsk := rt.CreateTask(task.Spec{TaskType: "demo", Goal: "verify failure ordering"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	out, err := rt.RunStep(context.Background(), sess.SessionID, plan.StepSpec{
		StepID: "verify_order",
		Title:  "verify order",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo hello", "timeout_ms": 5000}},
		Verify: verify.Spec{
			Mode: verify.ModeAll,
			Checks: []verify.Check{
				{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
				{Kind: "output_contains", Args: map[string]any{"text": "goodbye"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("run step: %v", err)
	}

	assertOrderedEventTypes(t, out.Events,
		"step.started",
		"tool.called",
		"tool.completed",
		"verify.completed",
		"state.changed",
	)
}

func assertOrderedEventTypes(t *testing.T, events []audit.Event, ordered ...string) {
	t.Helper()

	lastIndex := -1
	for _, want := range ordered {
		found := -1
		for idx := range events {
			if idx <= lastIndex {
				continue
			}
			if events[idx].Type == want {
				found = idx
				break
			}
		}
		if found == -1 {
			t.Fatalf("expected event %q after index %d, got %#v", want, lastIndex, events)
		}
		lastIndex = found
	}
}
