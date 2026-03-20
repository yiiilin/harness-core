package runtime

import (
	"context"
	"fmt"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/audit"
)

func BenchmarkEmitEventsBatch100(b *testing.B) {
	ctx := context.Background()
	events := make([]audit.Event, 100)
	for i := range events {
		events[i] = audit.Event{
			EventID:   fmt.Sprintf("evt_%03d", i),
			Type:      audit.EventToolCompleted,
			SessionID: "bench-session",
			StepID:    fmt.Sprintf("step_%03d", i),
			Payload:   map[string]any{"tool_name": "shell.exec"},
			CreatedAt: 1770000000000 + int64(i),
		}
	}

	for i := 0; i < b.N; i++ {
		store := audit.NewMemoryStore()
		svc := Service{Audit: store}
		svc.emitEvents(ctx, events)
	}
}
