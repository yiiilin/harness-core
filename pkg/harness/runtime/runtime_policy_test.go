package runtime_test

import (
	"context"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type denyAllPolicy struct{}

func (denyAllPolicy) Evaluate(_ context.Context, _ session.State, step plan.StepSpec) (permission.Decision, error) {
	return permission.Decision{Action: permission.Deny, Reason: "test deny", MatchedRule: "test/*"}, nil
}

func TestRunStepPolicyDenied(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	audits := audit.NewMemoryStore()

	rt := hruntime.New(hruntime.Options{Sessions: sessions, Tasks: tasks, Plans: plans, Tools: tools, Verifiers: verifiers, Audit: audits}).WithPolicyEvaluator(denyAllPolicy{})

	sess := mustCreateSession(t, rt, "deny session", "deny path")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "denied action should fail safely"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(attached.SessionID, "deny", []plan.StepSpec{{
		StepID: "step_deny",
		Title:  "forbidden windows call",
		Action: action.Spec{ToolName: "windows.native", Args: map[string]any{"action": "click"}},
		Verify: verify.Spec{Mode: verify.ModeAll},
		OnFail: plan.OnFailSpec{Strategy: "abort"},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	out, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}

	if out.Session.Phase != session.PhaseFailed {
		t.Fatalf("expected session failed, got %s", out.Session.Phase)
	}
	if out.Execution.Policy.Decision.Action != permission.Deny {
		t.Fatalf("expected deny decision, got %#v", out.Execution.Policy)
	}
	if out.UpdatedTask == nil || out.UpdatedTask.Status != task.StatusFailed {
		t.Fatalf("expected task failed, got %#v", out.UpdatedTask)
	}
	stored := mustListAuditEvents(t, rt, attached.SessionID)
	if len(stored) == 0 {
		t.Fatalf("expected audit events, got none")
	}
	foundPolicyDenied := false
	for _, event := range stored {
		if event.Type == audit.EventPolicyDenied {
			foundPolicyDenied = true
			break
		}
	}
	if !foundPolicyDenied {
		t.Fatalf("expected policy.denied event in audit trail")
	}
}
