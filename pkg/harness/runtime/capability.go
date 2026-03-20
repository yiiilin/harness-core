package runtime

import (
	"errors"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/capability"
)

func capabilityErrorResult(spec action.Spec, err error) action.Result {
	switch {
	case errors.Is(err, capability.ErrCapabilityDisabled):
		return action.Result{OK: false, Error: &action.Error{Code: "CAPABILITY_DISABLED", Message: spec.ToolName}}
	case errors.Is(err, capability.ErrCapabilityVersionNotFound):
		return action.Result{OK: false, Error: &action.Error{Code: "CAPABILITY_VERSION_NOT_FOUND", Message: spec.ToolName}}
	default:
		return action.Result{OK: false, Error: &action.Error{Code: "CAPABILITY_NOT_FOUND", Message: spec.ToolName}}
	}
}
