package runtime

import (
	"encoding/json"

	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

const (
	programVerifyScopeMetadataKey     = "program_verify_scope"
	programAggregateVerifyMetadataKey = "program_aggregate_verify"
	verificationScopeMetadataKey      = "verification_scope"
)

func compiledProgramVerifyScope(node execution.ProgramNode, fanoutCount int) execution.VerificationScope {
	scope := node.VerifyScope
	if fanoutCount <= 1 {
		if scope == "" {
			return execution.VerificationScopeStep
		}
		return scope
	}
	switch scope {
	case "", execution.VerificationScopeTarget:
		return execution.VerificationScopeTarget
	case execution.VerificationScopeStep, execution.VerificationScopeAggregate:
		return execution.VerificationScopeAggregate
	default:
		return scope
	}
}

func compileProgramNodeVerifySpec(node execution.ProgramNode, fanoutCount int) verify.Spec {
	scope := compiledProgramVerifyScope(node, fanoutCount)
	if node.Verify == nil {
		return verify.Spec{}
	}
	if fanoutCount > 1 && scope == execution.VerificationScopeAggregate {
		return verify.Spec{}
	}
	return *node.Verify
}

func applyProgramVerifyMetadata(metadata map[string]any, node execution.ProgramNode, fanoutCount int) map[string]any {
	scope := compiledProgramVerifyScope(node, fanoutCount)
	if metadata == nil {
		metadata = map[string]any{}
	}
	if scope != "" {
		metadata[programVerifyScopeMetadataKey] = string(scope)
	}
	if fanoutCount > 1 && scope == execution.VerificationScopeAggregate && node.Verify != nil {
		metadata[programAggregateVerifyMetadataKey] = *node.Verify
	}
	return metadata
}

func programVerifyScopeFromStep(step plan.StepSpec) execution.VerificationScope {
	if len(step.Metadata) == 0 {
		return execution.VerificationScopeStep
	}
	scope, _ := step.Metadata[programVerifyScopeMetadataKey].(string)
	if scope == "" {
		return execution.VerificationScopeStep
	}
	return execution.VerificationScope(scope)
}

func programAggregateVerifySpecFromStep(step plan.StepSpec) (*verify.Spec, bool) {
	if len(step.Metadata) == 0 {
		return nil, false
	}
	raw, ok := step.Metadata[programAggregateVerifyMetadataKey]
	if !ok || raw == nil {
		return nil, false
	}
	switch typed := raw.(type) {
	case verify.Spec:
		spec := typed
		return &spec, true
	case *verify.Spec:
		if typed == nil {
			return nil, false
		}
		spec := *typed
		return &spec, true
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, false
	}
	var spec verify.Spec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, false
	}
	return &spec, true
}
