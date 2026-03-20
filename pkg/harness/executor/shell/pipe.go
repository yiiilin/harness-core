package shell

import (
	"bytes"
	"context"
	"path/filepath"
	"os/exec"
	"strings"
	"time"

	"github.com/yiiilin/harness-core/pkg/harness/action"
)

const defaultMaxOutputBytes = 16 * 1024

type PipeExecutor struct {
	MaxOutputBytes       int
	AllowedCWDPrefixes   []string
	AllowedPathPrefixes  []string
}

func (e PipeExecutor) Invoke(ctx context.Context, args map[string]any) (action.Result, error) {
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
	return e.Execute(ctx, req)
}

func (e PipeExecutor) Execute(ctx context.Context, req Request) (action.Result, error) {
	command := req.Command
	if command == "" {
		return action.Result{OK: false, Error: &action.Error{Code: "MISSING_COMMAND", Message: "command is required"}}, nil
	}
	cwd := req.CWD
	if errResult, ok := e.validatePaths(command, cwd); ok {
		return errResult, nil
	}
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
	errorCode := ""
	if err != nil {
		if childCtx.Err() == context.DeadlineExceeded {
			status = "timed_out"
			exitCode = -1
			errorCode = "COMMAND_TIMED_OUT"
		} else if exitErr, ok := err.(*exec.ExitError); ok {
			status = "failed"
			exitCode = exitErr.ExitCode()
			errorCode = "COMMAND_EXIT_NONZERO"
		} else {
			status = "start_failed"
			exitCode = -1
			errorCode = "COMMAND_START_FAILED"
		}
	}
	if err == nil {
		exitCode = 0
	}

	stdoutText, stdoutMeta := truncateOutput(stdout.String(), e.maxOutputBytes())
	stderrText, stderrMeta := truncateOutput(stderr.String(), e.maxOutputBytes())

	result := action.Result{
		OK: err == nil,
		Data: map[string]any{
			"mode":      "pipe",
			"command":   command,
			"cwd":       cwd,
			"stdout":    stdoutText,
			"stderr":    stderrText,
			"exit_code": exitCode,
			"status":    status,
		},
		Meta: map[string]any{
			"duration_ms":          duration,
			"stdout_truncated":     stdoutMeta.truncated,
			"stdout_original_bytes": stdoutMeta.originalBytes,
			"stdout_returned_bytes": stdoutMeta.returnedBytes,
			"stderr_truncated":     stderrMeta.truncated,
			"stderr_original_bytes": stderrMeta.originalBytes,
			"stderr_returned_bytes": stderrMeta.returnedBytes,
		},
	}
	if err != nil {
		result.Error = &action.Error{Code: errorCode, Message: err.Error()}
	}
	return result, nil
}

func (e PipeExecutor) maxOutputBytes() int {
	if e.MaxOutputBytes > 0 {
		return e.MaxOutputBytes
	}
	return defaultMaxOutputBytes
}

func (e PipeExecutor) validatePaths(command, cwd string) (action.Result, bool) {
	if cwd != "" && len(e.AllowedCWDPrefixes) > 0 {
		absCWD, err := filepath.Abs(cwd)
		if err != nil || !hasAllowedPrefix(absCWD, e.AllowedCWDPrefixes) {
			return action.Result{
				OK: false,
				Data: map[string]any{
					"cwd":    cwd,
					"status": "blocked",
				},
				Error: &action.Error{Code: "CWD_NOT_ALLOWED", Message: "cwd is outside the allowed prefixes"},
			}, true
		}
	}

	if path := firstCommandPath(command); path != "" && len(e.AllowedPathPrefixes) > 0 {
		if !hasAllowedPrefix(path, e.AllowedPathPrefixes) {
			return action.Result{
				OK: false,
				Data: map[string]any{
					"command": command,
					"status":  "blocked",
				},
				Error: &action.Error{Code: "COMMAND_PATH_NOT_ALLOWED", Message: "command path is outside the allowed prefixes"},
			}, true
		}
	}

	return action.Result{}, false
}

func firstCommandPath(command string) string {
	fields := strings.Fields(strings.TrimSpace(command))
	if len(fields) == 0 {
		return ""
	}
	if !filepath.IsAbs(fields[0]) {
		return ""
	}
	return filepath.Clean(fields[0])
}

func hasAllowedPrefix(target string, prefixes []string) bool {
	cleanTarget := filepath.Clean(target)
	for _, prefix := range prefixes {
		if prefix == "" {
			continue
		}
		cleanPrefix := filepath.Clean(prefix)
		if cleanTarget == cleanPrefix || strings.HasPrefix(cleanTarget, cleanPrefix+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

type truncationMeta struct {
	truncated     bool
	originalBytes int
	returnedBytes int
}

func truncateOutput(text string, maxBytes int) (string, truncationMeta) {
	meta := truncationMeta{
		originalBytes: len(text),
		returnedBytes: len(text),
	}
	if maxBytes <= 0 || len(text) <= maxBytes {
		return text, meta
	}
	meta.truncated = true
	meta.returnedBytes = maxBytes
	return text[:maxBytes], meta
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
