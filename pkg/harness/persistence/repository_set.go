package persistence

import (
	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/capability"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/planning"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

type RepositorySet struct {
	Sessions            session.Store
	Tasks               task.Store
	Plans               plan.Store
	Audits              audit.Store
	Attempts            execution.AttemptStore
	Actions             execution.ActionStore
	Verifications       execution.VerificationStore
	Artifacts           execution.ArtifactStore
	RuntimeHandles      execution.RuntimeHandleStore
	Approvals           approval.Store
	CapabilitySnapshots capability.SnapshotStore
	PlanningRecords     planning.Store
}
