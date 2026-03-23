package runtime_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/persistence"
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

func TestLeaseOperationsEmitAuditEventsWithinRunnerBoundary(t *testing.T) {
	sessions := session.NewMemoryStore()
	audits := audit.NewMemoryStore()
	runner := &countingRunner{repos: persistence.RepositorySet{
		Sessions: sessions,
		Audits:   audits,
	}}
	rt := hruntime.New(hruntime.Options{
		Sessions: sessions,
		Audit:    audits,
		Runner:   runner,
	})

	sess := mustCreateSession(t, rt, "lease audit", "lease operations should emit audit events")

	claimed, ok, err := rt.ClaimRunnableSession(context.Background(), time.Minute)
	if err != nil {
		t.Fatalf("claim runnable session: %v", err)
	}
	if !ok {
		t.Fatalf("expected runnable session to be claimed")
	}
	if _, err := rt.RenewSessionLease(context.Background(), claimed.SessionID, claimed.LeaseID, 2*time.Minute); err != nil {
		t.Fatalf("renew session lease: %v", err)
	}
	if _, err := rt.ReleaseSessionLease(context.Background(), claimed.SessionID, claimed.LeaseID); err != nil {
		t.Fatalf("release session lease: %v", err)
	}

	if runner.calls < 3 {
		t.Fatalf("expected lease operations to use runner boundary, got %d calls", runner.calls)
	}

	events := mustListAuditEvents(t, rt, sess.SessionID)
	expected := map[string]bool{
		audit.EventLeaseClaimed:  false,
		audit.EventLeaseRenewed:  false,
		audit.EventLeaseReleased: false,
	}
	for _, event := range events {
		if _, ok := expected[event.Type]; !ok {
			continue
		}
		expected[event.Type] = true
		if got, _ := event.Payload["lease_id"].(string); got != claimed.LeaseID {
			t.Fatalf("expected lease event %s to carry lease_id %q, got %#v", event.Type, claimed.LeaseID, event)
		}
	}
	for typ, found := range expected {
		if !found {
			t.Fatalf("expected lease event %s, got %#v", typ, events)
		}
	}
}

func TestLeaseAuditFailuresAreBestEffortWithoutRunnerAndSurfaceWithRunner(t *testing.T) {
	t.Run("without runner claim stays successful", func(t *testing.T) {
		rt := hruntime.New(hruntime.Options{
			EventSink: selectiveFailingEventSink{failures: map[string]error{audit.EventLeaseClaimed: errors.New("boom:lease.claimed")}},
		})
		rt.Runner = nil
		sess := mustCreateSession(t, rt, "lease best effort", "claim should stay successful without runner")

		claimed, ok, err := rt.ClaimRunnableSession(context.Background(), time.Minute)
		if err != nil {
			t.Fatalf("expected claim to stay successful without runner, got %v", err)
		}
		if !ok || claimed.SessionID != sess.SessionID {
			t.Fatalf("expected claimed session %q, got ok=%v state=%#v", sess.SessionID, ok, claimed)
		}
	})

	t.Run("with runner claim surfaces emit failure", func(t *testing.T) {
		sessions := session.NewMemoryStore()
		audits := audit.NewMemoryStore()
		runner := &countingRunner{repos: persistence.RepositorySet{
			Sessions: sessions,
			Audits:   audits,
		}}
		boom := errors.New("boom:lease.claimed")
		rt := hruntime.New(hruntime.Options{
			Sessions: sessions,
			Audit:    audits,
			Runner:   runner,
			EventSink: selectiveFailingEventSink{failures: map[string]error{
				audit.EventLeaseClaimed: boom,
			}},
		})
		mustCreateSession(t, rt, "lease runner failure", "claim should surface emit error with runner")

		if _, _, err := rt.ClaimRunnableSession(context.Background(), time.Minute); !errors.Is(err, boom) {
			t.Fatalf("expected runner-backed claim to surface emit error, got %v", err)
		}
	})
}

func TestReleaseSessionLeaseRejectsExpiredHolder(t *testing.T) {
	sessions := session.NewMemoryStore()
	rt := hruntime.New(hruntime.Options{Sessions: sessions})

	mustCreateSession(t, rt, "lease", "expired holders should not release")

	claimed, ok, err := rt.ClaimRunnableSession(context.Background(), time.Minute)
	if err != nil {
		t.Fatalf("claim runnable session: %v", err)
	}
	if !ok {
		t.Fatalf("expected runnable session to be claimed")
	}

	stored, err := sessions.Get(claimed.SessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	stored.LeaseExpiresAt = time.Now().Add(-time.Second).UnixMilli()
	stored.Version++
	if err := sessions.Update(stored); err != nil {
		t.Fatalf("expire lease: %v", err)
	}

	if _, err := rt.ReleaseSessionLease(context.Background(), claimed.SessionID, claimed.LeaseID); !errors.Is(err, session.ErrSessionLeaseNotHeld) {
		t.Fatalf("expected expired release to fail with lease-not-held, got %v", err)
	}
}

func TestClaimRecoverableSessionReclaimsExpiredLeaseOnlyAfterExpiry(t *testing.T) {
	sessions := session.NewMemoryStore()
	rt := hruntime.New(hruntime.Options{Sessions: sessions})

	recoverable := mustCreateSession(t, rt, "recoverable", "reclaim only stale holders")
	now := time.Now().UnixMilli()
	recoverable.ExecutionState = session.ExecutionInterrupted
	recoverable.Phase = session.PhaseRecover
	recoverable.LeaseID = "lease_live"
	recoverable.LeaseClaimedAt = now
	recoverable.LeaseExpiresAt = now + int64(time.Minute/time.Millisecond)
	recoverable.LastHeartbeatAt = now
	recoverable.Version++
	if err := sessions.Update(recoverable); err != nil {
		t.Fatalf("update recoverable session: %v", err)
	}

	if _, ok, err := rt.ClaimRecoverableSession(context.Background(), time.Minute); err != nil {
		t.Fatalf("claim recoverable session with live lease: %v", err)
	} else if ok {
		t.Fatalf("expected live recoverable lease to block reclaim")
	}

	stale, err := sessions.Get(recoverable.SessionID)
	if err != nil {
		t.Fatalf("get recoverable session: %v", err)
	}
	stale.LeaseExpiresAt = time.Now().Add(-time.Second).UnixMilli()
	stale.Version++
	if err := sessions.Update(stale); err != nil {
		t.Fatalf("expire recoverable lease: %v", err)
	}

	claimed, ok, err := rt.ClaimRecoverableSession(context.Background(), time.Minute)
	if err != nil {
		t.Fatalf("claim stale recoverable session: %v", err)
	}
	if !ok {
		t.Fatalf("expected stale recoverable lease to be reclaimable")
	}
	if claimed.SessionID != recoverable.SessionID {
		t.Fatalf("expected recoverable session %s, got %#v", recoverable.SessionID, claimed)
	}
	if claimed.LeaseID == "" || claimed.LeaseID == "lease_live" {
		t.Fatalf("expected reclaim to issue a fresh lease, got %#v", claimed)
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
