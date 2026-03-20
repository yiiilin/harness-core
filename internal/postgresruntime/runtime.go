package postgresruntime

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"strings"

	_ "github.com/lib/pq"
	"github.com/yiiilin/harness-core/internal/postgres"
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

//go:embed schema.sql
var schemaSQL string

func OpenDB(ctx context.Context, dsn string) (*sql.DB, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, errors.New("postgres DSN is required")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func ApplySchema(ctx context.Context, db postgres.DBTX) error {
	_, err := db.ExecContext(ctx, schemaSQL)
	return err
}

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
		Manager: postgres.TxManager{DB: db},
		Factory: postgres.RepositoryFactory{
			SessionFactory:  func(dbtx postgres.DBTX) session.Store { return sessionrepo.New(dbtx) },
			TaskFactory:     func(dbtx postgres.DBTX) task.Store { return taskrepo.New(dbtx) },
			PlanFactory:     func(dbtx postgres.DBTX) plan.Store { return planrepo.New(dbtx) },
			AuditFactory:    func(dbtx postgres.DBTX) audit.Store { return auditrepo.New(dbtx) },
			ApprovalFactory: func(dbtx postgres.DBTX) approval.Store { return approvalrepo.New(dbtx) },
			CapabilitySnapshotFactory: func(dbtx postgres.DBTX) capability.SnapshotStore {
				return capabilityrepo.New(dbtx)
			},
			AttemptFactory: func(dbtx postgres.DBTX) execution.AttemptStore { return executionrepo.NewAttemptStore(dbtx) },
			ActionFactory:  func(dbtx postgres.DBTX) execution.ActionStore { return executionrepo.NewActionStore(dbtx) },
			VerificationFactory: func(dbtx postgres.DBTX) execution.VerificationStore {
				return executionrepo.NewVerificationStore(dbtx)
			},
			ArtifactFactory: func(dbtx postgres.DBTX) execution.ArtifactStore { return executionrepo.NewArtifactStore(dbtx) },
			RuntimeHandleFactory: func(dbtx postgres.DBTX) execution.RuntimeHandleStore {
				return executionrepo.NewRuntimeHandleStore(dbtx)
			},
			PlanningFactory: func(dbtx postgres.DBTX) planning.Store { return planningrepo.New(dbtx) },
		},
	}
	opts.EventSink = hruntime.AuditStoreSink{Store: opts.Audit}
	opts.StorageMode = "postgres"
	return opts
}

func OpenService(ctx context.Context, dsn string, opts hruntime.Options) (*hruntime.Service, *sql.DB, error) {
	db, err := OpenDB(ctx, dsn)
	if err != nil {
		return nil, nil, err
	}
	if err := ApplySchema(ctx, db); err != nil {
		_ = db.Close()
		return nil, nil, err
	}
	return hruntime.New(BuildOptions(db, opts)), db, nil
}
