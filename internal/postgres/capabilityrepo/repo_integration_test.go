package capabilityrepo_test

import (
	"context"
	"testing"

	"github.com/yiiilin/harness-core/internal/postgres/capabilityrepo"
	"github.com/yiiilin/harness-core/internal/postgresruntime"
	"github.com/yiiilin/harness-core/internal/postgrestest"
	"github.com/yiiilin/harness-core/pkg/harness/capability"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
)

func TestSnapshotRepoPersistsFrozenViewFieldsAgainstPostgres(t *testing.T) {
	pg := postgrestest.Start(t)
	db, err := postgresruntime.OpenDB(context.Background(), pg.DSN)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	repo := capabilityrepo.New(db)
	created, err := repo.Create(capability.Snapshot{
		SnapshotID:     "cap_pg_frozen_view",
		SessionID:      "sess_pg_frozen",
		TaskID:         "task_pg_frozen",
		PlanID:         "plan_pg_frozen",
		ViewID:         "view_pg_frozen",
		Scope:          capability.SnapshotScopePlan,
		ToolName:       "shell.exec",
		Version:        "v2",
		CapabilityType: "executor",
		RiskLevel:      tool.RiskHigh,
		Metadata:       map[string]any{"module": "shell"},
		ResolvedAt:     10,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.Get(created.SnapshotID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.PlanID != created.PlanID || got.ViewID != created.ViewID || got.Scope != capability.SnapshotScopePlan {
		t.Fatalf("unexpected persisted snapshot: %#v", got)
	}
	if got.RiskLevel != tool.RiskHigh || got.Version != "v2" {
		t.Fatalf("unexpected persisted capability fields: %#v", got)
	}

	items, err := repo.List(created.SessionID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one persisted snapshot, got %#v", items)
	}
	if items[0].ViewID != created.ViewID || items[0].PlanID != created.PlanID || items[0].Scope != capability.SnapshotScopePlan {
		t.Fatalf("unexpected listed snapshot: %#v", items[0])
	}
}
