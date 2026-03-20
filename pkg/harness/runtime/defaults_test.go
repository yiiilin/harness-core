package runtime_test

import (
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/builtins"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
)

func TestBareRuntimeDefaultsDoNotRegisterBuiltinModules(t *testing.T) {
	opts := hruntime.WithDefaults(hruntime.Options{})
	if len(opts.Tools.List()) != 0 {
		t.Fatalf("expected bare runtime defaults to keep builtin modules out, got %d tools", len(opts.Tools.List()))
	}
	if len(opts.Verifiers.List()) != 0 {
		t.Fatalf("expected bare runtime defaults to keep builtin verifiers out, got %d verifiers", len(opts.Verifiers.List()))
	}
}

func TestBuiltinsRegisterMutatesOptions(t *testing.T) {
	opts := hruntime.Options{}
	builtins.Register(&opts)
	if opts.Tools == nil {
		t.Fatalf("expected tools registry to be initialized")
	}
	if opts.Verifiers == nil {
		t.Fatalf("expected verifier registry to be initialized")
	}
	tools := opts.Tools.List()
	if len(tools) < 2 {
		t.Fatalf("expected builtin tools to be registered, got %d", len(tools))
	}
	verifiers := opts.Verifiers.List()
	if len(verifiers) < 2 {
		t.Fatalf("expected builtin verifiers to be registered, got %d", len(verifiers))
	}
}
