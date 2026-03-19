package runtime

import (
	"context"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	shellexec "github.com/yiiilin/harness-core/pkg/harness/executor/shell"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type shellHandler struct{ exec shellexec.PipeExecutor }

func (h shellHandler) Invoke(ctx context.Context, args map[string]any) (action.Result, error) {
	return h.exec.Invoke(ctx, args)
}

func RegisterBuiltins(opts *Options) {
	if opts == nil {
		return
	}
	resolved := WithDefaults(*opts)
	resolved.Tools.Register(tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true}, shellHandler{exec: shellexec.PipeExecutor{}})
	resolved.Tools.Register(tool.Definition{ToolName: "windows.native", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskHigh, Enabled: false}, nil)
	resolved.Verifiers.Register(verify.Definition{Kind: "exit_code", Description: "Verify that an execution result exit code is in the allowed set."}, verify.ExitCodeChecker{})
	resolved.Verifiers.Register(verify.Definition{Kind: "output_contains", Description: "Verify that stdout or stderr contains a target substring."}, verify.OutputContainsChecker{})
	*opts = resolved
}
