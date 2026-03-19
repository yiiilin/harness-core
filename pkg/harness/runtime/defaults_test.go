package runtime_test

import (
	"testing"

	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
)

func TestRegisterBuiltinsMutatesOptions(t *testing.T) {
	opts := hruntime.Options{}
	hruntime.RegisterBuiltins(&opts)
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
