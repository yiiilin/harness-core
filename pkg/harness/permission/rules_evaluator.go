package permission

import (
	"context"
	"fmt"
	"strings"

	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

type RulesEvaluator struct {
	Rules    []Rule
	Fallback Evaluator
}

func (e RulesEvaluator) Evaluate(ctx context.Context, state session.State, step plan.StepSpec) (Decision, error) {
	name := step.Action.ToolName
	if name == "" {
		if e.Fallback != nil {
			return e.Fallback.Evaluate(ctx, state, step)
		}
		return Decision{Action: Deny, Reason: "missing tool name"}, nil
	}

	for _, rule := range e.Rules {
		if rule.Permission != name {
			continue
		}
		if !matchesRulePattern(rule.Pattern, step.Action.Args) {
			continue
		}
		return Decision{
			Action:      rule.Action,
			Reason:      fmt.Sprintf("matched module default policy rule for %s", rule.Permission),
			MatchedRule: fmt.Sprintf("module/%s:%s", rule.Permission, rule.Pattern),
		}, nil
	}

	if e.Fallback != nil {
		return e.Fallback.Evaluate(ctx, state, step)
	}
	return Decision{Action: Allow, Reason: "default allow", MatchedRule: "default/*"}, nil
}

func matchesRulePattern(pattern string, args map[string]any) bool {
	trimmed := strings.TrimSpace(pattern)
	if trimmed == "" || trimmed == "*" {
		return true
	}
	for _, clause := range strings.Split(trimmed, ",") {
		clause = strings.TrimSpace(clause)
		parts := strings.SplitN(clause, "=", 2)
		if len(parts) != 2 {
			return false
		}
		actual, ok := args[strings.TrimSpace(parts[0])]
		if !ok {
			return false
		}
		if fmt.Sprint(actual) != strings.TrimSpace(parts[1]) {
			return false
		}
	}
	return true
}
