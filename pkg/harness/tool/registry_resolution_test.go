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
