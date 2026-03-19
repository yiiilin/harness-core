package persistence

import (
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

type RepositorySet struct {
	Sessions session.Store
	Tasks    task.Store
	Plans    plan.Store
	Audits   audit.Store
}
