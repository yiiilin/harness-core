package runtime_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yiiilin/harness-core/internal/postgrestest"
	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/builtins"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

func TestRecoveryReadPathAcrossPostgresRuntimeReinit(t *testing.T) {
	pg := postgrestest.Start(t)

	opts := hruntime.Options{}
	builtins.Register(&opts)

	rt1, db1 := pg.OpenService(t, opts)
	defer db1.Close()

	sess := mustCreateSession(t, rt1, "durable recovery", "mark in-flight and recover later")
	if _, err := rt1.MarkSessionInFlight(context.Background(), sess.SessionID, "step_1"); err != nil {
		t.Fatalf("mark in-flight: %v", err)
	}
	if _, err := rt1.MarkSessionInterrupted(context.Background(), sess.SessionID); err != nil {
		t.Fatalf("mark interrupted: %v", err)
	}

	rt2, db2 := pg.OpenService(t, opts)
	defer db2.Close()

	items := mustListRecoverableSessions(t, rt2)
	if len(items) != 1 {
		t.Fatalf("expected 1 recoverable session, got %d", len(items))
	}
	if items[0].SessionID != sess.SessionID {
		t.Fatalf("expected session %s, got %s", sess.SessionID, items[0].SessionID)
	}
	if items[0].ExecutionState != session.ExecutionInterrupted {
		t.Fatalf("expected interrupted execution state, got %s", items[0].ExecutionState)
	}
	if items[0].InFlightStepID != "step_1" {
		t.Fatalf("expected in-flight step step_1, got %s", items[0].InFlightStepID)
	}
}

func TestPostgresRecoverClaimedSessionRequiresCurrentLease(t *testing.T) {
	pg := postgrestest.Start(t)

	opts := hruntime.Options{}
	builtins.Register(&opts)

	rt, db := pg.OpenService(t, opts)
	defer db.Close()

	sess := mustCreateSession(t, rt, "durable claimed recovery", "claimed recovery should enforce lease ownership")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "claim before postgres recovery"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	if _, err := rt.CreatePlan(sess.SessionID, "postgres claimed recovery", []plan.StepSpec{{
		StepID: "step_pg_claimed_recover",
		Title:  "recover after postgres claim",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo durable-claimed", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
			{Kind: "output_contains", Args: map[string]any{"text": "durable-claimed"}},
		}},
	}}); err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := rt.MarkSessionInterrupted(context.Background(), sess.SessionID); err != nil {
		t.Fatalf("mark interrupted: %v", err)
	}

	claimed, ok, err := rt.ClaimRecoverableSession(context.Background(), time.Minute)
	if err != nil {
		t.Fatalf("claim recoverable session: %v", err)
	}
	if !ok {
		t.Fatalf("expected recoverable session to be claimed")
	}
	if claimed.SessionID != sess.SessionID {
		t.Fatalf("expected claimed session %s, got %#v", sess.SessionID, claimed)
	}

	if _, err := rt.RecoverSession(context.Background(), sess.SessionID); !errors.Is(err, session.ErrSessionLeaseNotHeld) {
		t.Fatalf("expected direct recovery without lease to fail, got %v", err)
	}

	out, err := rt.RecoverClaimedSession(context.Background(), sess.SessionID, claimed.LeaseID)
	if err != nil {
		t.Fatalf("recover claimed session: %v", err)
	}
	if out.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected recovered session to complete, got %#v", out.Session)
	}
}
