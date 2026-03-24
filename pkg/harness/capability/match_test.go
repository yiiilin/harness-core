package capability_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/capability"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
)

func TestRequestMarshalOmitsEmptyRequirements(t *testing.T) {
	data, err := json.Marshal(capability.Request{
		Action: action.Spec{ToolName: "shell.exec"},
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	if containsJSONField(data, "requirements") {
		t.Fatalf("expected empty requirements to be omitted, got %s", data)
	}
}

func TestUnsupportedMatchMarshalOmitsResolution(t *testing.T) {
	registry := tool.NewRegistry()
	registry.Register(tool.Definition{
		ToolName:       "shell.exec",
		Version:        "v1",
		CapabilityType: "executor",
		RiskLevel:      tool.RiskMedium,
		Enabled:        false,
	}, noopCapabilityHandler{})

	match, err := (capability.RegistryResolver{Registry: registry}).Match(context.Background(), capability.Request{
		Action: action.Spec{ToolName: "shell.exec"},
	})
	if err != nil {
		t.Fatalf("match capability: %v", err)
	}

	data, err := json.Marshal(match)
	if err != nil {
		t.Fatalf("marshal match: %v", err)
	}
	if containsJSONField(data, "resolution") {
		t.Fatalf("expected unsupported match to omit empty resolution, got %s", data)
	}
}

func TestUnsupportedReasonsUseStableToolVersionMetadataKey(t *testing.T) {
	registry := tool.NewRegistry()
	registry.Register(tool.Definition{
		ToolName:       "shell.exec",
		Version:        "v1",
		CapabilityType: "executor",
		RiskLevel:      tool.RiskMedium,
		Enabled:        true,
	}, noopCapabilityHandler{})

	match, err := (capability.RegistryResolver{Registry: registry}).Match(context.Background(), capability.Request{
		Action: action.Spec{ToolName: "shell.exec"},
		Requirements: &capability.SupportRequirements{
			MultiTargetFanout: true,
		},
	})
	if err != nil {
		t.Fatalf("match capability: %v", err)
	}
	if len(match.Reasons) != 1 {
		t.Fatalf("expected one reason, got %#v", match.Reasons)
	}
	if _, ok := match.Reasons[0].Metadata["tool_version"]; !ok {
		t.Fatalf("expected tool_version metadata key, got %#v", match.Reasons[0].Metadata)
	}
	if _, ok := match.Reasons[0].Metadata["version"]; ok {
		t.Fatalf("did not expect version metadata key, got %#v", match.Reasons[0].Metadata)
	}
}

type noopCapabilityHandler struct{}

func (noopCapabilityHandler) Invoke(_ context.Context, _ map[string]any) (action.Result, error) {
	return action.Result{OK: true}, nil
}

func containsJSONField(data []byte, field string) bool {
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		return false
	}
	_, ok := decoded[field]
	return ok
}
