package main

import (
	"context"
	"slices"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

func TestRunProgramGraphDemo(t *testing.T) {
	result, err := RunProgramGraphDemo(context.Background())
	if err != nil {
		t.Fatalf("run program graph demo: %v", err)
	}

	if result.SessionID == "" || result.Phase != session.PhaseComplete {
		t.Fatalf("expected completed session, got %#v", result)
	}
	if result.Aggregate.Status != execution.AggregateStatusCompleted || result.Aggregate.Completed != 2 || result.Aggregate.Failed != 0 {
		t.Fatalf("expected completed aggregate summary, got %#v", result.Aggregate)
	}
	if result.ArtifactRef.ArtifactID == "" || result.ArtifactRef.Kind != "action_result" {
		t.Fatalf("expected artifact-ref consumer to receive action_result ref, got %#v", result.ArtifactRef)
	}
	if !slices.Equal(result.DispatchOutputs, []string{"alpha:ship v1.0.1", "beta:ship v1.0.1"}) {
		t.Fatalf("unexpected fanout outputs: %#v", result.DispatchOutputs)
	}
	if len(result.TargetSliceCycleIDs) != 2 {
		t.Fatalf("expected target-sliced replay cycles for fanout steps, got %#v", result.TargetSliceCycleIDs)
	}
}
