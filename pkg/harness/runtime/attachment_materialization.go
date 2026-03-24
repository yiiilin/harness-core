package runtime

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/yiiilin/harness-core/pkg/harness/execution"
)

// LocalTempFileMaterializer materializes attachment payloads into local temp
// files and returns the filesystem path.
type LocalTempFileMaterializer struct {
	Dir string
}

func (m LocalTempFileMaterializer) Materialize(_ context.Context, request AttachmentMaterializeRequest) (any, error) {
	if request.Input.Materialize != execution.AttachmentMaterializeTempFile {
		return nil, fmt.Errorf("%w: attachment materialization %q", ErrProgramAttachmentUnsupported, request.Input.Materialize)
	}
	data, pattern, err := attachmentMaterializationBytes(request)
	if err != nil {
		return nil, err
	}
	file, err := os.CreateTemp(strings.TrimSpace(m.Dir), pattern)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		_ = os.Remove(file.Name())
		return nil, err
	}
	return file.Name(), nil
}

func attachmentMaterializationBytes(request AttachmentMaterializeRequest) ([]byte, string, error) {
	switch request.Input.Kind {
	case execution.AttachmentInputText:
		return []byte(request.Input.Text), attachmentTempFilePattern(request.Input.Name, "attachment"), nil
	case execution.AttachmentInputBytes:
		return append([]byte(nil), request.Input.Bytes...), attachmentTempFilePattern(request.Input.Name, "attachment"), nil
	case execution.AttachmentInputArtifactRef:
		if request.Artifact == nil {
			return nil, "", fmt.Errorf("%w: artifact materialization missing artifact payload", ErrProgramBindingResolveFailed)
		}
		data, ok := bytesFromProgramValue(request.Artifact.Payload["bytes"])
		if ok {
			return data, attachmentTempFilePattern(firstNonEmptyProgramValue(request.Input.Name, request.Artifact.Name), "artifact"), nil
		}
		if text, ok := stringFromProgramValue(request.Artifact.Payload["text"]); ok {
			return []byte(text), attachmentTempFilePattern(firstNonEmptyProgramValue(request.Input.Name, request.Artifact.Name), "artifact"), nil
		}
		if rawData, ok := request.Artifact.Payload["data"]; ok {
			if nested, ok := rawData.(map[string]any); ok {
				if data, ok := bytesFromProgramValue(nested["bytes"]); ok {
					return data, attachmentTempFilePattern(firstNonEmptyProgramValue(request.Input.Name, request.Artifact.Name), "artifact"), nil
				}
				if text, ok := stringFromProgramValue(nested["text"]); ok {
					return []byte(text), attachmentTempFilePattern(firstNonEmptyProgramValue(request.Input.Name, request.Artifact.Name), "artifact"), nil
				}
				if text, ok := stringFromProgramValue(nested["stdout"]); ok {
					return []byte(text), attachmentTempFilePattern(firstNonEmptyProgramValue(request.Input.Name, request.Artifact.Name), "artifact"), nil
				}
			}
		}
		return nil, "", fmt.Errorf("%w: artifact %q is not materializable as bytes", ErrProgramBindingResolveFailed, request.Artifact.ArtifactID)
	default:
		return nil, "", fmt.Errorf("%w: attachment kind %q", ErrProgramAttachmentUnsupported, request.Input.Kind)
	}
}

func attachmentTempFilePattern(name, fallback string) string {
	base := strings.TrimSpace(name)
	if base == "" {
		base = fallback
	}
	base = strings.ReplaceAll(base, "/", "_")
	base = strings.ReplaceAll(base, "\\", "_")
	return base + "-*"
}
