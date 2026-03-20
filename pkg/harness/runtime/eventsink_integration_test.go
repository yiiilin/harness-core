package runtime_test

import (
	"context"
	"errors"
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

func TestRunStepBestEffortEventEmissionWithoutRunner(t *testing.T) {
	rt, sess, step := newHappyRuntime(t)
	rt.Runner = nil
	rt.EventSink = selectiveFailingEventSink{failures: map[string]error{
		audit.EventStateChanged: errors.New("boom:state.changed"),
	}}

	out, err := rt.RunStep(context.Background(), sess.SessionID, step)
	if err != nil {
		t.Fatalf("expected run step to stay successful without runner compensation, got %v", err)
	}
	if out.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected completed session despite sink failure, got %#v", out.Session)
	}

	stored, err := rt.GetSession(sess.SessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if stored.Phase != session.PhaseComplete {
		t.Fatalf("expected stored session to be complete, got %#v", stored)
	}
	if attempts := mustListAttempts(t, rt, sess.SessionID); len(attempts) != 1 {
		t.Fatalf("expected committed attempt record despite sink failure, got %#v", attempts)
	}
}

func TestRunStepPersistsExecutionFactsWithPartialRunnerRepositories(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.handle", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, runtimeHandleHandler{})

	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	audits := audit.NewMemoryStore()

	rt := hruntime.New(hruntime.Options{
		Sessions: sessions,
		Tasks:    tasks,
		Plans:    plans,
		Audit:    audits,
		Tools:    tools,
		Runner: sinkRunner{repos: persistence.RepositorySet{
			Sessions: sessions,
			Tasks:    tasks,
			Plans:    plans,
			Audits:   audits,
		}},
		Policy: permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "partial repos", "runner should fall back to service stores")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "persist execution facts through fallback stores"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(attached.SessionID, "single step", []plan.StepSpec{{
		StepID: "step_partial_runner",
		Title:  "persist through fallbacks",
		Action: action.Spec{ToolName: "demo.handle", Args: map[string]any{"mode": "interactive"}},
		Verify: verify.Spec{},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	if _, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0]); err != nil {
		t.Fatalf("run step: %v", err)
	}

	if attempts := mustListAttempts(t, rt, attached.SessionID); len(attempts) != 1 {
		t.Fatalf("expected fallback attempt persistence, got %#v", attempts)
	}
	if actions := mustListActions(t, rt, attached.SessionID); len(actions) != 1 {
		t.Fatalf("expected fallback action persistence, got %#v", actions)
	}
	if verifications := mustListVerifications(t, rt, attached.SessionID); len(verifications) != 1 {
		t.Fatalf("expected fallback verification persistence, got %#v", verifications)
	}
	if artifacts := mustListArtifacts(t, rt, attached.SessionID); len(artifacts) == 0 {
		t.Fatalf("expected fallback artifact persistence, got %#v", artifacts)
	}
	if snapshots := mustListCapabilitySnapshots(t, rt, attached.SessionID); len(snapshots) != 1 {
		t.Fatalf("expected fallback capability snapshot persistence, got %#v", snapshots)
	}
	handles, err := rt.ListRuntimeHandles(attached.SessionID)
	if err != nil {
		t.Fatalf("list runtime handles: %v", err)
	}
	if len(handles) != 1 {
		t.Fatalf("expected fallback runtime handle persistence, got %#v", handles)
	}
}
