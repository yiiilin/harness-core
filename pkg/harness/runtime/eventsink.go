package runtime

import (
	"context"
	"errors"

	"github.com/yiiilin/harness-core/pkg/harness/audit"
)

type AuditStoreSink struct {
	Store audit.Store
}

func (s AuditStoreSink) Emit(_ context.Context, event audit.Event) error {
	if s.Store == nil {
		return nil
	}
	return s.Store.Emit(event)
}

func (s AuditStoreSink) WithAuditStore(store audit.Store) EventSink {
	return AuditStoreSink{Store: store}
}

type FanoutEventSink struct {
	Sinks []EventSink
}

func (s FanoutEventSink) Emit(ctx context.Context, event audit.Event) error {
	var errs []error
	for _, sink := range s.Sinks {
		if sink == nil {
			continue
		}
		if err := sink.Emit(ctx, event); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s FanoutEventSink) WithAuditStore(store audit.Store) EventSink {
	rebound := make([]EventSink, 0, len(s.Sinks))
	hasAuditAwareChild := false
	for _, sink := range s.Sinks {
		if sink == nil {
			continue
		}
		if aware, ok := sink.(auditStoreAwareSink); ok {
			rebound = append(rebound, aware.WithAuditStore(store))
			hasAuditAwareChild = true
			continue
		}
		rebound = append(rebound, sink)
	}
	if len(rebound) == 0 {
		return AuditStoreSink{Store: store}
	}
	if store != nil && !hasAuditAwareChild {
		rebound = append(rebound, AuditStoreSink{Store: store})
	}
	return FanoutEventSink{Sinks: rebound}
}

type auditStoreAwareSink interface {
	WithAuditStore(store audit.Store) EventSink
}

func bindEventSinkToAuditStore(sink EventSink, store audit.Store) EventSink {
	if sink == nil {
		if store == nil {
			return nil
		}
		return AuditStoreSink{Store: store}
	}
	if store == nil {
		return sink
	}
	if aware, ok := sink.(auditStoreAwareSink); ok {
		return aware.WithAuditStore(store)
	}
	return FanoutEventSink{Sinks: []EventSink{sink, AuditStoreSink{Store: store}}}
}
