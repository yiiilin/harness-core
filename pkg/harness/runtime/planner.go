package runtime

import (
	"context"
	"errors"

	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

var ErrNoPlannerConfigured = errors.New("no planner configured")

type NoopPlanner struct{}

func (NoopPlanner) PlanNext(_ context.Context, _ session.State, _ task.Spec, _ map[string]any) (plan.StepSpec, error) {
	return plan.StepSpec{}, ErrNoPlannerConfigured
}
