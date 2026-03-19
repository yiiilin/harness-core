package permission

import (
	"context"
	"strings"

	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

type DefaultEvaluator struct{}

func (DefaultEvaluator) Evaluate(_ context.Context, _ session.State, step plan.StepSpec) (Decision, error) {
	name := step.Action.ToolName
	if name == "" {
		return Decision{Action: Deny, Reason: "missing tool name"}, nil
	}
	if strings.HasPrefix(name, "windows.") {
		return Decision{Action: Ask, Reason: "windows executor is high risk by default", MatchedRule: "default/windows/*"}, nil
	}
	if name == "shell.exec" {
		if mode, _ := step.Action.Args["mode"].(string); mode == "pipe" {
			return Decision{Action: Allow, Reason: "shell.exec in pipe mode allowed by default in v1", MatchedRule: "default/shell.exec:pipe"}, nil
		}
		return Decision{Action: Ask, Reason: "non-pipe shell execution requires approval", MatchedRule: "default/shell.exec:*"}, nil
	}
	return Decision{Action: Allow, Reason: "default allow for registered low-risk tools", MatchedRule: "default/*"}, nil
}
