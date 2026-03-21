package auditrepo_test

import (
	"context"
	"testing"

	"github.com/yiiilin/harness-core/internal/postgres/auditrepo"
	"github.com/yiiilin/harness-core/internal/postgrestest"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	hpostgres "github.com/yiiilin/harness-core/pkg/harness/postgres"
)

func TestAuditRepoListPreservesEmitOrderWhenTimestampsTie(t *testing.T) {
	pg := postgrestest.Start(t)
	db, err := hpostgres.OpenDB(context.Background(), pg.DSN)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	repo := auditrepo.New(db)
	if err := repo.Emit(audit.Event{
		EventID:   "evt_b",
		Type:      audit.EventToolCalled,
		SessionID: "sess_emit_order",
		StepID:    "step_emit_order",
		CreatedAt: 10,
	}); err != nil {
		t.Fatalf("emit first event: %v", err)
	}
	if err := repo.Emit(audit.Event{
		EventID:   "evt_a",
		Type:      audit.EventToolCompleted,
		SessionID: "sess_emit_order",
		StepID:    "step_emit_order",
		CreatedAt: 10,
	}); err != nil {
		t.Fatalf("emit second event: %v", err)
	}

	items, err := repo.List("sess_emit_order")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected two events, got %#v", items)
	}
	if items[0].EventID != "evt_b" || items[1].EventID != "evt_a" {
		t.Fatalf("expected emit order to be preserved when timestamps tie, got %#v", items)
	}
}
