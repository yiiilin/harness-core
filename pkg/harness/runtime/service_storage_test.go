package runtime_test

import (
	"testing"

	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
)

func TestRuntimeInfoReflectsConfiguredStorageMode(t *testing.T) {
	rt := hruntime.New(hruntime.Options{StorageMode: "postgres"})
	info := rt.RuntimeInfo()
	if info.StorageMode != "postgres" {
		t.Fatalf("expected runtime info storage mode postgres, got %s", info.StorageMode)
	}
}

func TestRuntimeDefaultsToInMemoryStorageMode(t *testing.T) {
	rt := hruntime.New(hruntime.Options{})
	info := rt.RuntimeInfo()
	if info.StorageMode != "in-memory-dev" {
		t.Fatalf("expected default storage mode in-memory-dev, got %s", info.StorageMode)
	}
}
