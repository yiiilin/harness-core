package runtime_test

import (
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

func TestTaskSessionPlanWiring(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	audits := audit.NewMemoryStore()

	rt := hruntime.New(hruntime.Options{Sessions: sessions, Tasks: tasks, Plans: plans, Tools: tools, Verifiers: verifiers, Audit: audits})

	sess := rt.CreateSession("wiring", "check wiring")
	tsk := rt.CreateTask(task.Spec{TaskType: "demo", Goal: "task session relation"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	if attached.TaskID != tsk.TaskID {
		t.Fatalf("expected attached session task id %s, got %s", tsk.TaskID, attached.TaskID)
	}

	first, err := rt.CreatePlan(attached.SessionID, "initial", []plan.StepSpec{{StepID: "s1", Title: "step 1"}})
	if err != nil {
		t.Fatalf("create first plan: %v", err)
	}
	second, err := rt.CreatePlan(attached.SessionID, "revision", []plan.StepSpec{{StepID: "s2", Title: "step 2"}})
	if err != nil {
		t.Fatalf("create second plan: %v", err)
	}
	if first.Revision != 1 {
		t.Fatalf("expected first revision 1, got %d", first.Revision)
	}
	if second.Revision != 2 {
		t.Fatalf("expected second revision 2, got %d", second.Revision)
	}
	plansForSession := rt.ListPlans(attached.SessionID)
	if len(plansForSession) != 2 {
		t.Fatalf("expected 2 plans, got %d", len(plansForSession))
	}
}
