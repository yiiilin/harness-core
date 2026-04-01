package runtime

import "github.com/yiiilin/harness-core/pkg/harness/plan"

const (
	defaultTransportMaxBytes = 16 * 1024
	defaultInlineMaxChars    = 8192
	defaultPlannerMaxChars   = 8192
)

type RuntimePolicy struct {
	Output  OutputPolicy  `json:"output,omitempty"`
	Planner PlannerPolicy `json:"planner,omitempty"`
}

type OutputPolicy struct {
	Defaults      OutputModePolicy            `json:"defaults,omitempty"`
	ToolOverrides map[string]OutputModePolicy `json:"tool_overrides,omitempty"`
	StepOverrides map[string]OutputModePolicy `json:"step_overrides,omitempty"`
}

type OutputModePolicy struct {
	Transport TransportBudgetPolicy `json:"transport,omitempty"`
	Inline    InlineBudgetPolicy    `json:"inline,omitempty"`
	Raw       RawResultPolicy       `json:"raw,omitempty"`
}

type TransportBudgetPolicy struct {
	MaxBytes int `json:"max_bytes,omitempty"`
}

type InlineBudgetPolicy struct {
	MaxChars int `json:"max_chars,omitempty"`
}

type RawRetentionMode string

const RawRetentionBackendDefined RawRetentionMode = "backend_defined"

type RawResultPolicy struct {
	RetentionMode RawRetentionMode `json:"retention_mode,omitempty"`
}

type PlannerPolicy struct {
	Projection PlannerProjectionPolicy    `json:"projection,omitempty"`
	Context    PlannerContextBudgetPolicy `json:"context,omitempty"`
}

type PlannerProjectionMode string

const (
	PlannerProjectionInline PlannerProjectionMode = "inline"
	PlannerProjectionRaw    PlannerProjectionMode = "raw"
	PlannerProjectionCustom PlannerProjectionMode = "custom"
)

type PlannerProjectionPolicy struct {
	Mode      PlannerProjectionMode `json:"mode,omitempty"`
	Projector PlannerProjector      `json:"-"`
}

type PlannerContextBudgetPolicy struct {
	MaxChars int `json:"max_chars,omitempty"`
}

func defaultRuntimePolicy() RuntimePolicy {
	return RuntimePolicy{
		Output: OutputPolicy{
			Defaults: OutputModePolicy{
				Transport: TransportBudgetPolicy{MaxBytes: defaultTransportMaxBytes},
				Inline:    InlineBudgetPolicy{MaxChars: defaultInlineMaxChars},
				Raw:       RawResultPolicy{RetentionMode: RawRetentionBackendDefined},
			},
		},
		Planner: PlannerPolicy{
			Context: PlannerContextBudgetPolicy{MaxChars: defaultPlannerMaxChars},
		},
	}
}

func normalizeRuntimePolicy(policy RuntimePolicy) RuntimePolicy {
	defaults := defaultRuntimePolicy()
	policy.Output.Defaults = mergeOutputModePolicy(defaults.Output.Defaults, policy.Output.Defaults)
	if policy.Planner.Context.MaxChars <= 0 {
		policy.Planner.Context.MaxChars = defaults.Planner.Context.MaxChars
	}
	if policy.Planner.Projection.Projector == nil && defaults.Planner.Projection.Projector != nil {
		policy.Planner.Projection.Projector = defaults.Planner.Projection.Projector
	}
	return policy
}

func mergeOutputModePolicy(base, override OutputModePolicy) OutputModePolicy {
	if override.Transport.MaxBytes > 0 {
		base.Transport = override.Transport
	}
	if override.Inline.MaxChars > 0 {
		base.Inline = override.Inline
	}
	if override.Raw.RetentionMode != "" {
		base.Raw = override.Raw
	}
	return base
}

func (s *Service) outputPolicyForStep(step plan.StepSpec) OutputModePolicy {
	policy := s.RuntimePolicy.Output.Defaults
	if override, ok := s.RuntimePolicy.Output.ToolOverrides[step.Action.ToolName]; ok {
		policy = mergeOutputModePolicy(policy, override)
	}
	if override, ok := s.RuntimePolicy.Output.StepOverrides[step.StepID]; ok {
		policy = mergeOutputModePolicy(policy, override)
	}
	return policy
}
