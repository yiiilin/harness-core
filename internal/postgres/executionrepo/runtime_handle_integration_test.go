package executionrepo_test

import (
	"context"
	"testing"

	"github.com/yiiilin/harness-core/internal/postgres/executionrepo"
	"github.com/yiiilin/harness-core/internal/postgrestest"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	hpostgres "github.com/yiiilin/harness-core/pkg/harness/postgres"
)

func TestRuntimeHandleRepoPersistsLifecycleStateAgainstPostgres(t *testing.T) {
	pg := postgrestest.Start(t)
	db, err := hpostgres.OpenDB(context.Background(), pg.DSN)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	repo := executionrepo.NewRuntimeHandleStore(db)
	created, err := repo.Create(execution.RuntimeHandle{
		HandleID:     "hdl_pg_lifecycle",
		SessionID:    "sess_pg_runtime",
		TaskID:       "task_pg_runtime",
		AttemptID:    "att_pg_runtime",
		TraceID:      "trace_pg_runtime",
		Kind:         "pty",
		Value:        "pty-pg-runtime",
		Status:       execution.RuntimeHandleActive,
		StatusReason: "tool reported active",
		Metadata:     map[string]any{"origin": "integration-test"},
		CreatedAt:    10,
		UpdatedAt:    10,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	created.Status = execution.RuntimeHandleClosed
	created.StatusReason = "client closed"
	created.ClosedAt = 20
	created.UpdatedAt = 20
	created.Metadata["closed_by"] = "operator"
	if err := repo.Update(created); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := repo.Get(created.HandleID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != execution.RuntimeHandleClosed || got.ClosedAt != 20 || got.StatusReason != "client closed" {
		t.Fatalf("unexpected persisted runtime handle: %#v", got)
	}
	if got.Metadata["origin"] != "integration-test" || got.Metadata["closed_by"] != "operator" {
		t.Fatalf("expected lifecycle metadata to round-trip, got %#v", got.Metadata)
	}

	items, err := repo.List(created.SessionID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one runtime handle, got %#v", items)
	}
	if items[0].Status != execution.RuntimeHandleClosed || items[0].ClosedAt != 20 {
		t.Fatalf("unexpected listed runtime handle: %#v", items[0])
	}
}
