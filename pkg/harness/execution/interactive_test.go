package execution_test

import (
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/execution"
)

func TestInteractiveRuntimeFromHandle(t *testing.T) {
	exitCode := 7
	handle := execution.RuntimeHandle{
		HandleID:  "hdl_interactive",
		SessionID: "session-1",
		Status:    execution.RuntimeHandleActive,
		Metadata: map[string]any{
			execution.InteractiveMetadataKeyEnabled:             true,
			execution.InteractiveMetadataKeySupportsView:        true,
			execution.InteractiveMetadataKeySupportsWrite:       true,
			execution.InteractiveMetadataKeySupportsClose:       true,
			execution.InteractiveMetadataKeyNextOffset:          int64(41),
			execution.InteractiveMetadataKeyStatus:              "active",
			execution.InteractiveMetadataKeyStatusReason:        "remote session active",
			execution.InteractiveMetadataKeyExitCode:            exitCode,
			execution.InteractiveMetadataKeySnapshotArtifactID:  "art_snapshot_1",
			execution.InteractiveMetadataKeyLastOperationKind:   string(execution.InteractiveOperationWrite),
			execution.InteractiveMetadataKeyLastOperationAt:     int64(1234),
			execution.InteractiveMetadataKeyLastOperationBytes:  int64(12),
			execution.InteractiveMetadataKeyLastOperationOffset: int64(29),
			execution.TargetMetadataKeyID:                       "target-1",
			execution.TargetMetadataKeyKind:                     "host",
		},
	}

	runtime, ok := execution.InteractiveRuntimeFromHandle(handle)
	if !ok {
		t.Fatalf("expected interactive runtime")
	}
	if runtime.Handle.HandleID != "hdl_interactive" {
		t.Fatalf("unexpected handle: %#v", runtime)
	}
	if !runtime.Capabilities.View || !runtime.Capabilities.Write || !runtime.Capabilities.Close {
		t.Fatalf("expected interactive capabilities, got %#v", runtime.Capabilities)
	}
	if runtime.Observation.NextOffset != 41 || runtime.Observation.Status != "active" || runtime.Observation.StatusReason != "remote session active" {
		t.Fatalf("unexpected observation: %#v", runtime.Observation)
	}
	if runtime.Observation.ExitCode == nil || *runtime.Observation.ExitCode != 7 {
		t.Fatalf("expected exit code 7, got %#v", runtime.Observation.ExitCode)
	}
	if runtime.Observation.Snapshot.ArtifactID != "art_snapshot_1" {
		t.Fatalf("expected snapshot artifact ref, got %#v", runtime.Observation.Snapshot)
	}
	if runtime.LastOperation.Kind != execution.InteractiveOperationWrite || runtime.LastOperation.At != 1234 || runtime.LastOperation.Bytes != 12 || runtime.LastOperation.Offset != 29 {
		t.Fatalf("unexpected last operation: %#v", runtime.LastOperation)
	}
	if runtime.Target.TargetID != "target-1" || runtime.Target.Kind != "host" {
		t.Fatalf("expected target ref, got %#v", runtime.Target)
	}
}

func TestInteractiveRuntimesFromHandlesFiltersNonInteractive(t *testing.T) {
	items := execution.InteractiveRuntimesFromHandles([]execution.RuntimeHandle{
		{
			HandleID: "hdl_interactive",
			Metadata: map[string]any{execution.InteractiveMetadataKeyEnabled: true},
		},
		{
			HandleID: "hdl_plain",
			Metadata: map[string]any{"provider": "plain"},
		},
	})
	if len(items) != 1 || items[0].Handle.HandleID != "hdl_interactive" {
		t.Fatalf("expected only interactive handle to be projected, got %#v", items)
	}
}
