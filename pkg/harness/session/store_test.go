package session_test

import (
	"errors"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/session"
)

func TestMemoryStoreUpdateRequiresFreshVersion(t *testing.T) {
	store := session.NewMemoryStore()

	created, err := store.Create("demo", "goal")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Version != 1 {
		t.Fatalf("expected create version 1, got %d", created.Version)
	}

	first, err := store.Get(created.SessionID)
	if err != nil {
		t.Fatalf("get first: %v", err)
	}
	second, err := store.Get(created.SessionID)
	if err != nil {
		t.Fatalf("get second: %v", err)
	}

	first.Summary = "first writer"
	first.Version++
	if err := store.Update(first); err != nil {
		t.Fatalf("update first: %v", err)
	}

	second.Summary = "stale writer"
	second.Version++
	if err := store.Update(second); !errors.Is(err, session.ErrSessionVersionConflict) {
		t.Fatalf("expected version conflict, got %v", err)
	}

	latest, err := store.Get(created.SessionID)
	if err != nil {
		t.Fatalf("get latest: %v", err)
	}
	if latest.Summary != "first writer" {
		t.Fatalf("expected first writer summary to win, got %#v", latest)
	}
	if latest.Version != 2 {
		t.Fatalf("expected version 2 after one successful update, got %d", latest.Version)
	}
}

func TestMemoryStoreUpdateMissingSession(t *testing.T) {
	store := session.NewMemoryStore()
	err := store.Update(session.State{SessionID: "missing", Version: 1})
	if !errors.Is(err, session.ErrSessionNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}
