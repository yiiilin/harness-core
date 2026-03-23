package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	_ "github.com/lib/pq"
	internalpostgres "github.com/yiiilin/harness-core/internal/postgres"
	"github.com/yiiilin/harness-core/internal/postgres/approvalrepo"
	"github.com/yiiilin/harness-core/internal/postgres/auditrepo"
	"github.com/yiiilin/harness-core/internal/postgres/capabilityrepo"
	"github.com/yiiilin/harness-core/internal/postgres/contextrepo"
	"github.com/yiiilin/harness-core/internal/postgres/executionrepo"
	"github.com/yiiilin/harness-core/internal/postgres/planningrepo"
	"github.com/yiiilin/harness-core/internal/postgres/planrepo"
	"github.com/yiiilin/harness-core/internal/postgres/sessionrepo"
	"github.com/yiiilin/harness-core/internal/postgres/taskrepo"
	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/capability"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/persistence"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/planning"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

// SchemaApplier is the minimal SQL execution surface required to apply the
// canonical Postgres schema. *sql.DB and *sql.Tx both satisfy this contract.
type SchemaApplier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Config is the public schema-aware durable bootstrap configuration for
// embedding harness-core on Postgres.
type Config struct {
	DSN             string
	Schema          string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ApplyMigrations bool
}

var schemaNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_$]*$`)

// OpenDB opens and pings a Postgres database handle for durable runtime use.
func OpenDB(ctx context.Context, dsn string) (*sql.DB, error) {
	return openAndPingDB(ctx, dsn, 0, 0, 0)
}

type txBeginner interface {
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

// ApplyMigrations applies the canonical harness-core Postgres migrations.
func ApplyMigrations(ctx context.Context, db SchemaApplier) error {
	if beginner, ok := db.(txBeginner); ok {
		tx, err := beginner.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if err := applyMigrationsInStore(ctx, tx, true); err != nil {
			_ = tx.Rollback()
			return err
		}
		return tx.Commit()
	}
	return applyMigrationsInStore(ctx, db, false)
}

// ApplySchema remains as a compatibility wrapper over the migration-driven path.
func ApplySchema(ctx context.Context, db SchemaApplier) error {
	return ApplyMigrations(ctx, db)
}

// SchemaVersion returns the latest applied migration version or an empty string
// when the migration table does not exist yet.
func SchemaVersion(ctx context.Context, db SchemaApplier) (string, error) {
	exists, err := migrationTableExists(ctx, db)
	if err != nil || !exists {
		return "", err
	}
	var version string
	switch err := db.QueryRowContext(ctx, `
		SELECT version
		FROM harness_schema_migrations
		ORDER BY version DESC
		LIMIT 1
	`).Scan(&version); err {
	case nil:
		return version, nil
	case sql.ErrNoRows:
		return "", nil
	default:
		return "", err
	}
}

// LatestSchemaVersion reports the newest embedded migration version.
func LatestSchemaVersion() string {
	return internalpostgres.LatestMigrationVersion()
}

// EnsureSchema creates the target schema when it does not exist yet.
func EnsureSchema(ctx context.Context, adminDB *sql.DB, schema string) error {
	schema = strings.TrimSpace(schema)
	if schema == "" {
		return errors.New("schema is required")
	}
	if err := validateSchemaName(schema); err != nil {
		return err
	}
	if adminDB == nil {
		return errors.New("admin db is required")
	}
	_, err := adminDB.ExecContext(ctx, fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS %s`, quoteIdentifier(schema)))
	return err
}

