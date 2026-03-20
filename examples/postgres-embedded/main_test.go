package main

import (
	"context"
	"strings"
	"testing"

	"github.com/yiiilin/harness-core/internal/postgrestest"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

func TestRunEmbeddedDemo(t *testing.T) {
	pg := postgrestest.Start(t)

	result, err := RunEmbeddedDemo(context.Background(), pg.DSN)
	if err != nil {
		t.Fatalf("run embedded demo: %v", err)
	}

	if result.StorageMode != "postgres" {
		t.Fatalf("expected postgres storage mode, got %#v", result)
	}
	if result.Session.SessionID == "" || result.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected completed durable session, got %#v", result.Session)
	}
	if result.AttemptCount != 1 {
		t.Fatalf("expected one persisted attempt, got %#v", result)
	}
	if !strings.Contains(result.Output, "hello from durable runtime") {
		t.Fatalf("expected durable shell output, got %#v", result)
	}
}
