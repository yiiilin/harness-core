package runtime

import (
	"context"

	"github.com/yiiilin/harness-core/pkg/harness/persistence"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

func (s *Service) ensureRuntimeBudgetAnchor(ctx context.Context, state session.State, leaseID string, startedAt int64) (session.State, error) {
	if state.RuntimeStartedAt != 0 {
		return state, nil
	}

	updated := state
	updated.RuntimeStartedAt = startedAt
	persist := func(store session.Store) error {
		next, err := persistSessionUpdateAt(store, updated, leaseID, startedAt)
		if err != nil {
			return err
		}
		updated = next
		return nil
	}

	if s.Runner != nil {
		if err := s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
			return persist(s.repositoriesWithFallback(repos).Sessions)
		}); err != nil {
			return session.State{}, err
		}
		return updated, nil
	}

	if err := persist(s.Sessions); err != nil {
		return session.State{}, err
	}
	return updated, nil
}
