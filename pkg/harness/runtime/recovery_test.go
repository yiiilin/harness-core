package runtime_test

import (
	"context"
	"testing"

	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
)

func TestRecoveryReadPathAcrossRuntimeReinit(t *testing.T) {
	opts := hruntime.Options{}
	hruntime.RegisterBuiltins(&opts)
	rt1 := hruntime.New(opts)
	sess := rt1.CreateSession("recovery", "mark in-flight and recover later")
	_, err := rt1.MarkSessionInFlight(context.Background(), sess.SessionID, "step_1")
	if err != nil {
		t.Fatalf("mark in-flight: %v", err)
	}

	// Simulate restart by constructing a new runtime with the same backing stores.
	rt2 := hruntime.New(opts)
	items := rt2.ListRecoverableSessions()
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
