package shellmodule

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type PTYHandleActiveChecker struct {
	Inspector PTYInspector
}

type PTYStreamContainsChecker struct {
	Inspector PTYInspector
}

type PTYExitCodeChecker struct {
	Inspector PTYInspector
}

func (c PTYHandleActiveChecker) Verify(_ context.Context, _ map[string]any, result action.Result, _ session.State) (verify.Result, error) {
	if c.Inspector == nil {
		return verify.Result{Success: false, Reason: "pty inspector not configured"}, nil
	}
	handleID, err := ptyHandleIDFromResult(result)
	if err != nil {
		return verify.Result{Success: false, Reason: err.Error()}, nil
	}
	inspect, err := c.Inspector.Inspect(context.Background(), handleID)
	if err != nil {
		return verify.Result{Success: false, Reason: err.Error()}, nil
	}
	success := !inspect.Closed
	reason := "pty handle is active"
	if !success {
		reason = "pty handle is not active"
	}
	return verify.Result{
		Success: success,
		Reason:  reason,
		Details: map[string]any{
			"handle_id": handleID,
			"closed":    inspect.Closed,
			"status":    inspect.Status,
		},
	}, nil
}

func (c PTYStreamContainsChecker) Verify(ctx context.Context, args map[string]any, result action.Result, _ session.State) (verify.Result, error) {
	if c.Inspector == nil {
		return verify.Result{Success: false, Reason: "pty inspector not configured"}, nil
	}
	needle, _ := args["text"].(string)
	if needle == "" {
		return verify.Result{Success: false, Reason: "missing text"}, nil
	}
	handleID, err := ptyHandleIDFromResult(result)
	if err != nil {
		return verify.Result{Success: false, Reason: err.Error()}, nil
	}

	timeout := ptyTimeoutFromArgs(args, time.Second)
	deadline := time.Now().Add(timeout)
	offset := int64(0)
	if v, ok := asInt64(args["offset"]); ok && v > 0 {
		offset = v
	}
	maxBytes := 4096
	if v, ok := asInt64(args["max_bytes"]); ok && v > 0 {
		maxBytes = int(v)
	}
	seen := ""
	for {
		read, err := c.Inspector.Read(ctx, handleID, PTYReadRequest{
			Offset:   offset,
			MaxBytes: maxBytes,
		})
		if err != nil {
			return verify.Result{Success: false, Reason: err.Error()}, nil
		}
		if read.NextOffset > offset {
			seen += read.Data
			offset = read.NextOffset
		}
		if strings.Contains(seen, needle) {
			return verify.Result{
				Success: true,
				Reason:  "text found in PTY stream",
				Details: map[string]any{
					"handle_id":   handleID,
					"text":        needle,
					"next_offset": offset,
				},
			}, nil
		}
		if read.Closed || time.Now().After(deadline) {
			break
		}
		select {
		case <-ctx.Done():
			return verify.Result{Success: false, Reason: ctx.Err().Error()}, ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
	return verify.Result{
		Success: false,
		Reason:  "text not found in PTY stream",
		Details: map[string]any{
			"handle_id": handleID,
			"text":      needle,
		},
	}, nil
}

func (c PTYExitCodeChecker) Verify(ctx context.Context, args map[string]any, result action.Result, _ session.State) (verify.Result, error) {
	if c.Inspector == nil {
		return verify.Result{Success: false, Reason: "pty inspector not configured"}, nil
	}
	allowedRaw, ok := args["allowed"]
	if !ok {
		return verify.Result{Success: false, Reason: "missing allowed exit codes"}, nil
	}
	allowedList, ok := allowedRaw.([]any)
	if !ok {
		return verify.Result{Success: false, Reason: "allowed must be an array"}, nil
	}
	handleID, err := ptyHandleIDFromResult(result)
	if err != nil {
		return verify.Result{Success: false, Reason: err.Error()}, nil
	}

	timeout := ptyTimeoutFromArgs(args, time.Second)
	deadline := time.Now().Add(timeout)
	for {
		inspect, err := c.Inspector.Inspect(ctx, handleID)
		if err != nil {
			return verify.Result{Success: false, Reason: err.Error()}, nil
		}
		if inspect.Closed {
			for _, item := range allowedList {
				candidate, ok := verifyAsInt(item)
				if ok && candidate == inspect.ExitCode {
					return verify.Result{
						Success: true,
						Reason:  "PTY exit code allowed",
						Details: map[string]any{
							"handle_id": handleID,
							"exit_code": inspect.ExitCode,
						},
					}, nil
				}
			}
			return verify.Result{
				Success: false,
				Reason:  fmt.Sprintf("exit_code %d not allowed", inspect.ExitCode),
				Details: map[string]any{
					"handle_id": handleID,
					"exit_code": inspect.ExitCode,
				},
			}, nil
		}
		if time.Now().After(deadline) {
			return verify.Result{
				Success: false,
				Reason:  "PTY did not exit before timeout",
				Details: map[string]any{
					"handle_id": handleID,
				},
			}, nil
		}
		select {
		case <-ctx.Done():
			return verify.Result{Success: false, Reason: ctx.Err().Error()}, ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
}

func ptyHandleIDFromResult(result action.Result) (string, error) {
	if raw, ok := result.Data["runtime_handle"]; ok {
		switch handle := raw.(type) {
		case execution.RuntimeHandle:
			if handle.HandleID != "" {
				return handle.HandleID, nil
			}
		case *execution.RuntimeHandle:
			if handle != nil && handle.HandleID != "" {
				return handle.HandleID, nil
			}
		case map[string]any:
			if id, _ := handle["handle_id"].(string); id != "" {
				return id, nil
			}
		}
	}
	if raw, ok := result.Data["shell_stream"]; ok {
		switch stream := raw.(type) {
		case PTYStreamInfo:
			if stream.HandleID != "" {
				return stream.HandleID, nil
			}
		case *PTYStreamInfo:
			if stream != nil && stream.HandleID != "" {
				return stream.HandleID, nil
			}
		case map[string]any:
			if id, _ := stream["handle_id"].(string); id != "" {
				return id, nil
			}
		}
	}
	return "", fmt.Errorf("result missing PTY handle id")
}

func ptyTimeoutFromArgs(args map[string]any, fallback time.Duration) time.Duration {
	if v, ok := asInt64(args["timeout_ms"]); ok && v > 0 {
		return time.Duration(v) * time.Millisecond
	}
	return fallback
}

func asInt64(v any) (int64, bool) {
	switch x := v.(type) {
	case int:
		return int64(x), true
	case int32:
		return int64(x), true
	case int64:
		return x, true
	case float32:
		return int64(x), true
	case float64:
		return int64(x), true
	default:
		return 0, false
	}
}

func verifyAsInt(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int32:
		return int(x), true
	case int64:
		return int(x), true
	case float32:
		return int(x), true
	case float64:
		return int(x), true
	default:
		return 0, false
	}
}
