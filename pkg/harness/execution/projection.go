package execution

type BlockedRuntimeWaitScope string

const (
	BlockedRuntimeWaitStep   BlockedRuntimeWaitScope = "step"
	BlockedRuntimeWaitAction BlockedRuntimeWaitScope = "action"
	BlockedRuntimeWaitTarget BlockedRuntimeWaitScope = "target"
)

type TargetSlice struct {
	Target              TargetRef            `json:"target"`
	Attempts            []Attempt            `json:"attempts,omitempty"`
	Actions             []ActionRecord       `json:"actions,omitempty"`
	Verifications       []VerificationRecord `json:"verifications,omitempty"`
	Artifacts           []Artifact           `json:"artifacts,omitempty"`
	RuntimeHandles      []RuntimeHandle      `json:"runtime_handles,omitempty"`
	InteractiveRuntimes []InteractiveRuntime `json:"interactive_runtimes,omitempty"`
	Metadata            map[string]any       `json:"metadata,omitempty"`
}

type BlockedRuntimeWait struct {
	Scope      BlockedRuntimeWaitScope `json:"scope"`
	StepID     string                  `json:"step_id,omitempty"`
	ActionID   string                  `json:"action_id,omitempty"`
	Target     TargetRef               `json:"target,omitempty"`
	WaitingFor string                  `json:"waiting_for,omitempty"`
	Metadata   map[string]any          `json:"metadata,omitempty"`
}

type BlockedRuntimeProjection struct {
	Runtime             BlockedRuntime       `json:"runtime"`
	Wait                BlockedRuntimeWait   `json:"wait,omitempty"`
	TargetSlices        []TargetSlice        `json:"target_slices,omitempty"`
	InteractiveRuntimes []InteractiveRuntime `json:"interactive_runtimes,omitempty"`
	Metadata            map[string]any       `json:"metadata,omitempty"`
}

func (s TargetSlice) HasTarget() bool {
	return s.Target.TargetID != ""
}

func (s TargetSlice) HasExecutionFacts() bool {
	return len(s.Attempts) > 0 ||
		len(s.Actions) > 0 ||
		len(s.Verifications) > 0 ||
		len(s.Artifacts) > 0 ||
		len(s.RuntimeHandles) > 0 ||
		len(s.InteractiveRuntimes) > 0
}

func (w BlockedRuntimeWait) ReferencesAction() bool {
	return w.ActionID != ""
}

func (w BlockedRuntimeWait) ReferencesTarget() bool {
	return w.Target.TargetID != ""
}

func (p BlockedRuntimeProjection) HasTargetSlices() bool {
	return len(p.TargetSlices) > 0
}

func (p BlockedRuntimeProjection) HasInteractiveRuntimes() bool {
	return len(p.InteractiveRuntimes) > 0
}

func TargetSlicesFromCycle(cycle ExecutionCycle) []TargetSlice {
	index := map[string]int{}
	slices := []TargetSlice{}
	ensure := func(ref TargetRef) *TargetSlice {
		key := ref.TargetID + "\x00" + ref.Kind
		if pos, ok := index[key]; ok {
			return &slices[pos]
		}
		index[key] = len(slices)
		slices = append(slices, TargetSlice{Target: ref})
		return &slices[len(slices)-1]
	}

	for _, attempt := range cycle.Attempts {
		if ref, ok := TargetRefFromMetadata(attempt.Metadata); ok {
			slice := ensure(ref)
			slice.Attempts = append(slice.Attempts, attempt)
		}
	}
	for _, action := range cycle.Actions {
		if ref, ok := TargetRefFromMetadata(action.Metadata); ok {
			slice := ensure(ref)
			slice.Actions = append(slice.Actions, action)
		}
	}
	for _, verification := range cycle.Verifications {
		if ref, ok := TargetRefFromMetadata(verification.Metadata); ok {
			slice := ensure(ref)
			slice.Verifications = append(slice.Verifications, verification)
		}
	}
	for _, artifact := range cycle.Artifacts {
		if ref, ok := TargetRefFromMetadata(artifact.Metadata); ok {
			slice := ensure(ref)
			slice.Artifacts = append(slice.Artifacts, artifact)
		}
	}
	for _, handle := range cycle.RuntimeHandles {
		if ref, ok := TargetRefFromMetadata(handle.Metadata); ok {
			slice := ensure(ref)
			slice.RuntimeHandles = append(slice.RuntimeHandles, handle)
		}
	}
	for i := range slices {
		slices[i].InteractiveRuntimes = InteractiveRuntimesFromHandles(slices[i].RuntimeHandles)
	}
	return slices
}
