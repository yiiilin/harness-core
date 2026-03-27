package runtime

import (
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
)

func TestProgramReadyRoundForSelectionExcludesFailedReplanNodes(t *testing.T) {
	spec := plan.Spec{
		Steps: []plan.StepSpec{
			{
				StepID:  "node_replan",
				Status:  plan.StepFailed,
				Attempt: 1,
				OnFail:  plan.OnFailSpec{Strategy: "replan"},
				Metadata: map[string]any{
					programGroupMetadataKey:            "grp",
					execution.ProgramMetadataKeyNodeID: "node_replan",
				},
			},
			{
				StepID: "node_prepare",
				Status: plan.StepPending,
				Metadata: map[string]any{
					programGroupMetadataKey:            "grp",
					execution.ProgramMetadataKeyNodeID: "node_prepare",
				},
			},
			{
				StepID: "node_collect",
				Status: plan.StepPending,
				Metadata: map[string]any{
					programGroupMetadataKey:            "grp",
					execution.ProgramMetadataKeyNodeID: "node_collect",
				},
			},
		},
	}

	round, ok := programReadyRoundForSelection(spec, spec.Steps[1], DefaultLoopBudgets())
	if !ok {
		t.Fatalf("expected ready round for two pending siblings, got none")
	}
	if len(round.Steps) != 2 {
		t.Fatalf("expected only pending siblings in ready round, got %#v", round.Steps)
	}
	if round.Steps[0].StepID != "node_prepare" || round.Steps[1].StepID != "node_collect" {
		t.Fatalf("expected failed replan node to be excluded from ready round, got %#v", round.Steps)
	}
}

func TestProgramReadyRoundForSelectionExcludesPureTargetFanoutSiblingsFromSameNode(t *testing.T) {
	spec := plan.Spec{
		Steps: []plan.StepSpec{
			{
				StepID: "node_apply__host_a",
				Status: plan.StepPending,
				Metadata: map[string]any{
					programGroupMetadataKey:                      "grp",
					execution.ProgramMetadataKeyNodeID:           "node_apply",
					execution.AggregateMetadataKeyID:             "agg",
					execution.AggregateMetadataKeyScope:          string(execution.AggregateScopeTargetFanout),
					execution.AggregateMetadataKeyExpected:       2,
					execution.AggregateMetadataKeyStrategy:       string(execution.TargetFailureContinue),
					execution.AggregateMetadataKeyMaxConcurrency: 2,
					execution.TargetMetadataKeyID:                "host-a",
				},
			},
			{
				StepID: "node_apply__host_b",
				Status: plan.StepPending,
				Metadata: map[string]any{
					programGroupMetadataKey:                      "grp",
					execution.ProgramMetadataKeyNodeID:           "node_apply",
					execution.AggregateMetadataKeyID:             "agg",
					execution.AggregateMetadataKeyScope:          string(execution.AggregateScopeTargetFanout),
					execution.AggregateMetadataKeyExpected:       2,
					execution.AggregateMetadataKeyStrategy:       string(execution.TargetFailureContinue),
					execution.AggregateMetadataKeyMaxConcurrency: 2,
					execution.TargetMetadataKeyID:                "host-b",
				},
			},
		},
	}

	if round, ok := programReadyRoundForSelection(spec, spec.Steps[0], DefaultLoopBudgets()); ok {
		t.Fatalf("expected pure target fanout siblings from one node to stay on the fanout scheduler, got %#v", round)
	}
}
