package execution_test

import (
	"encoding/json"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/execution"
)

func TestAttachmentInputHelpers(t *testing.T) {
	textInput := execution.AttachmentInput{
		Kind: execution.AttachmentInputText,
		Text: "hello",
	}
	if !textInput.HasInlinePayload() {
		t.Fatalf("expected text attachment to report inline payload")
	}
	if textInput.ReferencesArtifact() {
		t.Fatalf("did not expect text attachment to reference artifact")
	}

	artifactRef := execution.AttachmentInput{
		Kind:       execution.AttachmentInputArtifactRef,
		ArtifactID: "art_123",
	}
	if artifactRef.HasInlinePayload() {
		t.Fatalf("did not expect artifact ref to report inline payload")
	}
	if !artifactRef.ReferencesArtifact() {
		t.Fatalf("expected artifact ref to report artifact reference")
	}
}

func TestAttachmentInputMarshalOmitsEmptyOptionalFields(t *testing.T) {
	data, err := json.Marshal(execution.AttachmentInput{Kind: execution.AttachmentInputText})
	if err != nil {
		t.Fatalf("marshal attachment input: %v", err)
	}
	if string(data) != "{\"kind\":\"text\"}" {
		t.Fatalf("unexpected attachment json: %s", data)
	}
}
