package runtime_test

import (
	"context"
	"testing"
	"time"

	"github.com/yiiilin/harness-core/internal/postgrestest"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
)

func TestPostgresClaimRunnableSessionHasSingleWinner(t *testing.T) {
	pg := postgrestest.Start(t)
	rt, db := pg.OpenService(t, hruntime.Options{})
	defer db.Close()

	mustCreateSession(t, rt, "pg-claim-race", "one winner")

	type claimResult struct {
		ok  bool
		err error
	}

	results := make(chan claimResult, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, ok, err := rt.ClaimRunnableSession(context.Background(), time.Minute)
			results <- claimResult{ok: ok, err: err}
		}()
	}

	winners := 0
	empty := 0
	for i := 0; i < 2; i++ {
		result := <-results
		if result.err != nil {
			t.Fatalf("claim runnable session: %v", result.err)
		}
		if result.ok {
			winners++
			continue
		}
		empty++
	}

	if winners != 1 || empty != 1 {
		t.Fatalf("expected one winner and one empty result, got %d winners and %d empty", winners, empty)
	}
}
