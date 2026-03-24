package runtime_test

import (
	"context"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/execution"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
)

func TestInteractiveRuntimeReadAndUpdateSurface(t *testing.T) {
	rt := hruntime.New(hruntime.Options{})
	sess := mustCreateSession(t, rt, "interactive runtime", "project typed interactive runtime state")

	_, err := rt.RuntimeHandles.Create(execution.RuntimeHandle{
		HandleID:  "hdl_interactive_runtime",
		SessionID: sess.SessionID,
		Status:    execution.RuntimeHandleActive,
		Metadata: map[string]any{
			execution.InteractiveMetadataKeyEnabled:       true,
			execution.InteractiveMetadataKeySupportsView:  true,
			execution.InteractiveMetadataKeySupportsWrite: true,
			execution.InteractiveMetadataKeySupportsClose: true,
			execution.InteractiveMetadataKeyStatus:        "active",
			execution.InteractiveMetadataKeyNextOffset:    int64(0),
		},
	})
	if err != nil {
		t.Fatalf("seed interactive runtime handle: %v", err)
	}

	projected, err := rt.GetInteractiveRuntime("hdl_interactive_runtime")
	if err != nil {
		t.Fatalf("get interactive runtime: %v", err)
	}
	if !projected.Capabilities.View || !projected.Capabilities.Write || projected.Observation.NextOffset != 0 {
		t.Fatalf("unexpected projected interactive runtime: %#v", projected)
	}

	exitCode := 0
	updated, err := rt.UpdateInteractiveRuntime(context.Background(), "hdl_interactive_runtime", hruntime.InteractiveRuntimeUpdate{
		Observation: &execution.InteractiveObservation{
			NextOffset:   99,
			Closed:       true,
			ExitCode:     &exitCode,
			Status:       "closed",
			StatusReason: "remote process exited",
			Snapshot: execution.InteractiveSnapshot{
				ArtifactID: "art_snapshot_updated",
			},
		},
		LastOperation: &execution.InteractiveOperation{
			Kind:   execution.InteractiveOperationView,
			At:     5000,
			Offset: 88,
		},
	})
	if err != nil {
		t.Fatalf("update interactive runtime: %v", err)
	}
	if updated.Observation.NextOffset != 99 || !updated.Observation.Closed || updated.Observation.ExitCode == nil || *updated.Observation.ExitCode != 0 {
		t.Fatalf("unexpected updated observation: %#v", updated)
	}
	if updated.Observation.Snapshot.ArtifactID != "art_snapshot_updated" {
		t.Fatalf("expected snapshot artifact to persist, got %#v", updated.Observation.Snapshot)
	}
	if updated.LastOperation.Kind != execution.InteractiveOperationView || updated.LastOperation.Offset != 88 {
		t.Fatalf("expected last operation metadata, got %#v", updated.LastOperation)
	}

	all, err := rt.ListInteractiveRuntimes(sess.SessionID)
	if err != nil {
		t.Fatalf("list interactive runtimes: %v", err)
	}
	if len(all) != 1 || all[0].Handle.HandleID != "hdl_interactive_runtime" {
		t.Fatalf("expected one interactive runtime projection, got %#v", all)
	}
}

func TestBlockedRuntimeProjectionIncludesWaitAndInteractiveState(t *testing.T) {
	rt := newBlockedRuntimeTestService()
	attached, initial := seedApprovalBlockedSession(t, rt, "blocked runtime projection", "project blocked wait state")

	attempts := mustListAttempts(t, rt, attached.SessionID)
	if len(attempts) != 1 {
		t.Fatalf("expected one blocked attempt, got %#v", attempts)
	}
	_, err := rt.RuntimeHandles.Create(execution.RuntimeHandle{
		HandleID:  "hdl_blocked_projection",
		SessionID: attached.SessionID,
		TaskID:    attached.TaskID,
		AttemptID: attempts[0].AttemptID,
		CycleID:   attempts[0].CycleID,
		Status:    execution.RuntimeHandleActive,
		Metadata: map[string]any{
			execution.InteractiveMetadataKeyEnabled:      true,
			execution.InteractiveMetadataKeySupportsView: true,
			execution.InteractiveMetadataKeyStatus:       "active",
			execution.InteractiveMetadataKeyNextOffset:   int64(12),
		},
	})
	if err != nil {
		t.Fatalf("seed blocked interactive handle: %v", err)
	}

	view, err := rt.GetBlockedRuntimeProjection(attached.SessionID)
	if err != nil {
		t.Fatalf("get blocked runtime projection: %v", err)
	}
	if view.Runtime.BlockedRuntimeID != initial.Execution.PendingApproval.ApprovalID {
		t.Fatalf("unexpected blocked runtime projection identity: %#v", view)
	}
	if view.Wait.WaitingFor != "approval" || view.Wait.StepID == "" {
		t.Fatalf("expected wait projection to point at approval step, got %#v", view.Wait)
	}
	if len(view.InteractiveRuntimes) != 1 || view.InteractiveRuntimes[0].Handle.HandleID != "hdl_blocked_projection" {
		t.Fatalf("expected blocked projection to expose interactive runtime state, got %#v", view)
	}

	items, err := rt.ListBlockedRuntimeProjections()
	if err != nil {
		t.Fatalf("list blocked runtime projections: %v", err)
	}
	if len(items) != 1 || items[0].Runtime.BlockedRuntimeID != view.Runtime.BlockedRuntimeID {
		t.Fatalf("expected list projection to contain the same blocked runtime, got %#v", items)
	}

	byApproval, err := rt.GetBlockedRuntimeProjectionByApproval(initial.Execution.PendingApproval.ApprovalID)
	if err != nil {
		t.Fatalf("get blocked runtime projection by approval: %v", err)
	}
	if byApproval.Runtime.BlockedRuntimeID != view.Runtime.BlockedRuntimeID {
		t.Fatalf("expected approval lookup to return same blocked projection, got %#v", byApproval)
	}
}
