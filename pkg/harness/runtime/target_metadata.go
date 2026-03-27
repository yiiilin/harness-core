package runtime

import "github.com/yiiilin/harness-core/pkg/harness/execution"

func executionFactMetadata(stepMetadata map[string]any) map[string]any {
	if len(stepMetadata) == 0 {
		return nil
	}
	out := map[string]any{}
	copyExecutionFactMetadata(out, stepMetadata)
	if len(out) == 0 {
		return nil
	}
	return out
}

func applyExecutionFactMetadata(dst *map[string]any, stepMetadata map[string]any) {
	if len(stepMetadata) == 0 {
		return
	}
	if *dst == nil {
		*dst = map[string]any{}
	}
	copyExecutionFactMetadata(*dst, stepMetadata)
}

func applyExecutionFactMetadataToHandles(handles []execution.RuntimeHandle, stepMetadata map[string]any) {
	if len(stepMetadata) == 0 {
		return
	}
	for i := range handles {
		if handles[i].Metadata == nil {
			handles[i].Metadata = map[string]any{}
		}
		copyExecutionFactMetadata(handles[i].Metadata, stepMetadata)
	}
}

func copyExecutionFactMetadata(dst, src map[string]any) {
	copyIfPresent(dst, src, execution.ProgramMetadataKeyID)
	copyIfPresent(dst, src, execution.ProgramMetadataKeyGroupID)
	copyIfPresent(dst, src, execution.ProgramMetadataKeyParentStepID)
	copyIfPresent(dst, src, execution.ProgramMetadataKeyNodeID)
	copyIfPresent(dst, src, execution.ProgramMetadataKeyDependsOn)
	copyIfPresent(dst, src, execution.ProgramMetadataKeyMaxConcurrency)
	copyIfPresent(dst, src, execution.ProgramMetadataKeyNodeMaxConcurrency)
	copyIfPresent(dst, src, execution.AggregateMetadataKeyID)
	copyIfPresent(dst, src, execution.AggregateMetadataKeyScope)
	copyIfPresent(dst, src, execution.AggregateMetadataKeyStrategy)
	copyIfPresent(dst, src, execution.AggregateMetadataKeyExpected)
	copyIfPresent(dst, src, execution.AggregateMetadataKeyTitle)
	copyIfPresent(dst, src, execution.AggregateMetadataKeyMaxConcurrency)
	copyIfPresent(dst, src, execution.TargetMetadataKeyID)
	copyIfPresent(dst, src, execution.TargetMetadataKeyKind)
	copyIfPresent(dst, src, execution.TargetMetadataKeyName)
	copyIfPresent(dst, src, execution.TargetMetadataKeyIndex)
	copyIfPresent(dst, src, execution.TargetMetadataKeyCount)
}

func copyIfPresent(dst, src map[string]any, key string) {
	if value, ok := src[key]; ok {
		dst[key] = value
	}
}
