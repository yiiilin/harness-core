package execution_test

import (
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/execution"
)

func TestTargetSliceHelpers(t *testing.T) {
	slice := execution.TargetSlice{
		Target: execution.TargetRef{TargetID: "target-1", Kind: "host"},
		Actions: []execution.ActionRecord{
			{ActionID: "action-1"},
		},
	}
	if !slice.HasTarget() {
		t.Fatalf("expected target ref")
	}
	if !slice.HasExecutionFacts() {
		t.Fatalf("expected execution facts")
	}
}

func TestBlockedRuntimeWaitHelpers(t *testing.T) {
	wait := execution.BlockedRuntimeWait{
		Scope:    execution.BlockedRuntimeWaitAction,
		ActionID: "action-1",
		Target:   execution.TargetRef{TargetID: "target-1"},
	}
	if !wait.ReferencesAction() {
		t.Fatalf("expected action reference")
	}
	if !wait.ReferencesTarget() {
		t.Fatalf("expected target reference")
	}
}

func TestBlockedRuntimeProjectionHelpers(t *testing.T) {
	view := execution.BlockedRuntimeProjection{
		Runtime: execution.BlockedRuntime{
			BlockedRuntimeID: "blocked-1",
			SessionID:        "session-1",
		},
		TargetSlices: []execution.TargetSlice{{Target: execution.TargetRef{TargetID: "target-1"}}},
	}
	if !view.HasTargetSlices() {
		t.Fatalf("expected target slices")
	}
	if view.Runtime.BlockedRuntimeID != "blocked-1" {
		t.Fatalf("unexpected runtime id: %s", view.Runtime.BlockedRuntimeID)
	}
}
