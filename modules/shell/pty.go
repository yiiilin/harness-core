package shellmodule

import (
	"context"
	"time"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	shellexec "github.com/yiiilin/harness-core/pkg/harness/executor/shell"
)

type PTYBackend struct {
	Manager *PTYManager
}

func (b PTYBackend) Execute(ctx context.Context, req shellexec.Request) (action.Result, error) {
	if b.Manager == nil {
		return action.Result{
			OK: false,
			Data: map[string]any{
				"mode":   "pty",
				"status": "unsupported",
			},
			Error: &action.Error{Code: "PTY_NOT_CONFIGURED", Message: "pty manager is not configured"},
		}, nil
	}

	startedAt := time.Now()
	started, err := b.Manager.Start(ctx, req.Command, PTYStartOptions{
		CWD:      req.CWD,
		Env:      stringifyEnv(req.Env),
		Metadata: cloneAnyMap(req.Metadata),
	})
	if err != nil {
		return action.Result{
			OK: false,
			Data: map[string]any{
				"mode":   "pty",
				"status": "start_failed",
			},
			Error: &action.Error{Code: "PTY_START_FAILED", Message: err.Error()},
		}, nil
	}

	return action.Result{
		OK: true,
		Data: map[string]any{
			"mode":           "pty",
			"command":        req.Command,
			"cwd":            req.CWD,
			"status":         "active",
			"runtime_handle": started.RuntimeHandle,
			"shell_stream":   started.Stream,
			"pid":            started.PID,
		},
		Meta: map[string]any{
			"duration_ms": time.Since(startedAt).Milliseconds(),
			"interactive": true,
		},
	}, nil
}
