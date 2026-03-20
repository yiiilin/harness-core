package runtime_test

import (
	"context"
	"testing"

	"github.com/yiiilin/harness-core/internal/postgrestest"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

func TestRecoveryReadPathAcrossPostgresRuntimeReinit(t *testing.T) {
	pg := postgrestest.Start(t)

	opts := hruntime.Options{}
	hruntime.RegisterBuiltins(&opts)

	rt1, db1 := pg.OpenService(t, opts)
	defer db1.Close()

	sess := rt1.CreateSession("durable recovery", "mark in-flight and recover later")
	if _, err := rt1.MarkSessionInFlight(context.Background(), sess.SessionID, "step_1"); err != nil {
		t.Fatalf("mark in-flight: %v", err)
	}
	if _, err := rt1.MarkSessionInterrupted(context.Background(), sess.SessionID); err != nil {
		t.Fatalf("mark interrupted: %v", err)
	}

	rt2, db2 := pg.OpenService(t, opts)
	defer db2.Close()

	items := rt2.ListRecoverableSessions()
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
