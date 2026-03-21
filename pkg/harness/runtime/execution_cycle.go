package runtime

import (
	"github.com/google/uuid"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
)

const stepExecutionCycleKey = "execution_cycle_id"

func executionCycleIDFromStep(step plan.StepSpec) string {
	if step.Status != plan.StepBlocked && step.Status != plan.StepRunning {
		return ""
	}
	if len(step.Metadata) == 0 {
		return ""
	}
	cycleID, _ := step.Metadata[stepExecutionCycleKey].(string)
	return cycleID
}

func ensureExecutionCycleID(step *plan.StepSpec, existing string) string {
	if existing == "" && step != nil {
		existing = executionCycleIDFromStep(*step)
	}
	if existing == "" {
		existing = "cyc_" + uuid.NewString()
	}
	if step != nil {
		if step.Metadata == nil {
			step.Metadata = map[string]any{}
		}
		step.Metadata[stepExecutionCycleKey] = existing
	}
	return existing
}
