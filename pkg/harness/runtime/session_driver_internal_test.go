package runtime

import (
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

func TestSelectNextStepForSessionDoesNotSkipBlockedProgramStepToUnrelatedLaterPendingStep(t *testing.T) {
	spec := plan.Spec{
		Steps: []plan.StepSpec{
			{
				StepID: "node_apply",
				Status: plan.StepPending,
				Metadata: map[string]any{
					programGroupMetadataKey:            "grp",
					execution.ProgramMetadataKeyNodeID: "node_apply",
					programDependsOnMetadataKey:        []string{"node_prepare"},
				},
			},
			{
				StepID: "step_later",
				Status: plan.StepPending,
			},
		},
	}

	selection := selectNextStepForSession(session.State{}, spec, spec, DefaultLoopBudgets())
	if selection.HasStep {
		t.Fatalf("expected blocked program step to keep unrelated later pending step from being selected, got %#v", selection.Step)
	}
	if selection.NeedsPlanning {
		t.Fatalf("expected blocked program step to stall selection rather than request replan")
	}
}

func TestPinnedStepForSessionRejectsProgramStepWithUnsatisfiedDependencies(t *testing.T) {
	spec := plan.Spec{
		Steps: []plan.StepSpec{
			{
				StepID: "node_apply",
				Status: plan.StepPending,
				Metadata: map[string]any{
					programGroupMetadataKey:            "grp",
					execution.ProgramMetadataKeyNodeID: "node_apply",
					programDependsOnMetadataKey:        []string{"node_prepare"},
				},
			},
		},
	}

	state := session.State{CurrentStepID: "node_apply"}
	if step, ok := pinnedStepForSession(state, spec, DefaultLoopBudgets()); ok {
		t.Fatalf("expected pinned blocked program step to be rejected, got %#v", step)
	}
}
