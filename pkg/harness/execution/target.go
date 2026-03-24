package execution

type Target struct {
	TargetID    string         `json:"target_id"`
	Kind        string         `json:"kind"`
	DisplayName string         `json:"display_name,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type TargetRef struct {
	TargetID string `json:"target_id"`
	Kind     string `json:"kind,omitempty"`
}

type TargetSelectionMode string
type TargetFailureStrategy string

const (
	TargetSelectionSingle         TargetSelectionMode = "single"
	TargetSelectionFanoutExplicit TargetSelectionMode = "fanout_explicit"
	TargetSelectionFanoutAll      TargetSelectionMode = "fanout_all"

	TargetFailureAbort    TargetFailureStrategy = "abort"
	TargetFailureContinue TargetFailureStrategy = "continue"
)

type TargetSelection struct {
	Mode             TargetSelectionMode   `json:"mode,omitempty"`
	Targets          []Target              `json:"targets,omitempty"`
	MaxConcurrency   int                   `json:"max_concurrency,omitempty"`
	OnPartialFailure TargetFailureStrategy `json:"on_partial_failure,omitempty"`
	Metadata         map[string]any        `json:"metadata,omitempty"`
}

func (s TargetSelection) MultiTargetRequested() bool {
	if len(s.Targets) > 1 {
		return true
	}
	switch s.Mode {
	case TargetSelectionFanoutExplicit, TargetSelectionFanoutAll:
		return true
	default:
		return false
	}
}
