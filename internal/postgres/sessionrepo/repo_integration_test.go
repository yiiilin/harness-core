package sessionrepo_test

import (
	"context"
	"testing"
	"time"

	"github.com/yiiilin/harness-core/internal/postgres/sessionrepo"
	"github.com/yiiilin/harness-core/internal/postgresruntime"
	"github.com/yiiilin/harness-core/internal/postgrestest"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

func TestSessionRepoClaimRenewReleaseLeaseAgainstPostgres(t *testing.T) {
	pg := postgrestest.Start(t)
	db, err := postgresruntime.OpenDB(context.Background(), pg.DSN)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	repo := sessionrepo.New(db)
	created, err := repo.Create("claim", "lease lifecycle")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	now := time.Now().UnixMilli()
	claimed, ok, err := repo.ClaimNext(session.ClaimModeRunnable, "lease_1", now, now+60_000)
	if err != nil {
		t.Fatalf("claim next: %v", err)
	}
	if !ok {
		t.Fatalf("expected claimable runnable session")
	}
	if claimed.SessionID != created.SessionID || claimed.LeaseID != "lease_1" {
		t.Fatalf("unexpected claimed session: %#v", claimed)
	}

	renewed, err := repo.RenewLease(created.SessionID, "lease_1", now+10_000, now+120_000)
	if err != nil {
		t.Fatalf("renew lease: %v", err)
	}
	if renewed.LeaseExpiresAt != now+120_000 {
		t.Fatalf("expected renewed expiry, got %#v", renewed)
	}

	released, err := repo.ReleaseLease(created.SessionID, "lease_1")
	if err != nil {
		t.Fatalf("release lease: %v", err)
	}
	if released.LeaseID != "" || released.LeaseExpiresAt != 0 {
		t.Fatalf("expected lease cleared, got %#v", released)
	}
}

func TestSessionRepoClaimNextHasSingleWinnerAgainstPostgres(t *testing.T) {
	pg := postgrestest.Start(t)
	db, err := postgresruntime.OpenDB(context.Background(), pg.DSN)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	repo := sessionrepo.New(db)
	created, err := repo.Create("claim-race", "one winner")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	type claimResult struct {
		state session.State
		ok    bool
		err   error
	}

	results := make(chan claimResult, 2)
	now := time.Now().UnixMilli()
	for i := 0; i < 2; i++ {
		leaseID := "lease_" + string(rune('A'+i))
		go func(id string) {
			st, ok, err := repo.ClaimNext(session.ClaimModeRunnable, id, now, now+60_000)
			results <- claimResult{state: st, ok: ok, err: err}
		}(leaseID)
	}

	winners := 0
	empty := 0
	for i := 0; i < 2; i++ {
		result := <-results
		if result.err != nil {
			t.Fatalf("claim next: %v", result.err)
		}
		if result.ok {
			winners++
			if result.state.SessionID != created.SessionID {
				t.Fatalf("expected claimed session %s, got %#v", created.SessionID, result.state)
			}
			continue
		}
		empty++
	}

	if winners != 1 || empty != 1 {
		t.Fatalf("expected one winner and one empty result, got %d winners and %d empty", winners, empty)
	}
}
