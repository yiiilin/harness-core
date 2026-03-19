package postgres_test

import (
	"context"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/yiiilin/harness-core/internal/postgres"
	"github.com/yiiilin/harness-core/internal/postgres/auditrepo"
	"github.com/yiiilin/harness-core/internal/postgres/planrepo"
	"github.com/yiiilin/harness-core/internal/postgres/sessionrepo"
	"github.com/yiiilin/harness-core/internal/postgres/taskrepo"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

func TestRepositoryFactoryFromTx(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectRollback()
	mgr := postgres.TxManager{DB: db}
	tx, err := mgr.Begin(context.Background())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	factory := postgres.RepositoryFactory{
		SessionFactory: func(db postgres.DBTX) session.Store { return sessionrepo.New(db) },
		TaskFactory:    func(db postgres.DBTX) task.Store { return taskrepo.New(db) },
		PlanFactory:    func(db postgres.DBTX) plan.Store { return planrepo.New(db) },
		AuditFactory:   func(db postgres.DBTX) audit.Store { return auditrepo.New(db) },
	}
	repos := factory.FromTx(tx)
	if repos.Sessions == nil || repos.Tasks == nil || repos.Plans == nil || repos.Audits == nil {
		t.Fatalf("expected all repos to be initialized")
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
