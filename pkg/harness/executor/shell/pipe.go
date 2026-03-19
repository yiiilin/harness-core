package shell

import (
	"bytes"
	"context"
	"os/exec"
	"time"

	"github.com/yiiilin/harness-core/pkg/harness/action"
)

type PipeExecutor struct{}

func (PipeExecutor) Invoke(ctx context.Context, args map[string]any) (action.Result, error) {
	req := Request{
		Command: func() string { v, _ := args["command"].(string); return v }(),
		CWD:     func() string { v, _ := args["cwd"].(string); return v }(),
		TimeoutMS: func() int {
			v, _ := asInt(args["timeout_ms"])
			return v
		}(),
	}
	mode, _ := args["mode"].(string)
	req.Mode = mode
	return PipeExecutor{}.Execute(ctx, req)
}

func (PipeExecutor) Execute(ctx context.Context, req Request) (action.Result, error) {
	command := req.Command
	if command == "" {
		return action.Result{OK: false, Error: &action.Error{Code: "MISSING_COMMAND", Message: "command is required"}}, nil
	}
	cwd := req.CWD
	timeoutMS := req.TimeoutMS
	if timeoutMS <= 0 {
		timeoutMS = 30000
	}
	childCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(childCtx, "/bin/bash", "-lc", command)
	if cwd != "" {
		cmd.Dir = cwd
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start).Milliseconds()
	exitCode := 0
	status := "completed"
	if err != nil {
		status = "failed"
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if childCtx.Err() == context.DeadlineExceeded {
			status = "timed_out"
			exitCode = -1
		} else {
			exitCode = -1
		}
	}
	if err == nil {
		exitCode = 0
	}

	result := action.Result{
		OK: err == nil,
		Data: map[string]any{
			"mode":      "pipe",
			"command":   command,
			"cwd":       cwd,
			"stdout":    stdout.String(),
			"stderr":    stderr.String(),
			"exit_code": exitCode,
			"status":    status,
		},
		Meta: map[string]any{
			"duration_ms": duration,
		},
	}
	if err != nil {
		result.Error = &action.Error{Code: "COMMAND_FAILED", Message: err.Error()}
	}
	return result, nil
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