// OpenDBWithConfig opens a Postgres database handle using embedder-facing
// schema/search_path and pool configuration.
func OpenDBWithConfig(ctx context.Context, cfg Config) (*sql.DB, error) {
	dsn := strings.TrimSpace(cfg.DSN)
	if dsn == "" {
		return nil, errors.New("postgres DSN is required")
	}
	schema := strings.TrimSpace(cfg.Schema)
	if schema != "" {
		if err := validateSchemaName(schema); err != nil {
			return nil, err
		}
		adminDB, err := openAndPingDB(ctx, dsn, 0, 0, 0)
		if err != nil {
			return nil, err
		}
		if err := EnsureSchema(ctx, adminDB, schema); err != nil {
			_ = adminDB.Close()
			return nil, err
		}
		if err := adminDB.Close(); err != nil {
			return nil, err
		}
		dsn, err = dsnWithSearchPath(dsn, schema)
		if err != nil {
			return nil, err
		}
	}
	db, err := openAndPingDB(ctx, dsn, cfg.MaxOpenConns, cfg.MaxIdleConns, cfg.ConnMaxLifetime)
	if err != nil {
		return nil, err
	}
	if cfg.ApplyMigrations {
		if err := ApplyMigrations(ctx, db); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	return db, nil
}

// BuildOptions wires Postgres-backed repositories and transaction boundaries
// into runtime options while preserving the caller's higher-level behavior.
func BuildOptions(db *sql.DB, opts hruntime.Options) hruntime.Options {
	opts = hruntime.WithDefaults(opts)
	opts.Sessions = sessionrepo.New(db)
	opts.Tasks = taskrepo.New(db)
	opts.Plans = planrepo.New(db)
	opts.Approvals = approvalrepo.New(db)
	opts.Attempts = executionrepo.NewAttemptStore(db)
	opts.Actions = executionrepo.NewActionStore(db)
	opts.Verifications = executionrepo.NewVerificationStore(db)
	opts.Artifacts = executionrepo.NewArtifactStore(db)
	opts.RuntimeHandles = executionrepo.NewRuntimeHandleStore(db)
	opts.CapabilitySnapshots = capabilityrepo.New(db)
	opts.PlanningRecords = planningrepo.New(db)
	opts.Audit = auditrepo.New(db)
	opts.ContextSummaries = contextrepo.New(db)
	opts.Runner = persistence.TransactionalRunner{
		Manager: internalpostgres.TxManager{DB: db},
		Factory: internalpostgres.RepositoryFactory{
			SessionFactory:  func(dbtx internalpostgres.DBTX) session.Store { return sessionrepo.New(dbtx) },
			TaskFactory:     func(dbtx internalpostgres.DBTX) task.Store { return taskrepo.New(dbtx) },
			PlanFactory:     func(dbtx internalpostgres.DBTX) plan.Store { return planrepo.New(dbtx) },
			AuditFactory:    func(dbtx internalpostgres.DBTX) audit.Store { return auditrepo.New(dbtx) },
			ApprovalFactory: func(dbtx internalpostgres.DBTX) approval.Store { return approvalrepo.New(dbtx) },
			CapabilitySnapshotFactory: func(dbtx internalpostgres.DBTX) capability.SnapshotStore {
				return capabilityrepo.New(dbtx)
			},
			AttemptFactory: func(dbtx internalpostgres.DBTX) execution.AttemptStore {
				return executionrepo.NewAttemptStore(dbtx)
			},
			ActionFactory: func(dbtx internalpostgres.DBTX) execution.ActionStore {
				return executionrepo.NewActionStore(dbtx)
			},
			VerificationFactory: func(dbtx internalpostgres.DBTX) execution.VerificationStore {
				return executionrepo.NewVerificationStore(dbtx)
			},
			ArtifactFactory: func(dbtx internalpostgres.DBTX) execution.ArtifactStore {
				return executionrepo.NewArtifactStore(dbtx)
			},
			RuntimeHandleFactory: func(dbtx internalpostgres.DBTX) execution.RuntimeHandleStore {
				return executionrepo.NewRuntimeHandleStore(dbtx)
			},
			PlanningFactory: func(dbtx internalpostgres.DBTX) planning.Store {
				return planningrepo.New(dbtx)
			},
		},
	}
	if opts.EventSink == nil {
		opts.EventSink = hruntime.AuditStoreSink{Store: opts.Audit}
	} else if aware, ok := opts.EventSink.(interface {
		WithAuditStore(store audit.Store) hruntime.EventSink
	}); ok {
		opts.EventSink = aware.WithAuditStore(opts.Audit)
	} else if opts.Audit != nil {
		opts.EventSink = hruntime.FanoutEventSink{Sinks: []hruntime.EventSink{opts.EventSink, hruntime.AuditStoreSink{Store: opts.Audit}}}
	}
	opts.StorageMode = "postgres"
	return opts
}

// OpenService opens a Postgres DB, applies schema, and returns a runtime
// service using durable Postgres-backed repositories and transaction support.
func OpenService(ctx context.Context, dsn string, opts hruntime.Options) (*hruntime.Service, *sql.DB, error) {
	return OpenServiceWithConfig(ctx, Config{DSN: dsn, ApplyMigrations: true}, opts)
}

// OpenServiceWithConfig opens a Postgres DB through the public schema-aware
// config path and returns a durable runtime service.
func OpenServiceWithConfig(ctx context.Context, cfg Config, opts hruntime.Options) (*hruntime.Service, *sql.DB, error) {
	db, err := OpenDBWithConfig(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}
	return hruntime.New(BuildOptions(db, opts)), db, nil
}

func applyMigrationsInStore(ctx context.Context, db SchemaApplier, lock bool) error {
	if lock {
		if _, err := db.ExecContext(ctx, `SELECT pg_advisory_xact_lock(48716321)`); err != nil {
			return err
		}
	}
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS harness_schema_migrations (
			version TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at BIGINT NOT NULL
		)
	`); err != nil {
		return err
	}
	for _, migration := range internalpostgres.Migrations() {
		applied, err := migrationApplied(ctx, db, migration.Version)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		if _, err := db.ExecContext(ctx, migration.SQL); err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO harness_schema_migrations (version, name, applied_at)
			VALUES ($1, $2, $3)
		`, migration.Version, migration.Name, time.Now().UnixMilli()); err != nil {
			return err
		}
	}
	return nil
}

