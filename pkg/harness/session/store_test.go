package session

import "testing"

func TestMemoryStoreClaimNextUsesCreatedOrder(t *testing.T) {
	store := &MemoryStore{
		sessions: map[string]State{
			"sess_a": {
				SessionID:      "sess_a",
				Title:          "newer",
				Phase:          PhaseReceived,
				ExecutionState: ExecutionIdle,
				Version:        1,
				CreatedAt:      20,
				UpdatedAt:      20,
			},
			"sess_b": {
				SessionID:      "sess_b",
				Title:          "older",
				Phase:          PhaseReceived,
				ExecutionState: ExecutionIdle,
				Version:        1,
				CreatedAt:      10,
				UpdatedAt:      10,
			},
		},
	}

	claimed, ok, err := store.ClaimNext(ClaimModeRunnable, "lease_1", 100, 200)
	if err != nil {
		t.Fatalf("claim next: %v", err)
	}
	if !ok {
		t.Fatalf("expected a claim winner")
	}
	if claimed.SessionID != "sess_b" {
		t.Fatalf("expected oldest created session to win, got %#v", claimed)
	}
}
