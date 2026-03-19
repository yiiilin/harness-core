package runtime

import (
	filesystemmodule "github.com/yiiilin/harness-core/modules/filesystem"
	httpmodule "github.com/yiiilin/harness-core/modules/http"
	shellmodule "github.com/yiiilin/harness-core/modules/shell"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
)

func RegisterBuiltins(opts *Options) {
	if opts == nil {
		return
	}
	resolved := WithDefaults(*opts)
	shellmodule.Register(resolved.Tools, resolved.Verifiers)
	filesystemmodule.Register(resolved.Tools, resolved.Verifiers)
	httpmodule.Register(resolved.Tools, resolved.Verifiers)
	resolved.Tools.Register(tool.Definition{ToolName: "windows.native", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskHigh, Enabled: false, Metadata: map[string]any{"module": "windows"}}, nil)
	*opts = resolved
}
