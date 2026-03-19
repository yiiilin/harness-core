package shellmodule

import (
	"context"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	shellexec "github.com/yiiilin/harness-core/pkg/harness/executor/shell"
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type handler struct{ exec shellexec.PipeExecutor }

func (h handler) Invoke(ctx context.Context, args map[string]any) (action.Result, error) {
	return h.exec.Invoke(ctx, args)
}

func Register(tools *tool.Registry, verifiers *verify.Registry) {
	if tools != nil {
		tools.Register(tool.Definition{
			ToolName:       "shell.exec",
			Version:        "v1",
			CapabilityType: "executor",
			RiskLevel:      tool.RiskMedium,
			Enabled:        true,
			Metadata: map[string]any{
				"module": "shell",
				"modes":  []string{"pipe"},
			},
		}, handler{exec: shellexec.PipeExecutor{}})
	}
	if verifiers != nil {
		verifiers.Register(verify.Definition{Kind: "exit_code", Description: "Verify that an execution result exit code is in the allowed set."}, verify.ExitCodeChecker{})
		verifiers.Register(verify.Definition{Kind: "output_contains", Description: "Verify that stdout or stderr contains a target substring."}, verify.OutputContainsChecker{})
	}
}

func DefaultPolicyRules() []permission.Rule {
	return []permission.Rule{
		{Permission: "shell.exec", Pattern: "mode=pipe", Action: permission.Allow},
		{Permission: "shell.exec", Pattern: "*", Action: permission.Ask},
	}
}
