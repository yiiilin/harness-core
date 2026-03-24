package capability

import (
	"errors"

	"github.com/yiiilin/harness-core/pkg/harness/tool"
)

type UnsupportedReasonCode string

const (
	ReasonCapabilityNotFound             UnsupportedReasonCode = "CAPABILITY_NOT_FOUND"
	ReasonCapabilityDisabled             UnsupportedReasonCode = "CAPABILITY_DISABLED"
	ReasonCapabilityVersionNotFound      UnsupportedReasonCode = "CAPABILITY_VERSION_NOT_FOUND"
	ReasonCapabilityViewNotFound         UnsupportedReasonCode = "CAPABILITY_VIEW_NOT_FOUND"
	ReasonCapabilityViewDrift            UnsupportedReasonCode = "CAPABILITY_VIEW_DRIFT"
	ReasonMultiTargetFanoutUnsupported   UnsupportedReasonCode = "MULTI_TARGET_FANOUT_UNSUPPORTED"
	ReasonPreplannedToolGraphUnsupported UnsupportedReasonCode = "PREPLANNED_TOOL_GRAPH_UNSUPPORTED"
	ReasonInteractiveReopenUnsupported   UnsupportedReasonCode = "INTERACTIVE_REOPEN_UNSUPPORTED"
	ReasonArtifactInputUnsupported       UnsupportedReasonCode = "ARTIFACT_INPUT_UNSUPPORTED"
)

const (
	SupportMetadataKeyMultiTargetFanout   = "supports_multi_target_fanout"
	SupportMetadataKeyPreplannedToolGraph = "supports_preplanned_tool_graph"
	SupportMetadataKeyInteractiveReopen   = "supports_interactive_reopen"
	SupportMetadataKeyArtifactInput       = "supports_artifact_input"
)

type UnsupportedReason struct {
	Code     UnsupportedReasonCode `json:"code"`
	Message  string                `json:"message,omitempty"`
	Metadata map[string]any        `json:"metadata,omitempty"`
}

type SupportRequirements struct {
	MultiTargetFanout   bool `json:"multi_target_fanout,omitempty"`
	PreplannedToolGraph bool `json:"preplanned_tool_graph,omitempty"`
	InteractiveReopen   bool `json:"interactive_reopen,omitempty"`
	ArtifactInput       bool `json:"artifact_input,omitempty"`
}

type MatchResult struct {
	Supported  bool                `json:"supported"`
	Resolution *Resolution         `json:"resolution,omitempty"`
	Reasons    []UnsupportedReason `json:"reasons,omitempty"`
}

func UnsupportedReasonFromError(err error, req Request) (UnsupportedReason, bool) {
	if err == nil {
		return UnsupportedReason{}, false
	}
	metadata := map[string]any{}
	if req.Action.ToolName != "" {
		metadata["tool_name"] = req.Action.ToolName
	}
	if req.Action.ToolVersion != "" {
		metadata["tool_version"] = req.Action.ToolVersion
	}
	switch {
	case errors.Is(err, ErrCapabilityNotFound):
		return UnsupportedReason{Code: ReasonCapabilityNotFound, Message: err.Error(), Metadata: metadata}, true
	case errors.Is(err, ErrCapabilityDisabled):
		return UnsupportedReason{Code: ReasonCapabilityDisabled, Message: err.Error(), Metadata: metadata}, true
	case errors.Is(err, ErrCapabilityVersionNotFound):
		return UnsupportedReason{Code: ReasonCapabilityVersionNotFound, Message: err.Error(), Metadata: metadata}, true
	case errors.Is(err, ErrCapabilityViewNotFound):
		return UnsupportedReason{Code: ReasonCapabilityViewNotFound, Message: err.Error(), Metadata: metadata}, true
	case errors.Is(err, ErrCapabilityViewDrift):
		return UnsupportedReason{Code: ReasonCapabilityViewDrift, Message: err.Error(), Metadata: metadata}, true
	default:
		return UnsupportedReason{}, false
	}
}

func UnsupportedReasonsForDefinition(def tool.Definition, requirements *SupportRequirements) []UnsupportedReason {
	var reasons []UnsupportedReason
	if requirements == nil {
		return nil
	}
	if requirements.MultiTargetFanout && !supportFlag(def.Metadata, SupportMetadataKeyMultiTargetFanout) {
		reasons = append(reasons, UnsupportedReason{
			Code:    ReasonMultiTargetFanoutUnsupported,
			Message: "capability does not advertise multi-target fan-out support",
			Metadata: map[string]any{
				"tool_name":    def.ToolName,
				"tool_version": def.Version,
			},
		})
	}
	if requirements.PreplannedToolGraph && !supportFlag(def.Metadata, SupportMetadataKeyPreplannedToolGraph) {
		reasons = append(reasons, UnsupportedReason{
			Code:    ReasonPreplannedToolGraphUnsupported,
			Message: "capability does not advertise preplanned tool-graph support",
			Metadata: map[string]any{
				"tool_name":    def.ToolName,
				"tool_version": def.Version,
			},
		})
	}
	if requirements.InteractiveReopen && !supportFlag(def.Metadata, SupportMetadataKeyInteractiveReopen) {
		reasons = append(reasons, UnsupportedReason{
			Code:    ReasonInteractiveReopenUnsupported,
			Message: "capability does not advertise interactive reopen support",
			Metadata: map[string]any{
				"tool_name":    def.ToolName,
				"tool_version": def.Version,
			},
		})
	}
	if requirements.ArtifactInput && !supportFlag(def.Metadata, SupportMetadataKeyArtifactInput) {
		reasons = append(reasons, UnsupportedReason{
			Code:    ReasonArtifactInputUnsupported,
			Message: "capability does not advertise artifact input support",
			Metadata: map[string]any{
				"tool_name":    def.ToolName,
				"tool_version": def.Version,
			},
		})
	}
	return reasons
}

func supportFlag(metadata map[string]any, key string) bool {
	if len(metadata) == 0 {
		return false
	}
	value, ok := metadata[key]
	if !ok {
		return false
	}
	flag, _ := value.(bool)
	return flag
}
