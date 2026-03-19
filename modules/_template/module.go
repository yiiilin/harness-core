package templatemodule

import (
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

// Register adds the module's tools and verifiers into the provided registries.
func Register(tools *tool.Registry, verifiers *verify.Registry) {
	_ = tools
	_ = verifiers
	// Register module tools here.
	// Register module verifiers here.
}

// DefaultPolicyRules returns suggested default rules for this module.
func DefaultPolicyRules() []permission.Rule {
	return nil
}
