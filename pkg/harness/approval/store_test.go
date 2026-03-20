package approval_test

import (
	"errors"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/approval"
)

func TestMemoryStoreUpdateRequiresFreshVersion(t *testing.T) {
	store := approval.NewMemoryStore()

	created, err := store.CreatePending(approval.Request{
		SessionID: "sess1",
		StepID:    "step_1",
		ToolName:  "shell.exec",
	})
	if err != nil {
		t.Fatalf("create pending: %v", err)
	}
	if created.Version != 1 {
		t.Fatalf("expected create version 1, got %d", created.Version)
	}

	first, err := store.Get(created.ApprovalID)
	if err != nil {
		t.Fatalf("get first: %v", err)
	}
	second, err := store.Get(created.ApprovalID)
	if err != nil {
		t.Fatalf("get second: %v", err)
	}

	first.Status = approval.StatusApproved
	first.Reply = approval.ReplyOnce
	first.Version++
	if err := store.Update(first); err != nil {
		t.Fatalf("update first: %v", err)
	}

	second.Status = approval.StatusRejected
	second.Reply = approval.ReplyReject
	second.Version++
	if err := store.Update(second); !errors.Is(err, approval.ErrApprovalVersionConflict) {
		t.Fatalf("expected version conflict, got %v", err)
	}

	latest, err := store.Get(created.ApprovalID)
	if err != nil {
		t.Fatalf("get latest: %v", err)
	}
	if latest.Status != approval.StatusApproved || latest.Reply != approval.ReplyOnce {
		t.Fatalf("expected first writer approval to win, got %#v", latest)
	}
	if latest.Version != 2 {
		t.Fatalf("expected version 2 after one successful update, got %d", latest.Version)
	}
}

func TestMemoryStoreUpdateMissingApproval(t *testing.T) {
	store := approval.NewMemoryStore()
	err := store.Update(approval.Record{ApprovalID: "missing", Version: 1})
	if !errors.Is(err, approval.ErrApprovalNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}
