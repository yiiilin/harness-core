package execution

import (
	"sort"

	"github.com/yiiilin/harness-core/pkg/harness/plan"
)

type AggregateScope string
type AggregateStatus string

const (
	AggregateScopeTargetFanout AggregateScope = "target_fanout"

	AggregateStatusPending       AggregateStatus = "pending"
	AggregateStatusCompleted     AggregateStatus = "completed"
	AggregateStatusPartialFailed AggregateStatus = "partial_failed"
	AggregateStatusFailed        AggregateStatus = "failed"
)

const (
	AggregateMetadataKeyID             = "aggregate_id"
	AggregateMetadataKeyScope          = "aggregate_scope"
	AggregateMetadataKeyStrategy       = "aggregate_strategy"
	AggregateMetadataKeyExpected       = "aggregate_expected"
	AggregateMetadataKeyTitle          = "aggregate_title"
	AggregateMetadataKeyMaxConcurrency = "aggregate_max_concurrency"
	AggregateMetadataKeyTargetStatus   = "aggregate_target_status"
	AggregateMetadataKeyTargetReason   = "aggregate_target_reason"
)

type AggregateTargetResult struct {
	Target  TargetRef       `json:"target,omitempty"`
	StepID  string          `json:"step_id,omitempty"`
	Status  plan.StepStatus `json:"status"`
	Attempt int             `json:"attempt,omitempty"`
	Reason  string          `json:"reason,omitempty"`
}

type AggregateResult struct {
	AggregateID    string                  `json:"aggregate_id"`
	Scope          AggregateScope          `json:"scope"`
	Strategy       TargetFailureStrategy   `json:"strategy,omitempty"`
	ProgramID      string                  `json:"program_id,omitempty"`
	NodeID         string                  `json:"node_id,omitempty"`
	Title          string                  `json:"title,omitempty"`
	MaxConcurrency int                     `json:"max_concurrency,omitempty"`
	Status         AggregateStatus         `json:"status"`
	Expected       int                     `json:"expected,omitempty"`
	Completed      int                     `json:"completed,omitempty"`
	Failed         int                     `json:"failed,omitempty"`
	Pending        int                     `json:"pending,omitempty"`
	Targets        []AggregateTargetResult `json:"targets,omitempty"`
}

func ApplyAggregateMetadata(metadata map[string]any, scope AggregateScope, aggregateID, programID, nodeID, title string, strategy TargetFailureStrategy, expected int) map[string]any {
	if metadata == nil {
		metadata = map[string]any{}
	}
	if aggregateID != "" {
		metadata[AggregateMetadataKeyID] = aggregateID
	}
	if scope != "" {
		metadata[AggregateMetadataKeyScope] = string(scope)
	}
	if strategy != "" {
		metadata[AggregateMetadataKeyStrategy] = string(strategy)
	}
	if expected > 0 {
		metadata[AggregateMetadataKeyExpected] = expected
	}
	if title != "" {
		metadata[AggregateMetadataKeyTitle] = title
	}
	if programID != "" {
		metadata[ProgramMetadataKeyID] = programID
	}
	if nodeID != "" {
		metadata[ProgramMetadataKeyNodeID] = nodeID
	}
	return metadata
}

