package capability

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
)

type RegistryFreezer struct {
	Registry *tool.Registry
}

func (r RegistryFreezer) Freeze(_ context.Context, sessionID, taskID string) (View, error) {
	view := View{
		ViewID:    "capv_" + uuid.NewString(),
		SessionID: sessionID,
		TaskID:    taskID,
		FrozenAt:  time.Now().UnixMilli(),
	}
	if r.Registry == nil {
		return view, nil
	}
	definitions := r.Registry.List()
	view.Entries = make([]Snapshot, 0, len(definitions))
	for _, def := range definitions {
		if !def.Enabled {
			continue
		}
		view.Entries = append(view.Entries, Snapshot{
			SessionID:      sessionID,
			TaskID:         taskID,
			ViewID:         view.ViewID,
			Scope:          SnapshotScopePlan,
			ToolName:       def.ToolName,
			Version:        def.Version,
			CapabilityType: def.CapabilityType,
			RiskLevel:      def.RiskLevel,
			Metadata:       cloneMap(def.Metadata),
			ResolvedAt:     view.FrozenAt,
		})
	}
	return view, nil
}
