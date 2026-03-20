package postgres

import (
	"context"
	"database/sql"

	internalpostgres "github.com/yiiilin/harness-core/internal/postgres"
)

type MigrationInfo struct {
	Version string `json:"version"`
	Name    string `json:"name"`
}

type MigrationStatus struct {
	Version   string `json:"version"`
	Name      string `json:"name"`
	Applied   bool   `json:"applied"`
	AppliedAt int64  `json:"applied_at,omitempty"`
}

// EmbeddedMigrations reports the canonical embedded migration set in order.
func EmbeddedMigrations() []MigrationInfo {
	items := internalpostgres.Migrations()
	out := make([]MigrationInfo, 0, len(items))
	for _, item := range items {
		out = append(out, MigrationInfo{
			Version: item.Version,
			Name:    item.Name,
		})
	}
	return out
}

// ListMigrationStatus reports each embedded migration together with its applied state.
func ListMigrationStatus(ctx context.Context, db SchemaApplier) ([]MigrationStatus, error) {
	embedded := EmbeddedMigrations()
	out := make([]MigrationStatus, 0, len(embedded))

	exists, err := migrationTableExists(ctx, db)
	if err != nil {
		return nil, err
	}

	for _, item := range embedded {
		status := MigrationStatus{
			Version: item.Version,
			Name:    item.Name,
		}
		if exists {
			var appliedAt int64
			switch err := db.QueryRowContext(ctx, `
				SELECT applied_at
				FROM harness_schema_migrations
				WHERE version = $1
			`, item.Version).Scan(&appliedAt); err {
			case nil:
				status.Applied = true
				status.AppliedAt = appliedAt
			case sql.ErrNoRows:
			default:
				return nil, err
			}
		}
		out = append(out, status)
	}
	return out, nil
}

// PendingMigrations reports the embedded migrations that have not been applied yet.
func PendingMigrations(ctx context.Context, db SchemaApplier) ([]MigrationInfo, error) {
	statuses, err := ListMigrationStatus(ctx, db)
	if err != nil {
		return nil, err
	}
	out := make([]MigrationInfo, 0, len(statuses))
	for _, status := range statuses {
		if status.Applied {
			continue
		}
		out = append(out, MigrationInfo{
			Version: status.Version,
			Name:    status.Name,
		})
	}
	return out, nil
}

// HasSchemaDrift reports whether the database version differs from the latest embedded migration version.
func HasSchemaDrift(ctx context.Context, db SchemaApplier) (bool, error) {
	current, err := SchemaVersion(ctx, db)
	if err != nil {
		return false, err
	}
	return current != LatestSchemaVersion(), nil
}
