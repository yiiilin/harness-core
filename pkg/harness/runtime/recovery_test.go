package runtime_test

import (
	"context"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/persistence"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

func TestRecoveryReadPathAcrossRuntimeReinit(t *testing.T) {
	opts := hruntime.Options{}
	hruntime.RegisterBuiltins(&opts)
	rt1 := hruntime.New(opts)
	sess := mustCreateSession(t, rt1, "recovery", "mark in-flight and recover later")
	_, err := rt1.MarkSessionInFlight(context.Background(), sess.SessionID, "step_1")
	if err != nil {
		t.Fatalf("mark in-flight: %v", err)
	}

	// Simulate restart by constructing a new runtime with the same backing stores.
	rt2 := hruntime.New(opts)
	items := mustListRecoverableSessions(t, rt2)
	if len(items) != 1 {
		t.Fatalf("expected 1 recoverable session, got %d", len(items))
	}
	if items[0].SessionID != sess.SessionID {
		t.Fatalf("expected session %s, got %s", sess.SessionID, items[0].SessionID)
	}
	if items[0].InFlightStepID != "step_1" {
		t.Fatalf("expected in-flight step step_1, got %s", items[0].InFlightStepID)
	}
}

func TestRecoveryStateTransitionsUseRunnerBoundary(t *testing.T) {
	sessions := session.NewMemoryStore()
	runner := &countingRunner{repos: persistence.RepositorySet{Sessions: sessions}}
	rt := hruntime.New(hruntime.Options{
		Sessions: sessions,
		Runner:   runner,
	})

	sess := mustCreateSession(t, rt, "runner recovery", "recovery updates should use runner")
	baselineCalls := runner.calls

	if _, err := rt.MarkSessionInFlight(context.Background(), sess.SessionID, "step_1"); err != nil {
		t.Fatalf("mark in-flight: %v", err)
	}
	if _, err := rt.MarkSessionInterrupted(context.Background(), sess.SessionID); err != nil {
		t.Fatalf("mark interrupted: %v", err)
	}

	if runner.calls < baselineCalls+2 {
		t.Fatalf("expected recovery writes to use runner, got %d calls from baseline %d", runner.calls, baselineCalls)
	}
}
