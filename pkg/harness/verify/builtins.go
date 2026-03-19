package verify

import (
	"context"
	"fmt"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

type ExitCodeChecker struct{}

func (ExitCodeChecker) Verify(_ context.Context, args map[string]any, result action.Result, _ session.State) (Result, error) {
	allowedRaw, ok := args["allowed"]
	if !ok {
		return Result{Success: false, Reason: "missing allowed exit codes"}, nil
	}
	allowedList, ok := allowedRaw.([]any)
	if !ok {
		return Result{Success: false, Reason: "allowed must be an array"}, nil
	}
	exitCodeRaw, ok := result.Data["exit_code"]
	if !ok {
		return Result{Success: false, Reason: "result missing exit_code"}, nil
	}
	exitCode, ok := asInt(exitCodeRaw)
	if !ok {
		return Result{Success: false, Reason: "exit_code is not numeric"}, nil
	}
	for _, item := range allowedList {
		candidate, ok := asInt(item)
		if ok && candidate == exitCode {
			return Result{Success: true, Details: map[string]any{"exit_code": exitCode}}, nil
		}
	}
	return Result{Success: false, Reason: fmt.Sprintf("exit_code %d not allowed", exitCode), Details: map[string]any{"exit_code": exitCode}}, nil
}

type OutputContainsChecker struct{}

func (OutputContainsChecker) Verify(_ context.Context, args map[string]any, result action.Result, _ session.State) (Result, error) {
	needle, _ := args["text"].(string)
	if needle == "" {
		return Result{Success: false, Reason: "missing text"}, nil
	}
	stdout, _ := result.Data["stdout"].(string)
	stderr, _ := result.Data["stderr"].(string)
	combined := stdout + "\n" + stderr
	if contains(combined, needle) {
		return Result{Success: true, Details: map[string]any{"text": needle}}, nil
	}
	return Result{Success: false, Reason: "text not found in output", Details: map[string]any{"text": needle}}, nil
}

func contains(s, needle string) bool {
	return len(needle) > 0 && len(s) >= len(needle) && (func() bool { return stringIndex(s, needle) >= 0 })()
}

func stringIndex(s, sep string) int {
	for i := 0; i+len(sep) <= len(s); i++ {
		if s[i:i+len(sep)] == sep {
			return i
		}
	}
	return -1
}

func asInt(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int32:
		return int(x), true
	case int64:
		return int(x), true
	case float64:
		return int(x), true
	case float32:
		return int(x), true
	default:
		return 0, false
	}
}
