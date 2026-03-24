package execution_test

import (
	"encoding/json"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
)

func TestProgramNodeHelpers(t *testing.T) {
	node := execution.ProgramNode{
		NodeID:     "node_http",
		DependsOn:  []string{"node_prepare"},
		Targeting:  &execution.TargetSelection{Mode: execution.TargetSelectionFanoutExplicit},
		InputBinds: []execution.ProgramInputBinding{{Name: "body", Kind: execution.ProgramInputBindingLiteral, Value: map[string]any{"ok": true}}},
	}
	if !node.HasDependencies() {
		t.Fatalf("expected dependencies")
	}
	if !node.MultiTargetRequested() {
		t.Fatalf("expected multi-target helper to follow targeting")
	}
	if !node.HasInputBindings() {
		t.Fatalf("expected input bindings")
	}
}

func TestProgramInputBindingHelpers(t *testing.T) {
	outputBinding := execution.ProgramInputBinding{
		Name: "stdout",
		Kind: execution.ProgramInputBindingOutputRef,
		Ref:  &execution.OutputRef{Kind: execution.OutputRefText, StepID: "step_1"},
	}
	if !outputBinding.ReferencesOutput() {
		t.Fatalf("expected output ref binding")
	}
	if outputBinding.HasAttachmentInput() {
		t.Fatalf("did not expect attachment input")
	}

	attachmentBinding := execution.ProgramInputBinding{
		Name:       "payload",
		Kind:       execution.ProgramInputBindingAttachment,
		Attachment: &execution.AttachmentInput{Kind: execution.AttachmentInputBytes, Bytes: []byte("abc")},
	}
	if !attachmentBinding.HasAttachmentInput() {
		t.Fatalf("expected attachment input")
	}
	if attachmentBinding.ReferencesOutput() {
		t.Fatalf("did not expect output ref")
	}
}

func TestProgramMarshalOmitsEmptyOptionalFields(t *testing.T) {
	program := execution.Program{
		Nodes: []execution.ProgramNode{
			{
				NodeID: "node_exec",
				Action: action.Spec{ToolName: "exec_command"},
			},
		},
	}
	data, err := json.Marshal(program)
	if err != nil {
		t.Fatalf("marshal program: %v", err)
	}
	expected := "{\"nodes\":[{\"node_id\":\"node_exec\",\"action\":{\"tool_name\":\"exec_command\"}}]}"
	if string(data) != expected {
		t.Fatalf("unexpected program json: %s", data)
	}
}
