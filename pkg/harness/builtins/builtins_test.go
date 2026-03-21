package builtins

import (
	"testing"

	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
)

func TestRegisterAppliesBuiltinModules(t *testing.T) {
	opts := hruntime.Options{}
	Register(&opts)
	if opts.Tools == nil {
		t.Fatalf("expected tools registry to be initialized")
	}
	if opts.Verifiers == nil {
		t.Fatalf("expected verifier registry to be initialized")
	}
	if len(opts.Tools.List()) < 2 {
		t.Fatalf("expected builtin tools to be registered, got %d", len(opts.Tools.List()))
	}
	if len(opts.Verifiers.List()) < 2 {
		t.Fatalf("expected builtin verifiers to be registered, got %d", len(opts.Verifiers.List()))
	}
	for _, kind := range []string{"value_equals", "string_matches_at", "tcp_port_open", "http_eventually_status_code"} {
		found := false
		for _, def := range opts.Verifiers.List() {
			if def.Kind == kind {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected builtin verifier %q to be registered", kind)
		}
	}
}

func TestNewBuildsConvenienceRuntimeWithBuiltinModules(t *testing.T) {
	rt := New()
	if len(rt.ListTools()) < 2 {
		t.Fatalf("expected built-in tools to be registered")
	}
	if len(rt.ListVerifiers()) < 2 {
		t.Fatalf("expected built-in verifiers to be registered")
	}
}
