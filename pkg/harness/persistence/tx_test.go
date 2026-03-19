package persistence_test

import (
	"context"
	"errors"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/persistence"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

type fakeTx struct {
	committed bool
	rolled    bool
}

func (f *fakeTx) Commit() error {
	f.committed = true
	return nil
}

func (f *fakeTx) Rollback() error {
	f.rolled = true
	return nil
}

type fakeManager struct {
	tx *fakeTx
}

func (m fakeManager) Begin(_ context.Context) (persistence.Tx, error) {
	return m.tx, nil
}

type fakeFactory struct {
	repos persistence.RepositorySet
}

func (f fakeFactory) FromTx(_ persistence.Tx) persistence.RepositorySet {
	return f.repos
}

func TestTransactionalRunnerCommitsOnSuccess(t *testing.T) {
	tx := &fakeTx{}
	repos := persistence.RepositorySet{
		Sessions: session.NewMemoryStore(),
		Tasks:    task.NewMemoryStore(),
		Plans:    plan.NewMemoryStore(),
		Audits:   audit.NewMemoryStore(),
	}
	runner := persistence.TransactionalRunner{
		Manager: fakeManager{tx: tx},
		Factory: fakeFactory{repos: repos},
	}
	called := false
	err := runner.Within(context.Background(), func(got persistence.RepositorySet) error {
		called = true
		if got.Sessions == nil || got.Tasks == nil || got.Plans == nil || got.Audits == nil {
			t.Fatalf("expected full repository set")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("within: %v", err)
	}
	if !called {
		t.Fatalf("expected closure to run")
	}
	if !tx.committed {
		t.Fatalf("expected commit to be called")
	}
	if tx.rolled {
		t.Fatalf("did not expect rollback on success")
	}
}

func TestTransactionalRunnerRollsBackOnFailure(t *testing.T) {
	tx := &fakeTx{}
	repos := persistence.RepositorySet{
		Sessions: session.NewMemoryStore(),
		Tasks:    task.NewMemoryStore(),
		Plans:    plan.NewMemoryStore(),
		Audits:   audit.NewMemoryStore(),
	}
	runner := persistence.TransactionalRunner{
		Manager: fakeManager{tx: tx},
		Factory: fakeFactory{repos: repos},
	}
	targetErr := errors.New("boom")
	err := runner.Within(context.Background(), func(_ persistence.RepositorySet) error {
		return targetErr
	})
	if !errors.Is(err, targetErr) {
		t.Fatalf("expected propagated error, got %v", err)
	}
	if tx.committed {
		t.Fatalf("did not expect commit on failure")
	}
	if !tx.rolled {
		t.Fatalf("expected rollback to be called")
	}
}
