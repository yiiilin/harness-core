package config_test

import (
	"testing"

	"github.com/yiiilin/harness-core/internal/config"
)

func TestLoadDefaultsIncludesStorageMode(t *testing.T) {
	t.Setenv("HARNESS_ADDR", "")
	t.Setenv("HARNESS_SHARED_TOKEN", "")
	t.Setenv("HARNESS_STORAGE_MODE", "")
	t.Setenv("HARNESS_POSTGRES_DSN", "")

	cfg := config.Load()
	if cfg.Addr != "127.0.0.1:8787" {
		t.Fatalf("expected default addr, got %s", cfg.Addr)
	}
	if cfg.SharedToken != "dev-token" {
		t.Fatalf("expected default token, got %s", cfg.SharedToken)
	}
	if cfg.StorageMode != "memory" {
		t.Fatalf("expected default storage mode memory, got %s", cfg.StorageMode)
	}
	if cfg.PostgresDSN != "" {
		t.Fatalf("expected empty default Postgres DSN, got %q", cfg.PostgresDSN)
	}
}

func TestLoadReadsDurableRuntimeSettings(t *testing.T) {
	t.Setenv("HARNESS_STORAGE_MODE", "postgres")
	t.Setenv("HARNESS_POSTGRES_DSN", "postgres://tester:pw@127.0.0.1:5432/harness_test?sslmode=disable")

	cfg := config.Load()
	if cfg.StorageMode != "postgres" {
		t.Fatalf("expected postgres storage mode, got %s", cfg.StorageMode)
	}
	if cfg.PostgresDSN == "" {
		t.Fatalf("expected postgres DSN to be populated")
	}
}
