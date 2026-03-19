package runtime

import (
	"context"

	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

type DefaultContextAssembler struct{}

func (DefaultContextAssembler) Assemble(_ context.Context, state session.State, spec task.Spec) (map[string]any, error) {
	return map[string]any{
		"task": map[string]any{
			"task_id":   spec.TaskID,
			"task_type": spec.TaskType,
			"goal":      spec.Goal,
		},
		"session": map[string]any{
			"session_id":      state.SessionID,
			"phase":           state.Phase,
			"current_step_id": state.CurrentStepID,
			"retry_count":     state.RetryCount,
		},
		"constraints": spec.Constraints,
		"metadata":    spec.Metadata,
	}, nil
}
