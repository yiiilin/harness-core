package execution_test

import (
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/execution"
)

func TestApplyTargetMetadataAndExtractRef(t *testing.T) {
	metadata := execution.ApplyTargetMetadata(nil, execution.Target{
		TargetID:    "target-1",
		Kind:        "host",
		DisplayName: "host-1",
	}, 1, 2)

	ref, ok := execution.TargetRefFromMetadata(metadata)
	if !ok {
		t.Fatalf("expected target ref from metadata")
	}
	if ref.TargetID != "target-1" || ref.Kind != "host" {
		t.Fatalf("unexpected target ref: %#v", ref)
	}
	if metadata[execution.TargetMetadataKeyIndex] != 1 || metadata[execution.TargetMetadataKeyCount] != 2 {
		t.Fatalf("unexpected target metadata: %#v", metadata)
	}
}

func TestTargetArgValue(t *testing.T) {
	arg := execution.TargetArgValue(execution.Target{
		TargetID:    "target-1",
		Kind:        "host",
		DisplayName: "host-1",
	})
	if got, _ := arg[execution.TargetMetadataKeyID].(string); got != "target-1" {
		t.Fatalf("unexpected target arg value: %#v", arg)
	}
}
