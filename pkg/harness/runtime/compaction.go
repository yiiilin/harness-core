package runtime

import (
	"context"

	"github.com/yiiilin/harness-core/pkg/harness/persistence"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

func (s *Service) CompactSessionContext(ctx context.Context, sessionID string, trigger CompactionTrigger) (ContextPackage, *ContextSummary, error) {
	state, err := s.GetSession(sessionID)
	if err != nil {
		return ContextPackage{}, nil, err
	}
	if state.TaskID == "" {
		return ContextPackage{}, nil, nil
	}
	rec, err := s.GetTask(state.TaskID)
	if err != nil {
		return ContextPackage{}, nil, err
	}
	spec := task.Spec{
		TaskID:      rec.TaskID,
		TaskType:    rec.TaskType,
		Goal:        rec.Goal,
		Constraints: rec.Constraints,
		Metadata:    rec.Metadata,
	}
	return s.compactAssembledContext(ctx, state, spec, trigger)
}

func (s *Service) compactAssembledContext(ctx context.Context, state session.State, spec task.Spec, trigger CompactionTrigger) (ContextPackage, *ContextSummary, error) {
	assembled, err := s.ContextAssembler.Assemble(ctx, state, spec)
	if err != nil {
		return ContextPackage{}, nil, err
	}
	return s.compactContextPackage(ctx, assembled, state, spec, trigger)
}

func (s *Service) compactContextPackage(ctx context.Context, assembled ContextPackage, state session.State, spec task.Spec, trigger CompactionTrigger) (ContextPackage, *ContextSummary, error) {
	previous, err := s.latestContextSummary(ctx, state.SessionID)
	if err != nil {
		return ContextPackage{}, nil, err
	}
	if previous != nil {
		if assembled.Extras == nil {
			assembled.Extras = map[string]any{}
		}
		assembled.Extras["previous_summary"] = map[string]any{
			"summary_id": previous.SummaryID,
			"trigger":    previous.Trigger,
			"summary":    previous.Summary,
			"metadata":   previous.Metadata,
		}
	}
	if !s.shouldCompact(trigger) || s.Compactor == nil {
		return assembled, nil, nil
	}

	compacted, summary, err := s.Compactor.Compact(ctx, assembled, state, spec, s.LoopBudgets)
	if err != nil {
		return ContextPackage{}, nil, err
	}
	assembled = compacted
	if summary == nil || s.ContextSummaries == nil {
		return assembled, nil, nil
	}

	if summary.SessionID == "" {
		summary.SessionID = state.SessionID
	}
	if summary.TaskID == "" {
		summary.TaskID = spec.TaskID
	}
	summary.Trigger = trigger
	if previous != nil && previous.SummaryID != "" && previous.SummaryID != summary.SummaryID {
		summary.SupersedesSummaryID = previous.SummaryID
	}
	persisted, err := s.persistContextSummary(ctx, *summary)
	if err != nil {
		return ContextPackage{}, nil, err
	}
	assembled.Compaction = &ContextCompaction{
		SummaryID:         persisted.SummaryID,
		PreviousSummaryID: persisted.SupersedesSummaryID,
		Trigger:           persisted.Trigger,
		Strategy:          persisted.Strategy,
		OriginalBytes:     persisted.OriginalBytes,
		CompactedBytes:    persisted.CompactedBytes,
		Metadata:          persisted.Metadata,
	}
	return assembled, &persisted, nil
}

func (s *Service) latestContextSummary(ctx context.Context, sessionID string) (*ContextSummary, error) {
	if sessionID == "" {
		return nil, nil
	}
	items, err := s.listContextSummaries(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}
	latest := items[len(items)-1]
	return &latest, nil
}

func (s *Service) shouldCompact(trigger CompactionTrigger) bool {
	switch trigger {
	case CompactionTriggerExecute:
		return s.CompactionPolicy.OnExecute
	case CompactionTriggerRecover:
		return s.CompactionPolicy.OnRecover
	case CompactionTriggerPlan:
		fallthrough
	default:
		return s.CompactionPolicy.OnPlan
	}
}

func (s *Service) compactSessionContextBestEffort(ctx context.Context, sessionID string, trigger CompactionTrigger) {
	_, _, _ = s.CompactSessionContext(ctx, sessionID, trigger)
}

func (s *Service) persistContextSummary(ctx context.Context, summary ContextSummary) (ContextSummary, error) {
	create := func(store ContextSummaryStore) (ContextSummary, error) {
		if store == nil {
			return ContextSummary{}, nil
		}
		return store.Create(summary)
	}
	if s.Runner != nil {
		var persisted ContextSummary
		err := s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
			var err error
			persisted, err = create(s.repositoriesWithFallback(repos).ContextSummaries)
			return err
		})
		return persisted, err
	}
	return create(s.ContextSummaries)
}

func (s *Service) listContextSummaries(ctx context.Context, sessionID string) ([]ContextSummary, error) {
	list := func(store ContextSummaryStore) ([]ContextSummary, error) {
		if store == nil {
			return nil, nil
		}
		return store.List(sessionID)
	}
	if s.Runner != nil {
		var items []ContextSummary
		err := s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
			var err error
			items, err = list(s.repositoriesWithFallback(repos).ContextSummaries)
			return err
		})
		return items, err
	}
	return list(s.ContextSummaries)
}
