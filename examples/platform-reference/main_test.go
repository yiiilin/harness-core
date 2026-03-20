package main

import (
	"context"
	"strings"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

func TestRunReferenceDemo(t *testing.T) {
	result, err := RunReferenceDemo(context.Background())
	if err != nil {
		t.Fatalf("run reference demo: %v", err)
	}

	if result.Worker.Claimed.SessionID == "" {
		t.Fatalf("expected worker to claim a session, got %#v", result.Worker)
	}
	if len(result.Worker.Run.Executions) != 1 {
		t.Fatalf("expected exactly one claimed execution, got %#v", result.Worker.Run.Executions)
	}
	if result.Worker.Run.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected claimed run to complete the session, got %#v", result.Worker.Run.Session)
	}
	if result.Worker.Released.LeaseID != "" || result.Worker.Released.LeaseExpiresAt != 0 {
		t.Fatalf("expected session lease to be released, got %#v", result.Worker.Released)
	}

	if result.PersistedRuntimeHandle.HandleID == "" {
		t.Fatalf("expected persisted runtime handle, got %#v", result.PersistedRuntimeHandle)
	}
	if !result.ActiveVerify.Success {
		t.Fatalf("expected pty_handle_active verifier to succeed, got %#v", result.ActiveVerify)
	}
	if !result.StreamVerify.Success {
		t.Fatalf("expected pty_stream_contains verifier to succeed, got %#v", result.StreamVerify)
	}
	if !strings.Contains(result.AttachOutput, "hello from platform reference") {
		t.Fatalf("expected attach output to contain bridged PTY bytes, got %q", result.AttachOutput)
	}
	if !result.AttachDetached {
		t.Fatalf("expected detach to stop the output bridge, got %#v", result)
	}
	if result.ClosedRuntimeHandle.Status != execution.RuntimeHandleClosed {
		t.Fatalf("expected runtime handle to be closed after PTY shutdown, got %#v", result.ClosedRuntimeHandle)
	}
	if !strings.Contains(result.StreamRead.Data, "after-detach") {
		t.Fatalf("expected direct PTY read to confirm the session stayed alive after detach, got %#v", result.StreamRead)
	}
	if !result.StreamRead.Closed {
		t.Fatalf("expected PTY stream to report closed after shutdown, got %#v", result.StreamRead)
	}
	if !result.InteractiveHandleReleased {
		t.Fatalf("expected lease release flag after worker completion, got %#v", result)
	}
}
