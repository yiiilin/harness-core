package runtime

import (
	"context"

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
