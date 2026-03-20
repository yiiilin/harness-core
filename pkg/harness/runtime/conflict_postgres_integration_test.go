package runtime_test

import (
	"context"
	"errors"
	"testing"

	"github.com/yiiilin/harness-core/internal/postgresruntime"
	"github.com/yiiilin/harness-core/internal/postgrestest"
	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/persistence"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

func newPostgresConflictRuntime(t *testing.T, policy any) (*hruntime.Service, *coordinatedSessionUpdateStore, *coordinatedApprovalUpdateStore, *countingHandler, func()) {
	t.Helper()

	pg := postgrestest.Start(t)
	db, err := postgresruntime.OpenDB(context.Background(), pg.DSN)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	handler := &countingHandler{}
	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		handler,
	)
	verifiers.Register(
		verify.Definition{Kind: "exit_code", Description: "Verify that an execution result exit code is in the allowed set."},
		verify.ExitCodeChecker{},
	)

	opts := postgresruntime.BuildOptions(db, hruntime.Options{
		Tools:     tools,
		Verifiers: verifiers,
	})
	if evaluator, ok := policy.(interface {
		Evaluate(context.Context, session.State, plan.StepSpec) (any, error)
	}); ok {
		_ = evaluator
	}
	switch p := policy.(type) {
	case askPolicy:
		opts.Policy = p
	case nil:
	default:
		t.Fatalf("unsupported policy type %T", policy)
	}

	sessionStore := &coordinatedSessionUpdateStore{Store: opts.Sessions}
	approvalStore := &coordinatedApprovalUpdateStore{Store: opts.Approvals}
	opts.Sessions = sessionStore
	opts.Approvals = approvalStore
	opts.Runner = persistence.NewMemoryUnitOfWork(persistence.RepositorySet{
		Sessions:            sessionStore,
		Tasks:               opts.Tasks,
		Plans:               opts.Plans,
		Audits:              opts.Audit,
		Attempts:            opts.Attempts,
		Actions:             opts.Actions,
		Verifications:       opts.Verifications,
		Artifacts:           opts.Artifacts,
		RuntimeHandles:      opts.RuntimeHandles,
		Approvals:           approvalStore,
		CapabilitySnapshots: opts.CapabilitySnapshots,
	})

	rt := hruntime.New(opts)
	cleanup := func() {
		_ = db.Close()
	}
	return rt, sessionStore, approvalStore, handler, cleanup
}

func TestPostgresRespondApprovalHasSingleWinnerUnderApprovalCAS(t *testing.T) {
	rt, _, approvals, handler, cleanup := newPostgresConflictRuntime(t, askPolicy{})
	defer cleanup()

	sess := mustCreateSession(t, rt, "pg respond race", "only one approval response should win")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "pg respond race"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(sess.SessionID, "pg respond race", []plan.StepSpec{{
		StepID: "step_pg_respond_race",
		Title:  "postgres approval race",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo gated", "timeout_ms": 5000}},
		Verify: verify.Spec{},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	initial, err := rt.RunStep(context.Background(), sess.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	approvalID := initial.Execution.PendingApproval.ApprovalID
	approvals.arm(approvalID, 2)

	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, _, err := rt.RespondApproval(approvalID, approval.Response{Reply: approval.ReplyOnce})
			errs <- err
		}()
	}

	successes := 0
	conflicts := 0
	for i := 0; i < 2; i++ {
		err := <-errs
		switch {
		case err == nil:
			successes++
		case errors.Is(err, approval.ErrApprovalVersionConflict):
			conflicts++
		default:
			t.Fatalf("unexpected respond result: %v", err)
		}
	}

	if successes != 1 || conflicts != 1 {
		t.Fatalf("expected one success and one approval conflict, got %d successes and %d conflicts", successes, conflicts)
	}
	if handler.calls != 0 {
		t.Fatalf("expected no tool execution during respond race, got %d", handler.calls)
	}
}

func TestPostgresResumePendingApprovalHasSingleWinnerUnderSessionCAS(t *testing.T) {
	rt, sessions, _, handler, cleanup := newPostgresConflictRuntime(t, askPolicy{})
	defer cleanup()

	sess := mustCreateSession(t, rt, "pg resume race", "only one postgres resume should win")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "pg resume race"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(sess.SessionID, "pg resume race", []plan.StepSpec{{
		StepID: "step_pg_resume_race",
		Title:  "postgres resume race",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo resumed", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
		}},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	initial, err := rt.RunStep(context.Background(), sess.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	if _, _, err := rt.RespondApproval(initial.Execution.PendingApproval.ApprovalID, approval.Response{Reply: approval.ReplyOnce}); err != nil {
		t.Fatalf("respond approval: %v", err)
	}

	sessions.arm(sess.SessionID, 2)

	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, err := rt.ResumePendingApproval(context.Background(), sess.SessionID)
			errs <- err
		}()
	}

	successes := 0
	conflicts := 0
	for i := 0; i < 2; i++ {
		err := <-errs
		switch {
		case err == nil:
			successes++
		case errors.Is(err, session.ErrSessionVersionConflict):
			conflicts++
		default:
			t.Fatalf("unexpected resume result: %v", err)
		}
	}

	if successes != 1 || conflicts != 1 {
		t.Fatalf("expected one success and one session conflict, got %d successes and %d conflicts", successes, conflicts)
	}
	if handler.calls != 1 {
		t.Fatalf("expected exactly one tool execution, got %d", handler.calls)
	}
}

func TestPostgresRecoverSessionHasSingleWinnerUnderSessionCAS(t *testing.T) {
	rt, sessions, _, handler, cleanup := newPostgresConflictRuntime(t, nil)
	defer cleanup()

	sess := mustCreateSession(t, rt, "pg recover race", "only one postgres recovery should win")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "pg recover race"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	if _, err := rt.CreatePlan(sess.SessionID, "pg recover race", []plan.StepSpec{{
		StepID: "step_pg_recover_race",
		Title:  "postgres recover race",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo recovered", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
		}},
	}}); err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := rt.MarkSessionInterrupted(context.Background(), sess.SessionID); err != nil {
		t.Fatalf("mark interrupted: %v", err)
	}

	sessions.arm(sess.SessionID, 2)

	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, err := rt.RecoverSession(context.Background(), sess.SessionID)
			errs <- err
		}()
	}

	successes := 0
	conflicts := 0
	for i := 0; i < 2; i++ {
		err := <-errs
		switch {
		case err == nil:
			successes++
		case errors.Is(err, session.ErrSessionVersionConflict):
			conflicts++
		default:
			t.Fatalf("unexpected recover result: %v", err)
		}
	}

	if successes != 1 || conflicts != 1 {
		t.Fatalf("expected one success and one session conflict, got %d successes and %d conflicts", successes, conflicts)
	}
	if handler.calls != 1 {
		t.Fatalf("expected exactly one recovery execution, got %d", handler.calls)
	}
}
