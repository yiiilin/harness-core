package runtime

import "github.com/yiiilin/harness-core/pkg/harness/execution"

func applyCompiledProgramBindings(args map[string]any, bindings []execution.ProgramInputBinding) ([]execution.ProgramInputBinding, error) {
	unresolved := make([]execution.ProgramInputBinding, 0, len(bindings))
	for _, binding := range bindings {
		switch binding.Kind {
		case "", execution.ProgramInputBindingLiteral:
			args[binding.Name] = binding.Value
		case execution.ProgramInputBindingOutputRef:
			unresolved = append(unresolved, binding)
		case execution.ProgramInputBindingAttachment:
			if binding.Attachment == nil {
				return nil, ErrProgramAttachmentUnsupported
			}
			if binding.Attachment.Materialize != "" && binding.Attachment.Materialize != execution.AttachmentMaterializeNone {
				unresolved = append(unresolved, binding)
				continue
			}
			switch binding.Attachment.Kind {
			case execution.AttachmentInputText:
				args[binding.Name] = binding.Attachment.Text
			case execution.AttachmentInputBytes:
				args[binding.Name] = append([]byte(nil), binding.Attachment.Bytes...)
			case execution.AttachmentInputArtifactRef:
				unresolved = append(unresolved, binding)
			default:
				return nil, ErrProgramAttachmentUnsupported
			}
		case execution.ProgramInputBindingRuntimeHandleRef:
			if binding.RuntimeHandle == nil {
				return nil, ErrProgramInputBindingUnsupported
			}
			unresolved = append(unresolved, binding)
		default:
			return nil, ErrProgramInputBindingUnsupported
		}
	}
	return unresolved, nil
}
