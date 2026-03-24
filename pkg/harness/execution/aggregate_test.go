package execution_test

import (
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
)

func TestAggregateResultsFromPlanSummarizesTargetFanoutOutcomes(t *testing.T) {
	metadataA := execution.ApplyAggregateMetadata(nil, execution.AggregateScopeTargetFanout, "agg_apply", "prog_demo", "node_apply", "apply", execution.TargetFailureContinue, 2)
	metadataA = execution.ApplyTargetMetadata(metadataA, execution.Target{TargetID: "host-a", Kind: "host"}, 1, 2)
	metadataB := execution.ApplyAggregateMetadata(nil, execution.AggregateScopeTargetFanout, "agg_apply", "prog_demo", "node_apply", "apply", execution.TargetFailureContinue, 2)
	metadataB = execution.ApplyTargetMetadata(metadataB, execution.Target{TargetID: "host-b", Kind: "host"}, 2, 2)

	aggregates := execution.AggregateResultsFromPlan(plan.Spec{
		Steps: []plan.StepSpec{
			{StepID: "prog_demo__node_apply__host-a", Status: plan.StepCompleted, Metadata: metadataA},
			{StepID: "prog_demo__node_apply__host-b", Status: plan.StepFailed, Metadata: metadataB, Attempt: 2, Reason: "verify failed"},
		},
	})
	if len(aggregates) != 1 {
		t.Fatalf("expected one aggregate result, got %#v", aggregates)
	}
	result := aggregates[0]
	if result.AggregateID != "agg_apply" || result.Scope != execution.AggregateScopeTargetFanout {
		t.Fatalf("unexpected aggregate identity: %#v", result)
	}
	if result.Status != execution.AggregateStatusPartialFailed {
		t.Fatalf("expected partial_failed aggregate, got %#v", result)
	}
	if result.Completed != 1 || result.Failed != 1 || result.Pending != 0 || result.Expected != 2 {
		t.Fatalf("unexpected aggregate counts: %#v", result)
	}
	if len(result.Targets) != 2 || result.Targets[0].Target.TargetID != "host-a" || result.Targets[1].Target.TargetID != "host-b" {
		t.Fatalf("expected ordered target results, got %#v", result.Targets)
	}
}

func TestAggregateResultsFromPlanMarksAllFailedFanoutAsFailed(t *testing.T) {
	metadataA := execution.ApplyAggregateMetadata(nil, execution.AggregateScopeTargetFanout, "agg_apply", "prog_demo", "node_apply", "apply", execution.TargetFailureContinue, 2)
	metadataA = execution.ApplyTargetMetadata(metadataA, execution.Target{TargetID: "host-a", Kind: "host"}, 1, 2)
	metadataB := execution.ApplyAggregateMetadata(nil, execution.AggregateScopeTargetFanout, "agg_apply", "prog_demo", "node_apply", "apply", execution.TargetFailureContinue, 2)
	metadataB = execution.ApplyTargetMetadata(metadataB, execution.Target{TargetID: "host-b", Kind: "host"}, 2, 2)

	aggregates := execution.AggregateResultsFromSteps([]plan.StepSpec{
		{StepID: "prog_demo__node_apply__host-a", Status: plan.StepFailed, Metadata: metadataA},
		{StepID: "prog_demo__node_apply__host-b", Status: plan.StepFailed, Metadata: metadataB},
	})
	if len(aggregates) != 1 {
		t.Fatalf("expected one aggregate result, got %#v", aggregates)
	}
	if aggregates[0].Status != execution.AggregateStatusFailed {
		t.Fatalf("expected failed aggregate, got %#v", aggregates[0])
	}
}
