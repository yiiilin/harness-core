package postgres

import (
  "github.com/yiiilin/harness-core/pkg/harness/audit"
  "github.com/yiiilin/harness-core/pkg/harness/persistence"
  "github.com/yiiilin/harness-core/pkg/harness/plan"
  "github.com/yiiilin/harness-core/pkg/harness/session"
  "github.com/yiiilin/harness-core/pkg/harness/task"
)

// RepositoryFactory maps an active SQL transaction into a RepositorySet.
// This is currently a scaffold. Concrete session/task/plan/audit repos will be
// plugged in here as they are implemented.
type RepositoryFactory struct {
  Sessions session.Store
  Tasks    task.Store
  Plans    plan.Store
  Audits   audit.Store
}

func (f RepositoryFactory) FromTx(_ persistence.Tx) persistence.RepositorySet {
  return persistence.RepositorySet{
    Sessions: f.Sessions,
    Tasks:    f.Tasks,
    Plans:    f.Plans,
    Audits:   f.Audits,
  }
}
