package runtime

import (
	filesystemmodule "github.com/yiiilin/harness-core/modules/filesystem"
	httpmodule "github.com/yiiilin/harness-core/modules/http"
	shellmodule "github.com/yiiilin/harness-core/modules/shell"
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
)

func RegisterBuiltins(opts *Options) {
	if opts == nil {
		return
	}
	hasCustomPolicy := opts.Policy != nil
	resolved := WithDefaults(*opts)
	shellmodule.Register(resolved.Tools, resolved.Verifiers)
	filesystemmodule.Register(resolved.Tools, resolved.Verifiers)
	httpmodule.Register(resolved.Tools, resolved.Verifiers)
	resolved.Tools.Register(tool.Definition{ToolName: "windows.native", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskHigh, Enabled: false, Metadata: map[string]any{"module": "windows"}}, nil)
	if !hasCustomPolicy {
		resolved.Policy = permission.RulesEvaluator{
			Rules: append(
				append(shellmodule.DefaultPolicyRules(), filesystemmodule.DefaultPolicyRules()...),
				httpmodule.DefaultPolicyRules()...,
			),
			Fallback: permission.DefaultEvaluator{},
		}
	}
	*opts = resolved
}
