package runtime_test

import (
	"context"
	"errors"
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

type nthFailingSessionGetStore struct {
	session.Store
	getErr        error
	failOnGetCall int
	getCalls      int
}

func (s *nthFailingSessionGetStore) Get(id string) (session.State, error) {
	s.getCalls++
	if s.failOnGetCall > 0 && s.getCalls == s.failOnGetCall {
		return session.State{}, s.getErr
	}
	return s.Store.Get(id)
}

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

func TestRunStepPolicyDeniedDoesNotAnchorRuntimeBudgetOrEmitRecoveryState(t *testing.T) {
	clock := &fakeClock{now: 1000}
	sessions := session.NewMemoryStoreWithClock(clock)
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	audits := audit.NewMemoryStore()

	rt := hruntime.New(hruntime.Options{
		Clock:     clock,
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Tools:     tools,
		Verifiers: verifiers,
		Audit:     audits,
	}).WithPolicyEvaluator(denyAllPolicy{})

	sess := mustCreateSession(t, rt, "deny anchor", "deny path should not burn runtime budget")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "denied action should not enter recovery state"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(attached.SessionID, "deny", []plan.StepSpec{{
		StepID: "step_deny_anchor",
		Title:  "forbidden action should stay out of recovery bookkeeping",
		Action: action.Spec{ToolName: "windows.native", Args: map[string]any{"action": "click"}},
		Verify: verify.Spec{Mode: verify.ModeAll},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	out, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	if out.Session.RuntimeStartedAt != 0 {
		t.Fatalf("expected denied step to leave runtime_started_at unset, got %#v", out.Session)
	}
	if out.Session.ExecutionState != session.ExecutionIdle || out.Session.InFlightStepID != "" {
		t.Fatalf("expected denied step to stay out of in-flight execution state, got %#v", out.Session)
	}

	stored, err := rt.GetSession(attached.SessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if stored.RuntimeStartedAt != 0 {
		t.Fatalf("expected persisted session to leave runtime_started_at unset after deny, got %#v", stored)
	}
	if recoverable, err := rt.ListRecoverableSessions(); err != nil {
		t.Fatalf("list recoverable sessions: %v", err)
	} else if len(recoverable) != 0 {
		t.Fatalf("expected denied step not to create recoverable sessions, got %#v", recoverable)
	}

	events := mustListAuditEvents(t, rt, attached.SessionID)
	for _, event := range events {
		if event.Type == audit.EventRecoveryStateChanged {
			t.Fatalf("did not expect deny path to emit recovery state changes, got %#v", events)
		}
	}
}

func TestRunStepFailsWhenAuthoritativeSessionReloadFailsAfterEnteringInFlight(t *testing.T) {
	boom := errors.New("boom:session.get")
	sessions := &nthFailingSessionGetStore{
		Store:         session.NewMemoryStore(),
		getErr:        boom,
		failOnGetCall: 4,
	}
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	audits := audit.NewMemoryStore()
	handler := &countingHandler{}

	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		handler,
	)
	verifiers.Register(
		verify.Definition{Kind: "exit_code", Description: "Verify that an execution result exit code is in the allowed set."},
		verify.ExitCodeChecker{},
	)

	rt := hruntime.New(hruntime.Options{
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Tools:     tools,
		Verifiers: verifiers,
		Audit:     audits,
	})

	sess := mustCreateSession(t, rt, "reload failure", "authoritative session reloads must not be ignored")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "fail fast when session reload breaks"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(attached.SessionID, "reload failure", []plan.StepSpec{{
		StepID: "step_reload_failure",
		Title:  "post in-flight reload should be required",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo should-not-run", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
		}},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	if _, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0]); !errors.Is(err, boom) {
		t.Fatalf("expected post in-flight session reload error, got %v", err)
	}
	if handler.calls != 0 {
		t.Fatalf("expected action not to execute after authoritative session reload failed, got %d calls", handler.calls)
	}
}
