package execution

import "github.com/yiiilin/harness-core/pkg/harness/plan"

const (
	TargetArgKey           = "execution_target"
	TargetMetadataKeyID    = "target_id"
	TargetMetadataKeyKind  = "target_kind"
	TargetMetadataKeyName  = "target_display_name"
	TargetMetadataKeyIndex = "target_index"
	TargetMetadataKeyCount = "target_count"
)

func ApplyTargetMetadata(metadata map[string]any, target Target, index, total int) map[string]any {
	if metadata == nil {
		metadata = map[string]any{}
	}
	if target.TargetID != "" {
		metadata[TargetMetadataKeyID] = target.TargetID
	}
	if target.Kind != "" {
		metadata[TargetMetadataKeyKind] = target.Kind
	}
	if target.DisplayName != "" {
		metadata[TargetMetadataKeyName] = target.DisplayName
	}
	if index > 0 {
		metadata[TargetMetadataKeyIndex] = index
	}
	if total > 0 {
		metadata[TargetMetadataKeyCount] = total
	}
	return metadata
}

func TargetRefFromMetadata(metadata map[string]any) (TargetRef, bool) {
	if len(metadata) == 0 {
		return TargetRef{}, false
	}
	targetID, _ := metadata[TargetMetadataKeyID].(string)
	if targetID == "" {
		return TargetRef{}, false
	}
	kind, _ := metadata[TargetMetadataKeyKind].(string)
	return TargetRef{TargetID: targetID, Kind: kind}, true
}

func TargetArgValue(target Target) map[string]any {
	value := map[string]any{
		TargetMetadataKeyID: target.TargetID,
	}
	if target.Kind != "" {
		value[TargetMetadataKeyKind] = target.Kind
	}
	if target.DisplayName != "" {
		value[TargetMetadataKeyName] = target.DisplayName
	}
	if len(target.Metadata) > 0 {
		value["metadata"] = target.Metadata
	}
	return value
}

func TargetFromStep(step plan.StepSpec) (TargetRef, bool) {
	return TargetRefFromMetadata(step.Metadata)
}
