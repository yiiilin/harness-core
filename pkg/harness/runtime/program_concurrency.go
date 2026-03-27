package runtime

import (
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
)

func normalizedProgramConcurrencyLimit(policy *execution.ConcurrencyPolicy) int {
	if policy == nil || policy.MaxConcurrency <= 0 {
		return 0
	}
	return policy.MaxConcurrency
}

func applyProgramConcurrencyMetadata(metadata map[string]any, programPolicy, nodePolicy *execution.ConcurrencyPolicy) map[string]any {
	if metadata == nil {
		metadata = map[string]any{}
	}
	delete(metadata, execution.ProgramMetadataKeyMaxConcurrency)
	delete(metadata, execution.ProgramMetadataKeyNodeMaxConcurrency)
	if limit := normalizedProgramConcurrencyLimit(programPolicy); limit > 0 {
		metadata[execution.ProgramMetadataKeyMaxConcurrency] = limit
	}
	if limit := normalizedProgramConcurrencyLimit(nodePolicy); limit > 0 {
		metadata[execution.ProgramMetadataKeyNodeMaxConcurrency] = limit
	}
	return metadata
}

func programConcurrencyLimitFromMetadata(metadata map[string]any) (int, bool) {
	return positiveIntMetadataValue(metadata, execution.ProgramMetadataKeyMaxConcurrency)
}

func programNodeConcurrencyLimitFromMetadata(metadata map[string]any) (int, bool) {
	return positiveIntMetadataValue(metadata, execution.ProgramMetadataKeyNodeMaxConcurrency)
}

func programReadyRoundMaxConcurrency(selected plan.StepSpec, candidateCount int) int {
	limit := candidateCount
	if explicit, ok := programConcurrencyLimitFromMetadata(selected.Metadata); ok && explicit < limit {
		limit = explicit
	}
	return limit
}

func fanoutRoundMaxConcurrency(step plan.StepSpec, targetCount int) int {
	limits := []int{targetCount}
	if aggregate, ok := execution.AggregateMaxConcurrencyFromMetadata(step.Metadata); ok {
		limits = append(limits, aggregate)
	}
	if programLimit, ok := programConcurrencyLimitFromMetadata(step.Metadata); ok {
		limits = append(limits, programLimit)
	}
	if nodeLimit, ok := programNodeConcurrencyLimitFromMetadata(step.Metadata); ok {
		limits = append(limits, nodeLimit)
	}
	return minPositiveInt(limits...)
}

func fanoutPreparedStepConcurrencyLimit(step plan.StepSpec) int {
	limits := make([]int, 0, 3)
	if aggregateID, scope, ok := execution.AggregateRefFromMetadata(step.Metadata); ok && scope == execution.AggregateScopeTargetFanout && aggregateID != "" {
		if aggregateLimit, ok := execution.AggregateMaxConcurrencyFromMetadata(step.Metadata); ok {
			limits = append(limits, aggregateLimit)
		}
	}
	if programLimit, ok := programConcurrencyLimitFromMetadata(step.Metadata); ok {
		limits = append(limits, programLimit)
	}
	if nodeLimit, ok := programNodeConcurrencyLimitFromMetadata(step.Metadata); ok {
		limits = append(limits, nodeLimit)
	}
	limit := minPositiveInt(limits...)
	if limit <= 0 {
		return 1
	}
	return limit
}

func positiveIntMetadataValue(metadata map[string]any, key string) (int, bool) {
	if len(metadata) == 0 {
		return 0, false
	}
	value, ok := metadata[key]
	if !ok {
		return 0, false
	}
	switch typed := value.(type) {
	case int:
		if typed > 0 {
			return typed, true
		}
	case int32:
		if typed > 0 {
			return int(typed), true
		}
	case int64:
		if typed > 0 {
			return int(typed), true
		}
	case float64:
		if typed > 0 {
			return int(typed), true
		}
	}
	return 0, false
}

func minPositiveInt(values ...int) int {
	limit := 0
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if limit == 0 || value < limit {
			limit = value
		}
	}
	return limit
}
