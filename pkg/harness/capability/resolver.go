package capability

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
)

type RegistryResolver struct {
	Registry *tool.Registry
}

func (r RegistryResolver) Match(ctx context.Context, req Request) (MatchResult, error) {
	resolution, err := r.Resolve(ctx, req)
	if err != nil {
		reason, ok := UnsupportedReasonFromError(err, req)
		if ok {
			return MatchResult{
				Supported: false,
				Reasons:   []UnsupportedReason{reason},
			}, nil
		}
		return MatchResult{}, err
	}
	reasons := UnsupportedReasonsForDefinition(resolution.Definition, req.Requirements)
	if len(reasons) > 0 {
		return MatchResult{
			Supported: false,
			Reasons:   reasons,
		}, nil
	}
	return MatchResult{
		Supported:  true,
		Resolution: &resolution,
	}, nil
}

func (r RegistryResolver) Resolve(_ context.Context, req Request) (Resolution, error) {
	if r.Registry == nil {
		return Resolution{}, ErrCapabilityNotFound
	}

	entry, ok := r.Registry.Resolve(req.Action.ToolName, req.Action.ToolVersion)
	if !ok {
		if req.Action.ToolVersion != "" {
			return Resolution{}, ErrCapabilityVersionNotFound
		}
		return Resolution{}, ErrCapabilityNotFound
	}
	if !entry.Definition.Enabled {
		return Resolution{}, ErrCapabilityDisabled
	}

	return Resolution{
		Snapshot: Snapshot{
			SnapshotID:     "cap_" + uuid.NewString(),
			SessionID:      req.SessionID,
			TaskID:         req.TaskID,
			StepID:         req.StepID,
			Scope:          SnapshotScopeAction,
			ToolName:       entry.Definition.ToolName,
			Version:        entry.Definition.Version,
			CapabilityType: entry.Definition.CapabilityType,
			RiskLevel:      entry.Definition.RiskLevel,
			Metadata:       cloneMap(entry.Definition.Metadata),
			ResolvedAt:     time.Now().UnixMilli(),
		},
		Definition: entry.Definition,
		Handler:    entry.Handler,
	}, nil
}

func cloneMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
