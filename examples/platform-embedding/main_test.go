package main

import (
	"context"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
)

func TestRunEmbeddingDemo(t *testing.T) {
	result, err := RunEmbeddingDemo(context.Background())
	if err != nil {
		t.Fatalf("run embedding demo: %v", err)
	}

	if result.Accepted.ExternalRunID == "" || result.Accepted.SessionID == "" {
		t.Fatalf("expected accepted run metadata, got %#v", result.Accepted)
	}
	if result.Accepted.Status != "accepted" {
		t.Fatalf("expected accepted-first wrapper response, got %#v", result.Accepted)
	}
	if result.StoredExternalRunID != result.Accepted.ExternalRunID {
		t.Fatalf("expected external run id to remain in platform metadata, got stored=%q accepted=%q", result.StoredExternalRunID, result.Accepted.ExternalRunID)
	}
	if !result.FirstWorkerRun.ApprovalPending {
		t.Fatalf("expected first worker run to pause for approval, got %#v", result.FirstWorkerRun)
	}
	if result.RemotePTYCallsBeforeApproval != 0 {
		t.Fatalf("expected remote PTY backend to remain untouched before approval, got %d", result.RemotePTYCallsBeforeApproval)
	}
	if result.PendingApproval.ApprovalID == "" || result.PendingApproval.Status != approval.StatusPending {
		t.Fatalf("expected pending approval surfaced to external UI, got %#v", result.PendingApproval)
	}
	if result.PTYVerifierRegistered {
		t.Fatalf("expected PTY verifiers to stay unregistered without a local PTY manager")
	}
	if result.ApprovalRecord.Status != approval.StatusApproved || result.ApprovalRecord.Reply != approval.ReplyOnce {
		t.Fatalf("expected external approval UI to approve once, got %#v", result.ApprovalRecord)
	}
	if result.SecondWorkerRun.NoWork || result.SecondWorkerRun.ApprovalPending {
		t.Fatalf("expected second worker run to complete approved work, got %#v", result.SecondWorkerRun)
	}
	if result.RemotePTYCallsAfterApproval != 1 {
		t.Fatalf("expected one remote PTY execution after approval, got %d", result.RemotePTYCallsAfterApproval)
	}
	if len(result.RuntimeHandles) != 1 {
		t.Fatalf("expected one persisted runtime handle, got %#v", result.RuntimeHandles)
	}
	if result.RuntimeHandles[0].Status != execution.RuntimeHandleActive || result.RuntimeHandles[0].Kind != "pty" {
		t.Fatalf("expected active PTY runtime handle, got %#v", result.RuntimeHandles[0])
	}
	if len(result.Projection.Cycles) != 1 {
		t.Fatalf("expected one replay cycle, got %#v", result.Projection)
	}
	if len(result.Projection.Cycles[0].Cycle.RuntimeHandles) != 1 {
		t.Fatalf("expected replay projection to carry runtime handle facts, got %#v", result.Projection.Cycles[0])
	}
	if len(result.Projection.Events) == 0 {
		t.Fatalf("expected replay projection to include ordered audit events, got %#v", result.Projection)
	}
}
