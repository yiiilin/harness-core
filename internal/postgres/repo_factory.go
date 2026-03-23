package postgres

import (
	"fmt"

	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/capability"
	"github.com/yiiilin/harness-core/pkg/harness/contextsummary"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/persistence"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/planning"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

type SessionFactory func(DBTX) session.Store
type TaskFactory func(DBTX) task.Store
type PlanFactory func(DBTX) plan.Store
type AuditFactory func(DBTX) audit.Store
type ApprovalFactory func(DBTX) approval.Store
type CapabilitySnapshotFactory func(DBTX) capability.SnapshotStore
type AttemptFactory func(DBTX) execution.AttemptStore
type ActionFactory func(DBTX) execution.ActionStore
type VerificationFactory func(DBTX) execution.VerificationStore
type ArtifactFactory func(DBTX) execution.ArtifactStore
type RuntimeHandleFactory func(DBTX) execution.RuntimeHandleStore
type ContextSummaryFactory func(DBTX) contextsummary.Store
type PlanningFactory func(DBTX) planning.Store

// RepositoryFactory maps an active SQL transaction into a RepositorySet using
// typed repository constructor functions.
type RepositoryFactory struct {
	SessionFactory            SessionFactory
	TaskFactory               TaskFactory
	PlanFactory               PlanFactory
	AuditFactory              AuditFactory
	ApprovalFactory           ApprovalFactory
	CapabilitySnapshotFactory CapabilitySnapshotFactory
	AttemptFactory            AttemptFactory
	ActionFactory             ActionFactory
	VerificationFactory       VerificationFactory
	ArtifactFactory           ArtifactFactory
	RuntimeHandleFactory      RuntimeHandleFactory
	ContextSummaryFactory     ContextSummaryFactory
	PlanningFactory           PlanningFactory
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
	if f.ApprovalFactory != nil {
		repos.Approvals = f.ApprovalFactory(dbtx)
	}
	if f.CapabilitySnapshotFactory != nil {
		repos.CapabilitySnapshots = f.CapabilitySnapshotFactory(dbtx)
	}
	if f.AttemptFactory != nil {
		repos.Attempts = f.AttemptFactory(dbtx)
	}
	if f.ActionFactory != nil {
		repos.Actions = f.ActionFactory(dbtx)
	}
	if f.VerificationFactory != nil {
		repos.Verifications = f.VerificationFactory(dbtx)
	}
	if f.ArtifactFactory != nil {
		repos.Artifacts = f.ArtifactFactory(dbtx)
	}
	if f.RuntimeHandleFactory != nil {
		repos.RuntimeHandles = f.RuntimeHandleFactory(dbtx)
	}
	if f.ContextSummaryFactory != nil {
		repos.ContextSummaries = f.ContextSummaryFactory(dbtx)
	}
	if f.PlanningFactory != nil {
		repos.PlanningRecords = f.PlanningFactory(dbtx)
	}
	return repos
}
