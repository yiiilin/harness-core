package execution

type AttachmentInputKind string
type AttachmentMaterialization string

const (
	AttachmentInputText        AttachmentInputKind = "text"
	AttachmentInputBytes       AttachmentInputKind = "bytes"
	AttachmentInputArtifactRef AttachmentInputKind = "artifact_ref"

	AttachmentMaterializeNone     AttachmentMaterialization = "none"
	AttachmentMaterializeTempFile AttachmentMaterialization = "temp_file"
)

type AttachmentInput struct {
	AttachmentID string                    `json:"attachment_id,omitempty"`
	Name         string                    `json:"name,omitempty"`
	MediaType    string                    `json:"media_type,omitempty"`
	Kind         AttachmentInputKind       `json:"kind"`
	Text         string                    `json:"text,omitempty"`
	Bytes        []byte                    `json:"bytes,omitempty"`
	ArtifactID   string                    `json:"artifact_id,omitempty"`
	Materialize  AttachmentMaterialization `json:"materialize,omitempty"`
	Metadata     map[string]any            `json:"metadata,omitempty"`
}

func (a AttachmentInput) HasInlinePayload() bool {
	return a.Text != "" || len(a.Bytes) > 0
}

func (a AttachmentInput) ReferencesArtifact() bool {
	return a.ArtifactID != "" || a.Kind == AttachmentInputArtifactRef
}
