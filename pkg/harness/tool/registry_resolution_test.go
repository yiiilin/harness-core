package tool_test

import (
	"context"
	"errors"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/capability"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
)

type noopHandler struct{}

func (noopHandler) Invoke(_ context.Context, _ map[string]any) (action.Result, error) {
	return action.Result{OK: true}, nil
}

func TestRegistryResolverRejectsDisabledCapability(t *testing.T) {
	registry := tool.NewRegistry()
	registry.Register(tool.Definition{
		ToolName:       "shell.exec",
		Version:        "v1",
		CapabilityType: "executor",
		RiskLevel:      tool.RiskMedium,
		Enabled:        false,
	}, noopHandler{})

	resolver := capability.RegistryResolver{Registry: registry}
	_, err := resolver.Resolve(context.Background(), capability.Request{
		Action: action.Spec{ToolName: "shell.exec"},
	})
	if !errors.Is(err, capability.ErrCapabilityDisabled) {
		t.Fatalf("expected ErrCapabilityDisabled, got %v", err)
	}
}

func TestRegistryResolverSelectsRequestedVersion(t *testing.T) {
	registry := tool.NewRegistry()
	registry.Register(tool.Definition{
		ToolName:       "shell.exec",
		Version:        "v1",
		CapabilityType: "executor",
		RiskLevel:      tool.RiskLow,
		Enabled:        true,
	}, noopHandler{})
	registry.Register(tool.Definition{
		ToolName:       "shell.exec",
		Version:        "v2",
		CapabilityType: "executor",
		RiskLevel:      tool.RiskHigh,
		Enabled:        true,
	}, noopHandler{})

	resolver := capability.RegistryResolver{Registry: registry}
	resolution, err := resolver.Resolve(context.Background(), capability.Request{
		SessionID: "sess_1",
		TaskID:    "task_1",
		StepID:    "step_1",
		Action:    action.Spec{ToolName: "shell.exec", ToolVersion: "v2"},
	})
	if err != nil {
		t.Fatalf("resolve capability: %v", err)
	}
	if resolution.Snapshot.ToolName != "shell.exec" || resolution.Snapshot.Version != "v2" {
		t.Fatalf("unexpected snapshot: %#v", resolution.Snapshot)
	}
	if resolution.Definition.RiskLevel != tool.RiskHigh {
		t.Fatalf("expected v2 risk metadata, got %#v", resolution.Definition)
	}
}

func TestRegistryResolverMatchReturnsCapabilityReasonCodes(t *testing.T) {
	registry := tool.NewRegistry()
	registry.Register(tool.Definition{
		ToolName:       "shell.exec",
		Version:        "v1",
		CapabilityType: "executor",
		RiskLevel:      tool.RiskMedium,
		Enabled:        false,
	}, noopHandler{})

	resolver := capability.RegistryResolver{Registry: registry}
	match, err := resolver.Match(context.Background(), capability.Request{
		Action: action.Spec{ToolName: "shell.exec"},
	})
	if err != nil {
		t.Fatalf("match capability: %v", err)
	}
	if match.Supported {
		t.Fatalf("expected unsupported match, got %#v", match)
	}
	if len(match.Reasons) != 1 || match.Reasons[0].Code != capability.ReasonCapabilityDisabled {
		t.Fatalf("expected CAPABILITY_DISABLED reason, got %#v", match.Reasons)
	}
}

func TestRegistryResolverMatchReturnsFeatureUnsupportedReasons(t *testing.T) {
	registry := tool.NewRegistry()
	registry.Register(tool.Definition{
		ToolName:       "shell.exec",
		Version:        "v1",
		CapabilityType: "executor",
		RiskLevel:      tool.RiskMedium,
		Enabled:        true,
	}, noopHandler{})

	resolver := capability.RegistryResolver{Registry: registry}
	match, err := resolver.Match(context.Background(), capability.Request{
		Action: action.Spec{ToolName: "shell.exec"},
		Requirements: &capability.SupportRequirements{
			MultiTargetFanout:   true,
			PreplannedToolGraph: true,
			InteractiveReopen:   true,
			ArtifactInput:       true,
		},
	})
	if err != nil {
		t.Fatalf("match capability: %v", err)
	}
	if match.Supported {
		t.Fatalf("expected unsupported match, got %#v", match)
	}
	got := map[capability.UnsupportedReasonCode]bool{}
	for _, reason := range match.Reasons {
		got[reason.Code] = true
	}
	for _, code := range []capability.UnsupportedReasonCode{
		capability.ReasonMultiTargetFanoutUnsupported,
		capability.ReasonPreplannedToolGraphUnsupported,
		capability.ReasonInteractiveReopenUnsupported,
		capability.ReasonArtifactInputUnsupported,
	} {
		if !got[code] {
			t.Fatalf("expected reason code %q in %#v", code, match.Reasons)
		}
	}
}

func TestRegistryResolverMatchRespectsDefinitionSupportMetadata(t *testing.T) {
	registry := tool.NewRegistry()
	registry.Register(tool.Definition{
		ToolName:       "shell.exec",
		Version:        "v1",
		CapabilityType: "executor",
		RiskLevel:      tool.RiskMedium,
		Enabled:        true,
		Metadata: map[string]any{
			"supports_multi_target_fanout":   true,
			"supports_preplanned_tool_graph": true,
			"supports_interactive_reopen":    true,
			"supports_artifact_input":        true,
		},
	}, noopHandler{})

	resolver := capability.RegistryResolver{Registry: registry}
	match, err := resolver.Match(context.Background(), capability.Request{
		SessionID: "sess_1",
		TaskID:    "task_1",
		StepID:    "step_1",
		Action:    action.Spec{ToolName: "shell.exec"},
		Requirements: &capability.SupportRequirements{
			MultiTargetFanout:   true,
			PreplannedToolGraph: true,
			InteractiveReopen:   true,
			ArtifactInput:       true,
		},
	})
	if err != nil {
		t.Fatalf("match capability: %v", err)
	}
	if !match.Supported {
		t.Fatalf("expected supported match, got %#v", match)
	}
	if match.Resolution.Snapshot.ToolName != "shell.exec" {
		t.Fatalf("expected resolved snapshot, got %#v", match.Resolution)
	}
	if len(match.Reasons) != 0 {
		t.Fatalf("expected no unsupported reasons, got %#v", match.Reasons)
	}
}
