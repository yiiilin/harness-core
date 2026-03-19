package postgres

import (
	"fmt"

	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/persistence"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

type SessionFactory func(DBTX) session.Store
type TaskFactory func(DBTX) task.Store
type PlanFactory func(DBTX) plan.Store
type AuditFactory func(DBTX) audit.Store

// RepositoryFactory maps an active SQL transaction into a RepositorySet using
// typed repository constructor functions.
type RepositoryFactory struct {
	SessionFactory SessionFactory
	TaskFactory    TaskFactory
	PlanFactory    PlanFactory
	AuditFactory   AuditFactory
}

func (f RepositoryFactory) FromTx(tx persistence.Tx) persistence.RepositorySet {
	dbtx, ok := tx.(DBTX)
	if !ok {
		panic(fmt.Sprintf("postgres.RepositoryFactory expected DBTX-compatible tx, got %T", tx))
	}
	var repos persistence.RepositorySet
	if f.SessionFactory != nil {
		repos.Sessions = f.SessionFactory(dbtx)
	}
	if f.TaskFactory != nil {
		repos.Tasks = f.TaskFactory(dbtx)
	}
	if f.PlanFactory != nil {
		repos.Plans = f.PlanFactory(dbtx)
	}
	if f.AuditFactory != nil {
		repos.Audits = f.AuditFactory(dbtx)
	}
	return repos
}
