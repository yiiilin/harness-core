package runtime

import (
	"context"

	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

type DefaultContextAssembler struct{}

func (DefaultContextAssembler) Assemble(_ context.Context, state session.State, spec task.Spec) (ContextPackage, error) {
	return ContextPackage{
		Task: ContextTask{
			TaskID:   spec.TaskID,
			TaskType: spec.TaskType,
			Goal:     spec.Goal,
		},
		Session: ContextSession{
			SessionID:      state.SessionID,
			Phase:          state.Phase,
			CurrentStepID:  state.CurrentStepID,
			RetryCount:     state.RetryCount,
			ExecutionState: state.ExecutionState,
		},
		Constraints: spec.Constraints,
		Metadata:    spec.Metadata,
	}, nil
}
