package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/yiiilin/harness-core/internal/postgrestest"
	hpostgres "github.com/yiiilin/harness-core/pkg/harness/postgres"
)

func TestRunMigrateStatusReportsPendingMigrations(t *testing.T) {
	pg := postgrestest.Start(t)
	db, err := hpostgres.OpenDB(context.Background(), pg.DSN)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if _, err := db.ExecContext(context.Background(), "DELETE FROM harness_schema_migrations WHERE version = $1", hpostgres.LatestSchemaVersion()); err != nil {
		t.Fatalf("delete latest migration row: %v", err)
	}

	t.Setenv("HARNESS_STORAGE_MODE", "postgres")
	t.Setenv("HARNESS_POSTGRES_DSN", pg.DSN)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := run(context.Background(), []string{"migrate", "status"}, &stdout, &stderr); err != nil {
		t.Fatalf("run migrate status: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "up_to_date=false") {
		t.Fatalf("expected status output to report drift, got %q", output)
	}
	if !strings.Contains(output, "pending") || !strings.Contains(output, hpostgres.LatestSchemaVersion()) {
		t.Fatalf("expected status output to list latest migration as pending, got %q", output)
	}
}

func TestRunMigrateUpAppliesPendingMigrations(t *testing.T) {
	pg := postgrestest.Start(t)
	db, err := hpostgres.OpenDB(context.Background(), pg.DSN)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if _, err := db.ExecContext(context.Background(), "DELETE FROM harness_schema_migrations WHERE version = $1", hpostgres.LatestSchemaVersion()); err != nil {
		t.Fatalf("delete latest migration row: %v", err)
	}

	t.Setenv("HARNESS_STORAGE_MODE", "postgres")
	t.Setenv("HARNESS_POSTGRES_DSN", pg.DSN)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := run(context.Background(), []string{"migrate", "up"}, &stdout, &stderr); err != nil {
		t.Fatalf("run migrate up: %v", err)
	}

	version, err := hpostgres.SchemaVersion(context.Background(), db)
	if err != nil {
		t.Fatalf("read schema version after migrate up: %v", err)
	}
	if version != hpostgres.LatestSchemaVersion() {
		t.Fatalf("expected migrate up to apply latest version %q, got %q", hpostgres.LatestSchemaVersion(), version)
	}
	if !strings.Contains(stdout.String(), hpostgres.LatestSchemaVersion()) {
		t.Fatalf("expected migrate up output to mention latest version, got %q", stdout.String())
	}
}

func TestRunMigrateVersionReportsCurrentAndLatest(t *testing.T) {
	pg := postgrestest.Start(t)

	t.Setenv("HARNESS_STORAGE_MODE", "postgres")
	t.Setenv("HARNESS_POSTGRES_DSN", pg.DSN)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := run(context.Background(), []string{"migrate", "version"}, &stdout, &stderr); err != nil {
		t.Fatalf("run migrate version: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "current="+hpostgres.LatestSchemaVersion()) {
		t.Fatalf("expected version output to include current version, got %q", output)
	}
	if !strings.Contains(output, "latest="+hpostgres.LatestSchemaVersion()) {
		t.Fatalf("expected version output to include latest version, got %q", output)
	}
}