func migrationApplied(ctx context.Context, db SchemaApplier, version string) (bool, error) {
	var exists bool
	if err := db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM harness_schema_migrations
			WHERE version = $1
		)
	`, version).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func migrationTableExists(ctx context.Context, db SchemaApplier) (bool, error) {
	schema, err := currentSchema(ctx, db)
	if err != nil {
		return false, err
	}
	var exists bool
	if err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.tables
			WHERE table_schema = $1
			  AND table_name = 'harness_schema_migrations'
		)
	`, schema).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func openAndPingDB(ctx context.Context, dsn string, maxOpenConns, maxIdleConns int, connMaxLifetime time.Duration) (*sql.DB, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, errors.New("postgres DSN is required")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	if maxOpenConns > 0 {
		db.SetMaxOpenConns(maxOpenConns)
	}
	if maxIdleConns > 0 {
		db.SetMaxIdleConns(maxIdleConns)
	}
	if connMaxLifetime > 0 {
		db.SetConnMaxLifetime(connMaxLifetime)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func currentSchema(ctx context.Context, db SchemaApplier) (string, error) {
	var schema string
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(current_schema(), 'public')`).Scan(&schema); err != nil {
		return "", err
	}
	if strings.TrimSpace(schema) == "" {
		return "public", nil
	}
	return schema, nil
}

func validateSchemaName(schema string) error {
	if !schemaNamePattern.MatchString(schema) {
		return fmt.Errorf("invalid schema name %q", schema)
	}
	return nil
}

func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func dsnWithSearchPath(dsn, schema string) (string, error) {
	searchPath := schema + ",public"
	if strings.Contains(dsn, "://") {
		parsed, err := url.Parse(dsn)
		if err != nil {
			return "", err
		}
		query := parsed.Query()
		query.Set("search_path", searchPath)
		parsed.RawQuery = query.Encode()
		return parsed.String(), nil
	}
	return strings.TrimSpace(dsn) + " search_path=" + searchPath, nil
}
