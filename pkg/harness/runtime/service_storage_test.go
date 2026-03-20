package runtime_test

import (
	"encoding/json"
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

func TestRuntimeInfoOmitsAdapterAndAuthMetadata(t *testing.T) {
	rt := hruntime.New(hruntime.Options{})
	info := rt.RuntimeInfo()

	raw, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("marshal runtime info: %v", err)
	}

	payload := map[string]any{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal runtime info: %v", err)
	}
	if _, ok := payload["transport"]; ok {
		t.Fatalf("expected runtime info to omit transport metadata, got %#v", payload)
	}
	if _, ok := payload["auth_mode"]; ok {
		t.Fatalf("expected runtime info to omit auth metadata, got %#v", payload)
	}
}
