package permission

import (
	"context"

	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

type Evaluator interface {
	Evaluate(ctx context.Context, state session.State, step plan.StepSpec) (Decision, error)
}
