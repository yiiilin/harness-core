package execution_test

import (
	"encoding/json"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
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

	runtimeHandleBinding := execution.ProgramInputBinding{
		Name: "runtime",
		Kind: execution.ProgramInputBindingRuntimeHandleRef,
		RuntimeHandle: &execution.RuntimeHandleRef{
			HandleID: "hdl_1",
			Kind:     "pty",
			Status:   execution.RuntimeHandleActive,
			Version:  3,
		},
	}
	if !runtimeHandleBinding.ReferencesRuntimeHandle() {
		t.Fatalf("expected runtime handle ref binding")
	}
	if runtimeHandleBinding.ReferencesOutput() {
		t.Fatalf("did not expect output ref for runtime handle binding")
	}
	if runtimeHandleBinding.HasAttachmentInput() {
		t.Fatalf("did not expect attachment input for runtime handle binding")
	}
}

func TestRuntimeHandleRefFromHandle(t *testing.T) {
	handle := execution.RuntimeHandle{
		HandleID: "hdl_runtime",
		Kind:     "pty",
		Value:    "pty-session-1",
		Status:   execution.RuntimeHandleActive,
		Version:  7,
	}

	ref := execution.RuntimeHandleRefFromHandle(handle)
	if ref.HandleID != handle.HandleID || ref.Kind != handle.Kind {
		t.Fatalf("expected runtime handle ref to preserve identity fields, got %#v", ref)
	}
	if ref.Status != handle.Status || ref.Version != handle.Version {
		t.Fatalf("expected runtime handle ref to preserve status/version, got %#v", ref)
	}
}

func TestProgramInputBindingsFromStepRoundTripsRuntimeHandleRefsFromGenericMetadata(t *testing.T) {
	step := execution.AttachProgramInputBindings(plan.StepSpec{StepID: "step_runtime_ref"}, []execution.ProgramInputBinding{{
		Name: "runtime",
		Kind: execution.ProgramInputBindingRuntimeHandleRef,
		RuntimeHandle: &execution.RuntimeHandleRef{
			HandleID: "hdl_roundtrip",
			StepID:   "node_start",
			Kind:     "pty",
			Status:   execution.RuntimeHandleActive,
			Version:  2,
		},
	}})

	data, err := json.Marshal(step.Metadata)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}

	var generic map[string]any
	if err := json.Unmarshal(data, &generic); err != nil {
		t.Fatalf("unmarshal generic metadata: %v", err)
	}

	roundTripped := plan.StepSpec{StepID: step.StepID, Metadata: generic}
	bindings, ok := execution.ProgramInputBindingsFromStep(roundTripped)
	if !ok || len(bindings) != 1 {
		t.Fatalf("expected one runtime handle binding after round trip, got %#v", roundTripped.Metadata)
	}
	if bindings[0].Kind != execution.ProgramInputBindingRuntimeHandleRef || bindings[0].RuntimeHandle == nil {
		t.Fatalf("expected typed runtime handle binding after round trip, got %#v", bindings[0])
	}
	if bindings[0].RuntimeHandle.HandleID != "hdl_roundtrip" || bindings[0].RuntimeHandle.StepID != "node_start" {
		t.Fatalf("expected runtime handle ref identity after round trip, got %#v", bindings[0].RuntimeHandle)
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
