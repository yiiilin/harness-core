package execution_test

import (
	"encoding/json"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/execution"
)

func TestOutputRefHelpers(t *testing.T) {
	artifactRef := execution.OutputRef{
		Kind:       execution.OutputRefArtifact,
		ArtifactID: "art_123",
	}
	if !artifactRef.ReferencesArtifact() {
		t.Fatalf("expected artifact output ref to reference artifact")
	}
	if artifactRef.ReferencesAttachment() {
		t.Fatalf("did not expect artifact output ref to reference attachment")
	}

	attachmentRef := execution.OutputRef{
		Kind:         execution.OutputRefAttachment,
		AttachmentID: "att_123",
	}
	if attachmentRef.ReferencesArtifact() {
		t.Fatalf("did not expect attachment output ref to reference artifact")
	}
	if !attachmentRef.ReferencesAttachment() {
		t.Fatalf("expected attachment output ref to reference attachment")
	}
}

func TestOutputRefMarshalOmitsEmptyOptionalFields(t *testing.T) {
	data, err := json.Marshal(execution.OutputRef{Kind: execution.OutputRefText})
	if err != nil {
		t.Fatalf("marshal output ref: %v", err)
	}
	if string(data) != "{\"kind\":\"text\"}" {
		t.Fatalf("unexpected output ref json: %s", data)
	}
}
