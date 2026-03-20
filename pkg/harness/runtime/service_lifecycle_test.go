package runtime_test

import (
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/persistence"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

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
