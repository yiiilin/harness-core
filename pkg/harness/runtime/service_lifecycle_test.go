package runtime_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/builtins"
	"github.com/yiiilin/harness-core/pkg/harness/persistence"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
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
		audit.EventSessionCreated:      false,
		audit.EventTaskCreated:         false,
		audit.EventSessionTaskAttached: false,
		audit.EventPlanGenerated:       false,
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

	t.Run("attach task stays successful when emit fails without runner", func(t *testing.T) {
		boom := errors.New("emit session.task_attached failed")
		rt := hruntime.New(hruntime.Options{
			EventSink: selectiveFailingEventSink{failures: map[string]error{audit.EventSessionTaskAttached: boom}},
		})
		rt.Runner = nil

		sess := mustCreateSession(t, rt, "attach emit", "attach should stay best effort without runner")
		tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "emit attach failure"})

		attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
		if err != nil {
			t.Fatalf("expected attach to succeed without runner compensation, got %v", err)
		}
		if attached.TaskID != tsk.TaskID {
			t.Fatalf("expected task attachment to persist, got %#v", attached)
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

	t.Run("runner-backed attach task surfaces emit errors", func(t *testing.T) {
		sessions := session.NewMemoryStore()
		tasks := task.NewMemoryStore()
		audits := audit.NewMemoryStore()
		runner := &countingRunner{repos: persistence.RepositorySet{
			Sessions: sessions,
			Tasks:    tasks,
			Audits:   audits,
		}}
		boom := errors.New("runner attach emit failed")
		rt := hruntime.New(hruntime.Options{
			Sessions: sessions,
			Tasks:    tasks,
			Audit:    audits,
			Runner:   runner,
			EventSink: selectiveFailingEventSink{failures: map[string]error{
				audit.EventSessionTaskAttached: boom,
			}},
		})

		sess := mustCreateSession(t, rt, "runner attach", "runner attach emit failures must surface")
		tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "attach under runner"})

		if _, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID); !errors.Is(err, boom) {
			t.Fatalf("expected runner-backed attach to surface emit error, got %v", err)
		}
	})
}

func TestAttachTaskToSessionCompensatesSessionWriteOnNoRunnerTaskStoreFailure(t *testing.T) {
	boom := errors.New("boom:task.update")
	sessions := session.NewMemoryStore()
	tasks := &nthFailingTaskUpdateStore{
		Store:            task.NewMemoryStore(),
		updateErr:        boom,
		failOnUpdateCall: 1,
	}
	rt := hruntime.New(hruntime.Options{
		Sessions: sessions,
		Tasks:    tasks,
	})
	rt.Runner = nil

	sess := mustCreateSession(t, rt, "attach rollback", "session goal should survive failed attach")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "task goal"})

	if _, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID); !errors.Is(err, boom) {
		t.Fatalf("expected attach to surface task update failure, got %v", err)
	}

	persistedSession, err := rt.GetSession(sess.SessionID)
	if err != nil {
		t.Fatalf("get session after failed attach: %v", err)
	}
	if persistedSession.TaskID != "" || persistedSession.Goal != "session goal should survive failed attach" {
		t.Fatalf("expected session attachment to be compensated, got %#v", persistedSession)
	}

	persistedTask, err := rt.GetTask(tsk.TaskID)
	if err != nil {
		t.Fatalf("get task after failed attach: %v", err)
	}
	if persistedTask.SessionID != "" || persistedTask.Status != task.StatusReceived {
		t.Fatalf("expected task to remain unattached after failed attach, got %#v", persistedTask)
	}
}

func TestControlPlaneEntryPointsEmitAttachLeaseAndRecoveryEvents(t *testing.T) {
	opts := hruntime.Options{}
	builtins.Register(&opts)
	rt := hruntime.New(opts)

	sess := mustCreateSession(t, rt, "control-plane events", "control-plane mutations should be auditable")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "emit attach and recovery events"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(attached.SessionID, "recover control-plane events", []plan.StepSpec{{
		StepID: "step_control_plane_events",
		Title:  "recover control-plane events",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo recover", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
			{Kind: "output_contains", Args: map[string]any{"text": "recover"}},
		}},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := rt.MarkSessionInFlight(context.Background(), attached.SessionID, pl.Steps[0].StepID); err != nil {
		t.Fatalf("mark in-flight: %v", err)
	}
	if _, err := rt.MarkSessionInterrupted(context.Background(), attached.SessionID); err != nil {
		t.Fatalf("mark interrupted: %v", err)
	}
	if _, err := rt.RecoverSession(context.Background(), attached.SessionID); err != nil {
		t.Fatalf("recover session: %v", err)
	}

	leaseTarget := mustCreateSession(t, rt, "lease events", "lease mutations should be auditable")
	claimed, ok, err := rt.ClaimRunnableSession(context.Background(), time.Minute)
	if err != nil {
		t.Fatalf("claim runnable session: %v", err)
	}
	if !ok || claimed.SessionID != leaseTarget.SessionID {
		t.Fatalf("expected lease target %q to be claimed, got ok=%v state=%#v", leaseTarget.SessionID, ok, claimed)
	}
	if _, err := rt.RenewSessionLease(context.Background(), claimed.SessionID, claimed.LeaseID, time.Minute); err != nil {
		t.Fatalf("renew session lease: %v", err)
	}
	if _, err := rt.ReleaseSessionLease(context.Background(), claimed.SessionID, claimed.LeaseID); err != nil {
		t.Fatalf("release session lease: %v", err)
	}

	controlEvents := mustListAuditEvents(t, rt, attached.SessionID)
	expectedControl := map[string]bool{
		audit.EventSessionTaskAttached:  false,
		audit.EventRecoveryStateChanged: false,
	}
	recoveryMutations := map[string]bool{
		"in_flight":   false,
		"interrupted": false,
		"recovered":   false,
	}
	for _, event := range controlEvents {
		if _, ok := expectedControl[event.Type]; ok {
			expectedControl[event.Type] = true
		}
		if event.Type == audit.EventRecoveryStateChanged {
			if mutation, _ := event.Payload["mutation"].(string); mutation != "" {
				if _, ok := recoveryMutations[mutation]; ok {
					recoveryMutations[mutation] = true
				}
			}
		}
	}
	for typ, found := range expectedControl {
		if !found {
			t.Fatalf("expected control-plane event %s, got %#v", typ, controlEvents)
		}
	}
	for mutation, found := range recoveryMutations {
		if !found {
			t.Fatalf("expected recovery mutation %s in audit trail, got %#v", mutation, controlEvents)
		}
	}

	leaseEvents := mustListAuditEvents(t, rt, leaseTarget.SessionID)
	expectedLease := map[string]bool{
		audit.EventLeaseClaimed:  false,
		audit.EventLeaseRenewed:  false,
		audit.EventLeaseReleased: false,
	}
	for _, event := range leaseEvents {
		if _, ok := expectedLease[event.Type]; ok {
			expectedLease[event.Type] = true
		}
	}
	for typ, found := range expectedLease {
		if !found {
			t.Fatalf("expected lease event %s, got %#v", typ, leaseEvents)
		}
	}
}
