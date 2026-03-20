package postgres_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/yiiilin/harness-core/internal/postgrestest"
	"github.com/yiiilin/harness-core/pkg/harness/builtins"
	hpostgres "github.com/yiiilin/harness-core/pkg/harness/postgres"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
)

func TestOpenDBRequiresDSN(t *testing.T) {
	if _, err := hpostgres.OpenDB(context.Background(), "   "); err == nil {
		t.Fatalf("expected empty DSN to be rejected")
	}
}

func TestBuildOptionsWiresDurableRuntimeStores(t *testing.T) {
	pg := postgrestest.Start(t)
	db, err := hpostgres.OpenDB(context.Background(), pg.DSN)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	var opts hruntime.Options
	builtins.Register(&opts)

	durable := hpostgres.BuildOptions(db, opts)
	if durable.StorageMode != "postgres" {
		t.Fatalf("expected postgres storage mode, got %q", durable.StorageMode)
	}
	if durable.Sessions == nil || durable.Tasks == nil || durable.Plans == nil || durable.Audit == nil {
		t.Fatalf("expected core durable stores to be wired")
	}
	if durable.Attempts == nil || durable.Actions == nil || durable.Verifications == nil || durable.Artifacts == nil {
		t.Fatalf("expected execution fact stores to be wired")
	}
	if durable.RuntimeHandles == nil || durable.CapabilitySnapshots == nil || durable.ContextSummaries == nil || durable.PlanningRecords == nil {
		t.Fatalf("expected advanced durable stores to be wired")
	}
	if durable.Runner == nil {
		t.Fatalf("expected transactional runner to be wired")
	}
	if durable.EventSink == nil {
		t.Fatalf("expected audit-backed event sink to be wired")
	}
}

func TestOpenServiceProvidesDurableService(t *testing.T) {
	pg := postgrestest.Start(t)
	var opts hruntime.Options
	builtins.Register(&opts)

	rt, db, err := hpostgres.OpenService(context.Background(), pg.DSN, opts)
	if err != nil {
		t.Fatalf("open service: %v", err)
	}
	defer db.Close()

	if rt.StorageMode != "postgres" {
		t.Fatalf("expected postgres storage mode, got %q", rt.StorageMode)
	}
	if _, err := rt.CreateSession("public postgres bootstrap", "persist a session through public bootstrap"); err != nil {
		t.Fatalf("create session: %v", err)
	}
	sessions, err := rt.ListSessions()
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected one persisted session, got %d", len(sessions))
	}
}

func TestApplyMigrationsReportsLatestVersionAndIsIdempotent(t *testing.T) {
	pg := postgrestest.Start(t)
	db, err := hpostgres.OpenDB(context.Background(), pg.DSN)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := hpostgres.ApplyMigrations(context.Background(), db); err != nil {
		t.Fatalf("apply migrations first pass: %v", err)
	}
	version, err := hpostgres.SchemaVersion(context.Background(), db)
	if err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version != hpostgres.LatestSchemaVersion() {
		t.Fatalf("expected latest schema version %q, got %q", hpostgres.LatestSchemaVersion(), version)
	}

	countBefore := mustCountMigrations(t, db)
	if err := hpostgres.ApplyMigrations(context.Background(), db); err != nil {
		t.Fatalf("apply migrations second pass: %v", err)
	}
	countAfter := mustCountMigrations(t, db)
	if countAfter != countBefore {
		t.Fatalf("expected idempotent migration apply, counts %d then %d", countBefore, countAfter)
	}
}

func TestApplySchemaRemainsCompatibleWithMigrationPath(t *testing.T) {
	pg := postgrestest.Start(t)
	db, err := hpostgres.OpenDB(context.Background(), pg.DSN)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := hpostgres.ApplySchema(context.Background(), db); err != nil {
		t.Fatalf("apply schema compatibility path: %v", err)
	}
	version, err := hpostgres.SchemaVersion(context.Background(), db)
	if err != nil {
		t.Fatalf("read schema version after ApplySchema: %v", err)
	}
	if version != hpostgres.LatestSchemaVersion() {
		t.Fatalf("expected ApplySchema to converge to latest version %q, got %q", hpostgres.LatestSchemaVersion(), version)
	}
}

func TestMigrationInspectionReportsAppliedPendingAndDrift(t *testing.T) {
	pg := postgrestest.Start(t)
	db, err := hpostgres.OpenDB(context.Background(), pg.DSN)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	embedded := hpostgres.EmbeddedMigrations()
	if len(embedded) == 0 {
		t.Fatalf("expected embedded migrations to be discoverable")
	}

	statuses, err := hpostgres.ListMigrationStatus(context.Background(), db)
	if err != nil {
		t.Fatalf("list migration status: %v", err)
	}
	if len(statuses) != len(embedded) {
		t.Fatalf("expected %d migration statuses, got %d", len(embedded), len(statuses))
	}
	for _, status := range statuses {
		if !status.Applied {
			t.Fatalf("expected prepared database migration %s to be applied, got %#v", status.Version, status)
		}
		if status.AppliedAt == 0 {
			t.Fatalf("expected applied migration %s to have applied_at, got %#v", status.Version, status)
		}
	}

	pending, err := hpostgres.PendingMigrations(context.Background(), db)
	if err != nil {
		t.Fatalf("list pending migrations: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected no pending migrations, got %#v", pending)
	}

	drift, err := hpostgres.HasSchemaDrift(context.Background(), db)
	if err != nil {
		t.Fatalf("check schema drift: %v", err)
	}
	if drift {
		t.Fatalf("expected prepared database to be current")
	}

	if _, err := db.ExecContext(context.Background(), "DELETE FROM harness_schema_migrations WHERE version = $1", hpostgres.LatestSchemaVersion()); err != nil {
		t.Fatalf("delete latest migration row: %v", err)
	}

	statuses, err = hpostgres.ListMigrationStatus(context.Background(), db)
	if err != nil {
		t.Fatalf("list migration status after drift: %v", err)
	}
	latest := findMigrationStatus(t, statuses, hpostgres.LatestSchemaVersion())
	if latest.Applied {
		t.Fatalf("expected latest migration to become pending after row deletion, got %#v", latest)
	}
	if latest.AppliedAt != 0 {
		t.Fatalf("expected pending migration to have zero applied_at, got %#v", latest)
	}

	pending, err = hpostgres.PendingMigrations(context.Background(), db)
	if err != nil {
		t.Fatalf("list pending migrations after drift: %v", err)
	}
	if len(pending) != 1 || pending[0].Version != hpostgres.LatestSchemaVersion() {
		t.Fatalf("expected latest migration to be pending, got %#v", pending)
	}

	drift, err = hpostgres.HasSchemaDrift(context.Background(), db)
	if err != nil {
		t.Fatalf("check schema drift after row deletion: %v", err)
	}
	if !drift {
		t.Fatalf("expected schema drift after latest migration row deletion")
	}
}

func mustCountMigrations(t *testing.T, db *sql.DB) int {
	t.Helper()

	var count int
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM harness_schema_migrations").Scan(&count); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	return count
}

func findMigrationStatus(t *testing.T, items []hpostgres.MigrationStatus, version string) hpostgres.MigrationStatus {
	t.Helper()
	for _, item := range items {
		if item.Version == version {
			return item
		}
	}
	t.Fatalf("expected migration status for version %s, got %#v", version, items)
	return hpostgres.MigrationStatus{}
}
