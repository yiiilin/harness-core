package harness_test

import (
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness"
)

func TestNewDefaultProvidesCoreComponents(t *testing.T) {
	rt := harness.NewDefault()
	if rt == nil {
		t.Fatalf("expected runtime, got nil")
	}
	info := rt.RuntimeInfo()
	if !info.HasPlanner {
		t.Fatalf("expected default planner to be present")
	}
	if !info.HasContextAssembler {
		t.Fatalf("expected default context assembler to be present")
	}
	if !info.HasEventSink {
		t.Fatalf("expected default event sink to be present")
	}
}

func TestNewWithBuiltinsRegistersBuiltins(t *testing.T) {
	rt := harness.NewWithBuiltins()
	if len(rt.ListTools()) < 2 {
		t.Fatalf("expected built-in tools to be registered")
	}
	if len(rt.ListVerifiers()) < 2 {
		t.Fatalf("expected built-in verifiers to be registered")
	}
}
