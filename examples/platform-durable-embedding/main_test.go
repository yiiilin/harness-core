package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/yiiilin/harness-core/internal/postgrestest"
	hpostgres "github.com/yiiilin/harness-core/pkg/harness/postgres"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

func TestRunDurableEmbeddingDemoSurvivesRestartAndApprovalResume(t *testing.T) {
	pg := postgrestest.Start(t)

	result, err := RunDurableEmbeddingDemo(context.Background(), hpostgres.Config{
		DSN:             pg.DSN,
		Schema:          testSchemaName("platform_embed"),
		ApplyMigrations: true,
		MaxOpenConns:    4,
		MaxIdleConns:    2,
		ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		t.Fatalf("run durable embedding demo: %v", err)
	}

	if result.RunID == "" || result.SessionID == "" {
		t.Fatalf("expected non-empty run and session ids, got %#v", result)
	}
	if result.MappedSessionID != result.SessionID {
		t.Fatalf("expected run mapping to resolve same session, got %#v", result)
	}
	if result.ApprovalID == "" {
		t.Fatalf("expected approval id, got %#v", result)
	}
	if result.FinalPhase != session.PhaseComplete {
		t.Fatalf("expected resumed session to complete, got %#v", result)
	}
	if !strings.Contains(result.Output, "approved durable embedding") {
		t.Fatalf("expected resumed output to contain approval marker, got %#v", result)
	}
	if result.ActionCount != 1 {
		t.Fatalf("expected one persisted action, got %#v", result)
	}
}

func testSchemaName(prefix string) string {
	return fmt.Sprintf("hc_%s_%d", prefix, time.Now().UnixNano())
}
