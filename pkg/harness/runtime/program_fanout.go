package runtime

import (
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
)

func compileRuntimeProgramNodeOnFail(node execution.ProgramNode, fanoutCount int) plan.OnFailSpec {
	var out plan.OnFailSpec
	if node.OnFail != nil {
		out = *node.OnFail
	}
	if fanoutCount > 1 && normalizedProgramTargetFailureStrategy(node.Targeting) == execution.TargetFailureContinue {
		out.Strategy = string(execution.TargetFailureContinue)
	}
	return out
}

func compileAttachedProgramNodeOnFail(parent plan.OnFailSpec, node execution.ProgramNode, fanoutCount int) plan.OnFailSpec {
	out := parent
	if node.OnFail != nil {
		out = *node.OnFail
	}
	if fanoutCount > 1 && normalizedProgramTargetFailureStrategy(node.Targeting) == execution.TargetFailureContinue {
		out.Strategy = string(execution.TargetFailureContinue)
	}
	return out
}

func normalizedProgramTargetFailureStrategy(selection *execution.TargetSelection) execution.TargetFailureStrategy {
	if selection == nil || selection.OnPartialFailure == "" {
		return execution.TargetFailureAbort
	}
	return selection.OnPartialFailure
}

func normalizedProgramTargetMaxConcurrency(selection *execution.TargetSelection, targetCount int) int {
	if selection == nil {
		return 0
	}
	return selection.EffectiveMaxConcurrency(targetCount)
}

func applyProgramNodeAggregateMetadata(metadata map[string]any, aggregateID, programID, nodeID, title string, strategy execution.TargetFailureStrategy, expected int, maxConcurrency int) map[string]any {
	if expected <= 1 {
		return metadata
	}
	metadata = execution.ApplyAggregateMetadata(
		metadata,
		execution.AggregateScopeTargetFanout,
		aggregateID,
		programID,
		nodeID,
		title,
		strategy,
		expected,
	)
	return execution.ApplyAggregateConcurrencyMetadata(metadata, maxConcurrency)
}
