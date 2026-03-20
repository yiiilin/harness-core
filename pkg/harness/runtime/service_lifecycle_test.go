package runtime_test

import (
	"context"
	"errors"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/persistence"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

type selectiveFailingEventSink struct {
	failures map[string]error
}

func (s selectiveFailingEventSink) Emit(_ context.Context, event audit.Event) error {
	if err, ok := s.failures[event.Type]; ok {
		return err
	}
	return nil
}

func TestLifecycleEntryPointsEmitCreationEventsAndAttachUsesRunner(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	audits := audit.NewMemoryStore()
	runner := &countingRunner{repos: persistence.RepositorySet{
		Sessions: sessions,
		Tasks:    tasks,
		Plans:    plans,
		Audits:   audits,
	}}

	rt := hruntime.New(hruntime.Options{
		Sessions: sessions,
		Tasks:    tasks,
		Plans:    plans,
		Audit:    audits,
		Runner:   runner,
	})

	sess := mustCreateSession(t, rt, "lifecycle", "emit lifecycle events")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "attach task transactionally"})
	if _, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID); err != nil {
		t.Fatalf("attach task: %v", err)
	}
	if _, err := rt.CreatePlan(sess.SessionID, "initial plan", []plan.StepSpec{{StepID: "step_1", Title: "noop"}}); err != nil {
		t.Fatalf("create plan: %v", err)
	}

	if runner.calls == 0 {
		t.Fatalf("expected runner to be used for lifecycle operations")
	}

	events := mustListAuditEvents(t, rt, sess.SessionID)
	expected := map[string]bool{
		audit.EventSessionCreated: false,
		audit.EventTaskCreated:    false,
		audit.EventPlanGenerated:  false,
	}
	for _, event := range events {
		if _, ok := expected[event.Type]; ok {
			expected[event.Type] = true
		}
	}
	for typ, found := range expected {
		if !found {
			t.Fatalf("expected lifecycle event %s, got %#v", typ, events)
		}
	}
}

func TestLifecycleEntryPointsBestEffortWithoutRunnerAndTransactionalWithRunner(t *testing.T) {
	t.Run("session create stays successful when emit fails without runner", func(t *testing.T) {
		boom := errors.New("emit session.created failed")
		rt := hruntime.New(hruntime.Options{
			EventSink: selectiveFailingEventSink{failures: map[string]error{audit.EventSessionCreated: boom}},
		})
		rt.Runner = nil

		created, err := rt.CreateSession("broken session", "emit errors are best effort without a runner")
		if err != nil {
			t.Fatalf("expected session create to succeed without runner compensation, got %v", err)
		}
		sessions, err := rt.ListSessions()
		if err != nil {
			t.Fatalf("list sessions: %v", err)
		}
		if len(sessions) != 1 || sessions[0].SessionID != created.SessionID {
			t.Fatalf("expected created session to remain visible, got %#v", sessions)
		}
	})

	t.Run("task create stays successful when emit fails without runner", func(t *testing.T) {
		boom := errors.New("emit task.created failed")
		rt := hruntime.New(hruntime.Options{
			EventSink: selectiveFailingEventSink{failures: map[string]error{audit.EventTaskCreated: boom}},
		})
		rt.Runner = nil

		created, err := rt.CreateTask(task.Spec{TaskType: "demo", Goal: "emit task failure"})
		if err != nil {
			t.Fatalf("expected task create to succeed without runner compensation, got %v", err)
		}
		tasks, err := rt.ListTasks()
		if err != nil {
			t.Fatalf("list tasks: %v", err)
		}
		if len(tasks) != 1 || tasks[0].TaskID != created.TaskID {
			t.Fatalf("expected created task to remain visible, got %#v", tasks)
		}
	})

	t.Run("plan create stays successful when emit fails without runner", func(t *testing.T) {
		boom := errors.New("emit plan.generated failed")
		rt := hruntime.New(hruntime.Options{
			EventSink: selectiveFailingEventSink{failures: map[string]error{audit.EventPlanGenerated: boom}},
		})
		rt.Runner = nil

		sess := mustCreateSession(t, rt, "plan emit", "create plan must fail on emit")
		tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "emit plan failure"})
		attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
		if err != nil {
			t.Fatalf("attach task: %v", err)
		}

		created, err := rt.CreatePlan(attached.SessionID, "emit fail", []plan.StepSpec{{StepID: "step_1", Title: "noop"}})
		if err != nil {
			t.Fatalf("expected plan create to succeed without runner compensation, got %v", err)
		}
		plans, err := rt.ListPlans(attached.SessionID)
		if err != nil {
			t.Fatalf("list plans: %v", err)
		}
		if len(plans) != 1 || plans[0].PlanID != created.PlanID {
			t.Fatalf("expected created plan to remain visible, got %#v", plans)
		}
	})

	t.Run("runner path also returns emit errors", func(t *testing.T) {
		sessions := session.NewMemoryStore()
		tasks := task.NewMemoryStore()
		plans := plan.NewMemoryStore()
		audits := audit.NewMemoryStore()
		runner := &countingRunner{repos: persistence.RepositorySet{
			Sessions: sessions,
			Tasks:    tasks,
			Plans:    plans,
			Audits:   audits,
		}}
		boom := errors.New("runner emit failed")
		rt := hruntime.New(hruntime.Options{
			Sessions: sessions,
			Tasks:    tasks,
			Plans:    plans,
			Audit:    audits,
			Runner:   runner,
			EventSink: selectiveFailingEventSink{failures: map[string]error{
				audit.EventSessionCreated: boom,
			}},
		})

		if _, err := rt.CreateSession("runner broken", "runner emit failures must surface"); !errors.Is(err, boom) {
			t.Fatalf("expected runner-backed session create to surface emit error, got %v", err)
		}
	})
}
