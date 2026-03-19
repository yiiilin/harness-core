package shellmodule_test

import (
	"testing"

	shellmodule "github.com/yiiilin/harness-core/modules/shell"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

func TestRegisterShellModule(t *testing.T) {
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	shellmodule.Register(tools, verifiers)
	if _, ok := tools.Get("shell.exec"); !ok {
		t.Fatalf("expected shell.exec to be registered")
	}
	if len(verifiers.List()) < 2 {
		t.Fatalf("expected shell verifiers to be registered")
	}
}

func TestDefaultPolicyRules(t *testing.T) {
	rules := shellmodule.DefaultPolicyRules()
	if len(rules) == 0 {
		t.Fatalf("expected non-empty policy rules")
	}
}
