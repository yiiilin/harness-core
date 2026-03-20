package main

import (
	"context"
	"testing"

	"github.com/yiiilin/harness-core/internal/postgrestest"
)

func TestRunWorkersDemo(t *testing.T) {
	pg := postgrestest.Start(t)

	result, err := RunWorkersDemo(context.Background(), pg.DSN)
	if err != nil {
		t.Fatalf("run workers demo: %v", err)
	}

	if len(result.Workers) != 2 {
		t.Fatalf("expected two worker results, got %#v", result)
	}
	if result.Runnable.SessionID == "" || result.Recoverable.SessionID == "" {
		t.Fatalf("expected both seeded sessions to be tracked, got %#v", result)
	}
	if result.AttemptCount != 2 {
		t.Fatalf("expected two persisted attempts, got %#v", result)
	}
	if result.TotalRenewals < 1 {
		t.Fatalf("expected at least one lease renewal, got %#v", result)
	}
	for _, worker := range result.Workers {
		if worker.Mode == "" || worker.Session.SessionID == "" {
			t.Fatalf("expected worker claim metadata, got %#v", worker)
		}
		if worker.FinalLeaseID != "" {
			t.Fatalf("expected lease release after worker completion, got %#v", worker)
		}
		if worker.Output == "" {
			t.Fatalf("expected worker execution output, got %#v", worker)
		}
	}
}
