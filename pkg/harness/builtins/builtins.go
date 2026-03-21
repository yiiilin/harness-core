package builtins

import (
	filesystemmodule "github.com/yiiilin/harness-core/modules/filesystem"
	httpmodule "github.com/yiiilin/harness-core/modules/http"
	shellmodule "github.com/yiiilin/harness-core/modules/shell"
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	hverify "github.com/yiiilin/harness-core/pkg/harness/verify"
)

// Register wires the default built-in capability modules into runtime options.
// This is a composition helper, not part of the bare runtime kernel.
func Register(opts *hruntime.Options) {
	if opts == nil {
		return
	}
	hasCustomPolicy := opts.Policy != nil
	resolved := hruntime.WithDefaults(*opts)
	hverify.RegisterBuiltins(resolved.Verifiers)
	shellmodule.Register(resolved.Tools, resolved.Verifiers)
	filesystemmodule.Register(resolved.Tools, resolved.Verifiers)
	httpmodule.Register(resolved.Tools, resolved.Verifiers)
	resolved.Tools.Register(tool.Definition{
		ToolName:       "windows.native",
		Version:        "v1",
		CapabilityType: "executor",
		RiskLevel:      tool.RiskHigh,
		Enabled:        false,
		Metadata:       map[string]any{"module": "windows"},
	}, nil)
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

// New constructs a runtime with default in-memory components plus the built-in capability modules.
func New() *hruntime.Service {
	opts := hruntime.Options{}
	Register(&opts)
	return hruntime.New(opts)
}
