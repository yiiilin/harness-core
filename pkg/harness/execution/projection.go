package execution

import "github.com/yiiilin/harness-core/pkg/harness/approval"

type BlockedRuntimeWaitScope string

const (
	BlockedRuntimeWaitStep   BlockedRuntimeWaitScope = "step"
	BlockedRuntimeWaitAction BlockedRuntimeWaitScope = "action"
	BlockedRuntimeWaitTarget BlockedRuntimeWaitScope = "target"
)

type TargetSlice struct {
	Target              TargetRef            `json:"target"`
	Program             *ProgramLineage      `json:"program,omitempty"`
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

type ProgramLineage struct {
	ProgramID    string   `json:"program_id,omitempty"`
	GroupID      string   `json:"group_id,omitempty"`
	ParentStepID string   `json:"parent_step_id,omitempty"`
	NodeID       string   `json:"node_id,omitempty"`
	DependsOn    []string `json:"depends_on,omitempty"`
}

type ApprovalLinkage struct {
	ApprovalID string          `json:"approval_id,omitempty"`
	StepID     string          `json:"step_id,omitempty"`
	Status     approval.Status `json:"status,omitempty"`
}

type BlockedRuntimeLinkage struct {
	BlockedRuntimeID string    `json:"blocked_runtime_id,omitempty"`
	ApprovalID       string    `json:"approval_id,omitempty"`
	StepID           string    `json:"step_id,omitempty"`
	ActionID         string    `json:"action_id,omitempty"`
	AttemptID        string    `json:"attempt_id,omitempty"`
	CycleID          string    `json:"cycle_id,omitempty"`
	Target           TargetRef `json:"target,omitempty"`
}

type RuntimeHandleLineage struct {
	HandleID  string          `json:"handle_id,omitempty"`
	AttemptID string          `json:"attempt_id,omitempty"`
	CycleID   string          `json:"cycle_id,omitempty"`
	Target    TargetRef       `json:"target,omitempty"`
	Program   *ProgramLineage `json:"program,omitempty"`
}

type BlockedRuntimeProjection struct {
	Runtime               BlockedRuntime         `json:"runtime"`
	Program               *ProgramLineage        `json:"program,omitempty"`
	ApprovalLinkage       *ApprovalLinkage       `json:"approval_linkage,omitempty"`
	BlockedRuntimeLinkage *BlockedRuntimeLinkage `json:"blocked_runtime_linkage,omitempty"`
	Wait                  BlockedRuntimeWait     `json:"wait,omitempty"`
	TargetSlices          []TargetSlice          `json:"target_slices,omitempty"`
	InteractiveRuntimes   []InteractiveRuntime   `json:"interactive_runtimes,omitempty"`
	Metadata              map[string]any         `json:"metadata,omitempty"`
}

func (s TargetSlice) HasTarget() bool {
	return s.Target.TargetID != ""
}

func (l ProgramLineage) HasLineage() bool {
	return l.ProgramID != "" || l.GroupID != "" || l.ParentStepID != "" || l.NodeID != "" || len(l.DependsOn) > 0
}

func (l ApprovalLinkage) HasLinkage() bool {
	return l.ApprovalID != ""
}

func (l BlockedRuntimeLinkage) HasLinkage() bool {
	return l.BlockedRuntimeID != "" || l.ApprovalID != "" || l.StepID != "" || l.ActionID != "" || l.AttemptID != "" || l.CycleID != "" || l.Target.TargetID != ""
}

func (l RuntimeHandleLineage) HasLineage() bool {
	return l.AttemptID != "" || l.CycleID != "" || l.Target.TargetID != "" || (l.Program != nil && l.Program.HasLineage())
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

func ProgramLineageFromMetadata(metadata map[string]any) (ProgramLineage, bool) {
	if len(metadata) == 0 {
		return ProgramLineage{}, false
	}
	lineage := ProgramLineage{
		ProgramID:    stringFromAnyMetadata(metadata[ProgramMetadataKeyID]),
		GroupID:      stringFromAnyMetadata(metadata[ProgramMetadataKeyGroupID]),
		ParentStepID: stringFromAnyMetadata(metadata[ProgramMetadataKeyParentStepID]),
		NodeID:       stringFromAnyMetadata(metadata[ProgramMetadataKeyNodeID]),
		DependsOn:    stringSliceFromAnyMetadata(metadata[ProgramMetadataKeyDependsOn]),
	}
	if !lineage.HasLineage() {
		return ProgramLineage{}, false
	}
	return lineage, true
}

func ProgramLineageFromCycle(cycle ExecutionCycle) (ProgramLineage, bool) {
	for _, attempt := range cycle.Attempts {
		if lineage, ok := ProgramLineageFromMetadata(attempt.Step.Metadata); ok {
			return lineage, true
		}
		if lineage, ok := ProgramLineageFromMetadata(attempt.Metadata); ok {
			return lineage, true
		}
	}
	for _, action := range cycle.Actions {
		if lineage, ok := ProgramLineageFromMetadata(action.Metadata); ok {
			return lineage, true
		}
	}
	for _, verification := range cycle.Verifications {
		if lineage, ok := ProgramLineageFromMetadata(verification.Metadata); ok {
			return lineage, true
		}
	}
	for _, artifact := range cycle.Artifacts {
		if lineage, ok := ProgramLineageFromMetadata(artifact.Metadata); ok {
			return lineage, true
		}
	}
	for _, handle := range cycle.RuntimeHandles {
		if lineage, ok := ProgramLineageFromMetadata(handle.Metadata); ok {
			return lineage, true
		}
	}
	return ProgramLineage{}, false
}

func ApprovalLinkageFromCycle(cycle ExecutionCycle) (ApprovalLinkage, bool) {
	if cycle.ApprovalID == "" {
		return ApprovalLinkage{}, false
	}
	return ApprovalLinkage{
		ApprovalID: cycle.ApprovalID,
		StepID:     cycle.StepID,
	}, true
}

func ApprovalLinkageFromBlockedRuntime(blocked BlockedRuntime) (ApprovalLinkage, bool) {
	approvalID := blocked.ApprovalID
	if approvalID == "" {
		approvalID = blocked.Approval.ApprovalID
	}
	if approvalID == "" {
		return ApprovalLinkage{}, false
	}
	stepID := blocked.StepID
	if blocked.Approval.StepID != "" {
		stepID = blocked.Approval.StepID
	}
	status := blocked.Approval.Status
	return ApprovalLinkage{
		ApprovalID: approvalID,
		StepID:     stepID,
		Status:     status,
	}, true
}

func BlockedRuntimeLinkageFromBlockedRuntime(blocked BlockedRuntime) (BlockedRuntimeLinkage, bool) {
	linkage := BlockedRuntimeLinkage{
		BlockedRuntimeID: blocked.BlockedRuntimeID,
		ApprovalID:       blocked.ApprovalID,
		StepID:           blocked.StepID,
		ActionID:         blocked.ActionID,
		AttemptID:        blocked.AttemptID,
		CycleID:          blocked.CycleID,
		Target:           blocked.Target,
	}
	if !linkage.HasLinkage() {
		return BlockedRuntimeLinkage{}, false
	}
	return linkage, true
}

func RuntimeHandleLineageFromHandle(handle RuntimeHandle) (RuntimeHandleLineage, bool) {
	lineage := RuntimeHandleLineage{
		HandleID:  handle.HandleID,
		AttemptID: handle.AttemptID,
		CycleID:   handle.CycleID,
	}
	if target, ok := TargetRefFromMetadata(handle.Metadata); ok {
		lineage.Target = target
	}
	if program, ok := ProgramLineageFromMetadata(handle.Metadata); ok {
		programCopy := program
		lineage.Program = &programCopy
	}
	if !lineage.HasLineage() {
		return RuntimeHandleLineage{}, false
	}
	return lineage, true
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
		if lineage, ok := programLineageFromTargetSlice(slices[i]); ok {
			lineageCopy := lineage
			slices[i].Program = &lineageCopy
		}
		slices[i].InteractiveRuntimes = InteractiveRuntimesFromHandles(slices[i].RuntimeHandles)
	}
	return slices
}

func programLineageFromTargetSlice(slice TargetSlice) (ProgramLineage, bool) {
	for _, attempt := range slice.Attempts {
		if lineage, ok := ProgramLineageFromMetadata(attempt.Step.Metadata); ok {
			return lineage, true
		}
		if lineage, ok := ProgramLineageFromMetadata(attempt.Metadata); ok {
			return lineage, true
		}
	}
	for _, action := range slice.Actions {
		if lineage, ok := ProgramLineageFromMetadata(action.Metadata); ok {
			return lineage, true
		}
	}
	for _, verification := range slice.Verifications {
		if lineage, ok := ProgramLineageFromMetadata(verification.Metadata); ok {
			return lineage, true
		}
	}
	for _, artifact := range slice.Artifacts {
		if lineage, ok := ProgramLineageFromMetadata(artifact.Metadata); ok {
			return lineage, true
		}
	}
	for _, handle := range slice.RuntimeHandles {
		if lineage, ok := ProgramLineageFromMetadata(handle.Metadata); ok {
			return lineage, true
		}
	}
	return ProgramLineage{}, false
}

func stringFromAnyMetadata(raw any) string {
	text, _ := raw.(string)
	return text
}

func stringSliceFromAnyMetadata(raw any) []string {
	switch typed := raw.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok && text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}
