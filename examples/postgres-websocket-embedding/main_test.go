package main

import (
	"context"
	"strings"
	"testing"

	"github.com/yiiilin/harness-core/internal/postgrestest"
	hpostgres "github.com/yiiilin/harness-core/pkg/harness/postgres"
)

func TestRunPostgresWebSocketEmbeddingDemo(t *testing.T) {
	pg := postgrestest.Start(t)

	result, err := RunPostgresWebSocketEmbeddingDemo(context.Background(), hpostgresTestConfig(pg.DSN))
	if err != nil {
		t.Fatalf("run postgres websocket embedding demo: %v", err)
	}
	if result.StorageMode != "postgres" {
		t.Fatalf("expected postgres storage mode, got %#v", result)
	}
	if !result.InteractiveConfigured {
		t.Fatalf("expected builtins to wire an interactive controller, got %#v", result)
	}
	if result.SessionID == "" || result.HandleID == "" {
		t.Fatalf("expected durable session and interactive handle ids, got %#v", result)
	}
	if !strings.Contains(result.Echo, "hello from postgres websocket example") {
		t.Fatalf("expected echoed interactive output, got %#v", result)
	}
}

func hpostgresTestConfig(dsn string) hpostgres.Config {
	return hpostgres.Config{
		DSN:             dsn,
		Schema:          "postgres_websocket_embedding",
		MaxOpenConns:    4,
		MaxIdleConns:    2,
		ApplyMigrations: true,
	}
}
