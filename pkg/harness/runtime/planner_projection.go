package runtime

import (
	"context"
	"sort"

	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

func (s *Service) ProjectPlannerContext(ctx context.Context, assembled ContextPackage, state session.State, spec task.Spec) (ContextPackage, error) {
	policy := s.RuntimePolicy.Planner
	switch policy.Projection.Mode {
	case "":
		return ContextPackage{}, ErrPlannerProjectionPolicyRequired
	case PlannerProjectionRaw:
		return cloneContextPackage(assembled), nil
	case PlannerProjectionInline:
		return projectContextPackageInline(assembled, policy.Context.MaxChars), nil
	case PlannerProjectionCustom:
		if policy.Projection.Projector == nil {
			return ContextPackage{}, ErrPlannerProjectorRequired
		}
		return policy.Projection.Projector.ProjectPlannerContext(ctx, cloneContextPackage(assembled), state, spec, policy)
	default:
		return ContextPackage{}, ErrPlannerProjectionModeUnsupported
	}
}

func cloneContextPackage(pkg ContextPackage) ContextPackage {
	out := pkg
	out.Constraints = cloneAnyMap(pkg.Constraints)
	out.Metadata = cloneAnyMap(pkg.Metadata)
	out.Derived = cloneAnyMap(pkg.Derived)
	out.Extras = cloneAnyMap(pkg.Extras)
	if pkg.Compaction != nil {
		compaction := *pkg.Compaction
		compaction.Metadata = cloneAnyMap(pkg.Compaction.Metadata)
		out.Compaction = &compaction
	}
	return out
}

func projectContextPackageInline(pkg ContextPackage, maxChars int) ContextPackage {
	out := cloneContextPackage(pkg)
	if maxChars <= 0 {
		return out
	}
	remaining := maxChars
	out.Task.Goal = projectStringWithRemaining(out.Task.Goal, &remaining)
	out.Constraints = projectAnyMapInline(out.Constraints, &remaining)
	out.Metadata = projectAnyMapInline(out.Metadata, &remaining)
	out.Derived = projectAnyMapInline(out.Derived, &remaining)
	out.Extras = projectAnyMapInline(out.Extras, &remaining)
	if out.Compaction != nil {
		out.Compaction.Metadata = projectAnyMapInline(out.Compaction.Metadata, &remaining)
	}
	return out
}

func projectAnyMapInline(in map[string]any, remaining *int) map[string]any {
	if len(in) == 0 {
		return in
	}
	out := make(map[string]any, len(in))
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		out[key] = projectAnyInline(in[key], remaining)
	}
	return out
}

func projectAnyInline(value any, remaining *int) any {
	switch v := value.(type) {
	case string:
		return projectStringWithRemaining(v, remaining)
	case map[string]any:
		return projectAnyMapInline(v, remaining)
	case []any:
		out := make([]any, len(v))
		for i := range v {
			out[i] = projectAnyInline(v[i], remaining)
		}
		return out
	case []string:
		out := make([]string, len(v))
		for i := range v {
			out[i] = projectStringWithRemaining(v[i], remaining)
		}
		return out
	default:
		return value
	}
}

func projectStringWithRemaining(value string, remaining *int) string {
	if remaining == nil || *remaining <= 0 || value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= *remaining {
		*remaining -= len(runes)
		return value
	}
	projected := string(runes[:*remaining])
	*remaining = 0
	return projected
}