func ApplyAggregateConcurrencyMetadata(metadata map[string]any, maxConcurrency int) map[string]any {
	if maxConcurrency <= 0 {
		return metadata
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata[AggregateMetadataKeyMaxConcurrency] = maxConcurrency
	return metadata
}

func ApplyAggregateTargetOutcomeMetadata(metadata map[string]any, status plan.StepStatus, reason string) map[string]any {
	cloned := cloneAggregateMetadataMap(metadata)
	if status != "" {
		cloned[AggregateMetadataKeyTargetStatus] = string(status)
	} else {
		delete(cloned, AggregateMetadataKeyTargetStatus)
	}
	if reason != "" {
		cloned[AggregateMetadataKeyTargetReason] = reason
	} else {
		delete(cloned, AggregateMetadataKeyTargetReason)
	}
	if len(cloned) == 0 {
		return nil
	}
	return cloned
}

func cloneAggregateMetadataMap(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

func AggregateRefFromMetadata(metadata map[string]any) (aggregateID string, scope AggregateScope, ok bool) {
	if len(metadata) == 0 {
		return "", "", false
	}
	aggregateID, _ = metadata[AggregateMetadataKeyID].(string)
	if aggregateID == "" {
		return "", "", false
	}
	scopeValue, _ := metadata[AggregateMetadataKeyScope].(string)
	return aggregateID, AggregateScope(scopeValue), true
}

func AggregateMaxConcurrencyFromMetadata(metadata map[string]any) (int, bool) {
	if len(metadata) == 0 {
		return 0, false
	}
	value, ok := metadata[AggregateMetadataKeyMaxConcurrency]
	if !ok {
		return 0, false
	}
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	default:
		return 0, false
	}
}

func AggregateTargetOutcomeFromMetadata(metadata map[string]any) (plan.StepStatus, string, bool) {
	if len(metadata) == 0 {
		return "", "", false
	}
	statusValue, _ := metadata[AggregateMetadataKeyTargetStatus].(string)
	if statusValue == "" {
		return "", "", false
	}
	reason, _ := metadata[AggregateMetadataKeyTargetReason].(string)
	return plan.StepStatus(statusValue), reason, true
}

func AggregateResultsFromPlan(spec plan.Spec) []AggregateResult {
	return AggregateResultsFromSteps(spec.Steps)
}

func AggregateResultsFromSteps(steps []plan.StepSpec) []AggregateResult {
	type aggregateBucket struct {
		result AggregateResult
		order  int
	}

	buckets := map[string]*aggregateBucket{}
	order := []string{}
	for idx, step := range steps {
		aggregateID, scope, ok := AggregateRefFromMetadata(step.Metadata)
		if !ok {
			continue
		}
		bucket, exists := buckets[aggregateID]
		if !exists {
			result := AggregateResult{
				AggregateID:    aggregateID,
				Scope:          scope,
				Status:         AggregateStatusPending,
				ProgramID:      stringFromMetadata(step.Metadata, ProgramMetadataKeyID),
				NodeID:         stringFromMetadata(step.Metadata, ProgramMetadataKeyNodeID),
				Title:          stringFromMetadata(step.Metadata, AggregateMetadataKeyTitle),
				Strategy:       TargetFailureStrategy(stringFromMetadata(step.Metadata, AggregateMetadataKeyStrategy)),
				MaxConcurrency: intFromMetadata(step.Metadata, AggregateMetadataKeyMaxConcurrency),
				Expected:       intFromMetadata(step.Metadata, AggregateMetadataKeyExpected),
			}
			bucket = &aggregateBucket{result: result, order: idx}
			buckets[aggregateID] = bucket
			order = append(order, aggregateID)
		}

		targetStatus := step.Status
		targetReason := step.Reason
		if metadataStatus, metadataReason, ok := AggregateTargetOutcomeFromMetadata(step.Metadata); ok {
			targetStatus = metadataStatus
			if metadataReason != "" || targetReason == "" {
				targetReason = metadataReason
			}
		}
		target, _ := TargetFromStep(step)
		bucket.result.Targets = append(bucket.result.Targets, AggregateTargetResult{
			Target:  target,
			StepID:  step.StepID,
			Status:  targetStatus,
			Attempt: step.Attempt,
			Reason:  targetReason,
		})
		switch targetStatus {
		case plan.StepCompleted:
			bucket.result.Completed++
		case plan.StepFailed:
			bucket.result.Failed++
		default:
			bucket.result.Pending++
		}
	}

	sort.SliceStable(order, func(i, j int) bool {
		return buckets[order[i]].order < buckets[order[j]].order
	})

	results := make([]AggregateResult, 0, len(order))
	for _, aggregateID := range order {
		bucket := buckets[aggregateID]
		if bucket.result.Expected <= 0 {
			bucket.result.Expected = len(bucket.result.Targets)
		}
		sort.SliceStable(bucket.result.Targets, func(i, j int) bool {
			left := bucket.result.Targets[i]
			right := bucket.result.Targets[j]
			if left.Target.TargetID == right.Target.TargetID {
				return left.StepID < right.StepID
			}
			if left.Target.TargetID == "" {
				return false
			}
			if right.Target.TargetID == "" {
				return true
			}
			return left.Target.TargetID < right.Target.TargetID
		})
		switch {
		case bucket.result.Pending > 0:
			bucket.result.Status = AggregateStatusPending
		case bucket.result.Failed == 0:
			bucket.result.Status = AggregateStatusCompleted
		case bucket.result.Completed > 0:
			bucket.result.Status = AggregateStatusPartialFailed
		default:
			bucket.result.Status = AggregateStatusFailed
		}
		results = append(results, bucket.result)
	}
	return results
}

func stringFromMetadata(metadata map[string]any, key string) string {
	if len(metadata) == 0 {
		return ""
	}
	value, _ := metadata[key].(string)
	return value
}

func intFromMetadata(metadata map[string]any, key string) int {
	if len(metadata) == 0 {
		return 0
	}
	switch typed := metadata[key].(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}
