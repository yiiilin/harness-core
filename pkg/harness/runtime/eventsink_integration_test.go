package runtime_test

import (
	"context"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/persistence"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type recordingEventSink struct {
	events []audit.Event
}

func (s *recordingEventSink) Emit(_ context.Context, event audit.Event) error {
	s.events = append(s.events, event)
	return nil
}

type sinkRunner struct {
	repos persistence.RepositorySet
}

func (r sinkRunner) Within(ctx context.Context, fn func(repos persistence.RepositorySet) error) error {
	return fn(r.repos)
}

func TestRunStepEmitsEventsViaConfiguredEventSink(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.echo", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, &countingHandler{})

	sink := &recordingEventSink{}
	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		EventSink: sink,
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "events", "emit events through sink")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "run a step"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(attached.SessionID, "single step", []plan.StepSpec{{
		StepID: "step_events",
		Title:  "emit",
		Action: action.Spec{ToolName: "demo.echo", Args: map[string]any{"message": "hello"}},
		Verify: verify.Spec{},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	if _, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0]); err != nil {
		t.Fatalf("run step: %v", err)
	}
	if len(sink.events) == 0 {
		t.Fatalf("expected configured EventSink to receive runtime events")
	}
}

func TestRunStepEmitsEventsViaConfiguredEventSinkWithinRunnerTransaction(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.echo", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, &countingHandler{})

	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	audits := audit.NewMemoryStore()
	sink := &recordingEventSink{}
	rt := hruntime.New(hruntime.Options{
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Audit:     audits,
		Tools:     tools,
		EventSink: sink,
		Runner: sinkRunner{repos: persistence.RepositorySet{
			Sessions: sessions,
			Tasks:    tasks,
			Plans:    plans,
			Audits:   audits,
		}},
		Policy: permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "tx events", "emit events through sink in tx path")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "run a step in tx path"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(attached.SessionID, "single step", []plan.StepSpec{{
		StepID: "step_tx_events",
		Title:  "emit tx",
		Action: action.Spec{ToolName: "demo.echo", Args: map[string]any{"message": "hello"}},
		Verify: verify.Spec{},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	if _, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0]); err != nil {
		t.Fatalf("run step: %v", err)
	}
	if len(sink.events) == 0 {
		t.Fatalf("expected configured EventSink to receive tx-path runtime events")
	}
}
