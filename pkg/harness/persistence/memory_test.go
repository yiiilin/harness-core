package persistence_test

import (
	"context"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/persistence"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

func TestMemoryUnitOfWorkCallsClosureWithRepositories(t *testing.T) {
	repos := persistence.RepositorySet{
		Sessions: session.NewMemoryStore(),
		Tasks:    task.NewMemoryStore(),
		Plans:    plan.NewMemoryStore(),
		Audits:   audit.NewMemoryStore(),
	}
	uow := persistence.NewMemoryUnitOfWork(repos)
	called := false
	err := uow.Within(context.Background(), func(got persistence.RepositorySet) error {
		called = true
		if got.Sessions == nil || got.Tasks == nil || got.Plans == nil || got.Audits == nil {
			t.Fatalf("expected all repositories to be present")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("within: %v", err)
	}
	if !called {
		t.Fatalf("expected closure to be called")
	}
}
