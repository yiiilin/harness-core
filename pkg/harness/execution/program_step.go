package execution

import (
	"encoding/json"

	"github.com/yiiilin/harness-core/pkg/harness/plan"
)

const (
	StepMetadataProgramKey              = "execution_program"
	StepMetadataProgramInputBindingsKey = "execution_program_input_bindings"
)

func AttachProgram(step plan.StepSpec, program Program) plan.StepSpec {
	if step.Metadata == nil {
		step.Metadata = map[string]any{}
	}
	step.Metadata[StepMetadataProgramKey] = program
	return step
}

func ProgramFromStep(step plan.StepSpec) (*Program, bool) {
	if len(step.Metadata) == 0 {
		return nil, false
	}
	raw, ok := step.Metadata[StepMetadataProgramKey]
	if !ok || raw == nil {
		return nil, false
	}
	switch typed := raw.(type) {
	case Program:
		program := typed
		return &program, true
	case *Program:
		if typed == nil {
			return nil, false
		}
		program := *typed
		return &program, true
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, false
	}
	var program Program
	if err := json.Unmarshal(data, &program); err != nil {
		return nil, false
	}
	return &program, true
}

func AttachProgramInputBindings(step plan.StepSpec, bindings []ProgramInputBinding) plan.StepSpec {
	if len(bindings) == 0 {
		return step
	}
	if step.Metadata == nil {
		step.Metadata = map[string]any{}
	}
	step.Metadata[StepMetadataProgramInputBindingsKey] = bindings
	return step
}

func ProgramInputBindingsFromStep(step plan.StepSpec) ([]ProgramInputBinding, bool) {
	if len(step.Metadata) == 0 {
		return nil, false
	}
	raw, ok := step.Metadata[StepMetadataProgramInputBindingsKey]
	if !ok || raw == nil {
		return nil, false
	}
	switch typed := raw.(type) {
	case []ProgramInputBinding:
		return append([]ProgramInputBinding(nil), typed...), true
	case []*ProgramInputBinding:
		out := make([]ProgramInputBinding, 0, len(typed))
		for _, item := range typed {
			if item == nil {
				continue
			}
			out = append(out, *item)
		}
		return out, true
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, false
	}
	var bindings []ProgramInputBinding
	if err := json.Unmarshal(data, &bindings); err != nil {
		return nil, false
	}
	return bindings, true
}
