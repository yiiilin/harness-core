package runtime_test

import (
	"context"
	"testing"
	"time"

	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

func TestClaimRunnableSessionSkipsBlockedAndTerminalSessions(t *testing.T) {
	sessions := session.NewMemoryStore()
	rt := hruntime.New(hruntime.Options{Sessions: sessions})

	blocked := mustCreateSession(t, rt, "blocked", "awaiting approval")
	blocked.ExecutionState = session.ExecutionAwaitingApproval
	blocked.PendingApprovalID = "apv_1"
	blocked.Version++
	if err := sessions.Update(blocked); err != nil {
		t.Fatalf("update blocked session: %v", err)
	}

	terminal := mustCreateSession(t, rt, "terminal", "already complete")
	terminal.Phase = session.PhaseComplete
	terminal.Version++
	if err := sessions.Update(terminal); err != nil {
		t.Fatalf("update terminal session: %v", err)
	}

	runnable := mustCreateSession(t, rt, "runnable", "claim me")

	claimed, ok, err := rt.ClaimRunnableSession(context.Background(), time.Minute)
	if err != nil {
		t.Fatalf("claim runnable session: %v", err)
	}
	if !ok {
		t.Fatalf("expected to claim a runnable session")
	}
	if claimed.SessionID != runnable.SessionID {
		t.Fatalf("expected runnable session %s, got %#v", runnable.SessionID, claimed)
	}
	if claimed.LeaseID == "" || claimed.LeaseExpiresAt == 0 {
		t.Fatalf("expected lease fields to be populated, got %#v", claimed)
	}
}

func TestClaimRecoverableSessionClaimsInterruptedSession(t *testing.T) {
	sessions := session.NewMemoryStore()
	rt := hruntime.New(hruntime.Options{Sessions: sessions})

	idle := mustCreateSession(t, rt, "idle", "not recoverable")
	_ = idle
	interrupted := mustCreateSession(t, rt, "interrupted", "recover me")
	interrupted.ExecutionState = session.ExecutionInterrupted
	interrupted.Phase = session.PhaseRecover
	interrupted.Version++
	if err := sessions.Update(interrupted); err != nil {
		t.Fatalf("update interrupted session: %v", err)
	}

	claimed, ok, err := rt.ClaimRecoverableSession(context.Background(), time.Minute)
	if err != nil {
		t.Fatalf("claim recoverable session: %v", err)
	}
	if !ok {
		t.Fatalf("expected recoverable session to be claimed")
	}
	if claimed.SessionID != interrupted.SessionID {
		t.Fatalf("expected interrupted session %s, got %#v", interrupted.SessionID, claimed)
	}
	if claimed.LeaseID == "" || claimed.LeaseExpiresAt == 0 {
		t.Fatalf("expected recoverable claim to attach lease, got %#v", claimed)
	}
}

func TestRenewAndReleaseSessionLease(t *testing.T) {
	sessions := session.NewMemoryStore()
	rt := hruntime.New(hruntime.Options{Sessions: sessions})

	mustCreateSession(t, rt, "lease", "renew and release")

	claimed, ok, err := rt.ClaimRunnableSession(context.Background(), time.Minute)
	if err != nil {
		t.Fatalf("claim runnable session: %v", err)
	}
	if !ok {
		t.Fatalf("expected runnable session to be claimed")
	}

	renewed, err := rt.RenewSessionLease(context.Background(), claimed.SessionID, claimed.LeaseID, 2*time.Minute)
	if err != nil {
		t.Fatalf("renew session lease: %v", err)
	}
	if renewed.LeaseID != claimed.LeaseID {
		t.Fatalf("expected lease ID to stay stable, got %#v", renewed)
	}
	if renewed.LeaseExpiresAt <= claimed.LeaseExpiresAt {
		t.Fatalf("expected lease expiry to extend, before=%d after=%d", claimed.LeaseExpiresAt, renewed.LeaseExpiresAt)
	}

	released, err := rt.ReleaseSessionLease(context.Background(), claimed.SessionID, claimed.LeaseID)
	if err != nil {
		t.Fatalf("release session lease: %v", err)
	}
	if released.LeaseID != "" || released.LeaseExpiresAt != 0 {
		t.Fatalf("expected lease fields cleared after release, got %#v", released)
	}
}

func TestClaimRunnableSessionHasSingleWinner(t *testing.T) {
	sessions := session.NewMemoryStore()
	rt := hruntime.New(hruntime.Options{Sessions: sessions})

	runnable := mustCreateSession(t, rt, "race", "only one winner")
	_ = runnable

	type claimResult struct {
		state session.State
		ok    bool
		err   error
	}

	results := make(chan claimResult, 2)
	for i := 0; i < 2; i++ {
		go func() {
			state, ok, err := rt.ClaimRunnableSession(context.Background(), time.Minute)
			results <- claimResult{state: state, ok: ok, err: err}
		}()
	}

	winners := 0
	empty := 0
	var winner session.State
	for i := 0; i < 2; i++ {
		result := <-results
		if result.err != nil {
			t.Fatalf("claim runnable session: %v", result.err)
		}
		if result.ok {
			winners++
			winner = result.state
			continue
		}
		empty++
	}

	if winners != 1 || empty != 1 {
		t.Fatalf("expected one claim winner and one empty result, got %d winners and %d empty", winners, empty)
	}
	if winner.LeaseID == "" {
		t.Fatalf("expected winning claim to include lease, got %#v", winner)
	}
}
