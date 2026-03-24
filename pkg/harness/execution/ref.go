package execution

type ArtifactRef struct {
	ArtifactID string `json:"artifact_id"`
	Name       string `json:"name,omitempty"`
	Kind       string `json:"kind,omitempty"`
}

type AttachmentRef struct {
	AttachmentID string `json:"attachment_id"`
	Name         string `json:"name,omitempty"`
	MediaType    string `json:"media_type,omitempty"`
}

type OutputRefKind string

const (
	OutputRefStructured OutputRefKind = "structured"
	OutputRefText       OutputRefKind = "text"
	OutputRefBytes      OutputRefKind = "bytes"
	OutputRefArtifact   OutputRefKind = "artifact"
	OutputRefAttachment OutputRefKind = "attachment"
)

type OutputRef struct {
	Kind         OutputRefKind  `json:"kind"`
	StepID       string         `json:"step_id,omitempty"`
	ActionID     string         `json:"action_id,omitempty"`
	ArtifactID   string         `json:"artifact_id,omitempty"`
	AttachmentID string         `json:"attachment_id,omitempty"`
	Path         string         `json:"path,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

func (r OutputRef) ReferencesArtifact() bool {
	return r.ArtifactID != "" || r.Kind == OutputRefArtifact
}

func (r OutputRef) ReferencesAttachment() bool {
	return r.AttachmentID != "" || r.Kind == OutputRefAttachment
}
