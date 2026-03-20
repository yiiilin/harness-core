package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

var ErrNoPlannerConfigured = errors.New("no planner configured")

type NoopPlanner struct{}

func (NoopPlanner) PlanNext(_ context.Context, _ session.State, _ task.Spec, _ ContextPackage) (plan.StepSpec, error) {
	return plan.StepSpec{}, ErrNoPlannerConfigured
}

type DemoPlanner struct{}

func (DemoPlanner) PlanNext(_ context.Context, _ session.State, spec task.Spec, assembled ContextPackage) (plan.StepSpec, error) {
	goal := spec.Goal
	if goal == "" {
		goal = assembled.Task.Goal
	}
	if goal == "" {
		return plan.StepSpec{}, errors.New("missing goal")
	}
	lower := strings.ToLower(goal)
	if strings.Contains(lower, "echo ") {
		return plan.StepSpec{
			StepID: fmt.Sprintf("planned_%d", len(goal)),
			Title:  goal,
			Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": goal, "timeout_ms": 5000}},
			Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}}}},
			OnFail: plan.OnFailSpec{Strategy: "abort"},
		}, nil
	}
	return plan.StepSpec{}, errors.New("demo planner cannot derive a step for this goal")
}
