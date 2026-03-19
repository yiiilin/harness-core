package shellmodule

import (
	"context"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	shellexec "github.com/yiiilin/harness-core/pkg/harness/executor/shell"
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type Options struct {
	Backend     shellexec.Backend
	SandboxHook shellexec.SandboxHook
}

type handler struct {
	backend shellexec.Backend
	hook    shellexec.SandboxHook
}

func (h handler) Invoke(ctx context.Context, args map[string]any) (action.Result, error) {
	req := shellexec.Request{}
	if v, _ := args["mode"].(string); v != "" {
		req.Mode = v
	}
	if v, _ := args["command"].(string); v != "" {
		req.Command = v
	}
	if v, _ := args["cwd"].(string); v != "" {
		req.CWD = v
	}
	if v, ok := args["timeout_ms"]; ok {
		req.TimeoutMS = asInt(v)
	}
	decision, err := h.hook.BeforeExecute(ctx, req)
	if err != nil {
		return action.Result{OK: false, Error: &action.Error{Code: "SANDBOX_HOOK_FAILED", Message: err.Error()}}, nil
	}
	if decision.Action != "allow" {
		return action.Result{OK: false, Error: &action.Error{Code: "SANDBOX_BLOCKED", Message: decision.Reason}, Data: map[string]any{"sandbox_action": decision.Action}}, nil
	}
	result, err := h.backend.Execute(ctx, req)
	if err != nil {
		return result, err
	}
	if err := h.hook.AfterExecute(ctx, req, result); err != nil {
		result.Error = &action.Error{Code: "SANDBOX_AFTER_HOOK_FAILED", Message: err.Error()}
		result.OK = false
	}
	return result, nil
}

func Register(tools *tool.Registry, verifiers *verify.Registry) {
	RegisterWithOptions(tools, verifiers, Options{})
}

func RegisterWithOptions(tools *tool.Registry, verifiers *verify.Registry, opts Options) {
	backend := opts.Backend
	if backend == nil {
		backend = shellexec.PipeExecutor{}
	}
	hook := opts.SandboxHook
	if hook == nil {
		hook = shellexec.NoopSandboxHook{}
	}
	if tools != nil {
		tools.Register(tool.Definition{
			ToolName:       "shell.exec",
			Version:        "v1",
			CapabilityType: "executor",
			RiskLevel:      tool.RiskMedium,
			Enabled:        true,
			Metadata: map[string]any{
				"module": "shell",
				"modes":  []string{"pipe"},
				"extensible": map[string]any{
					"backend":      true,
					"sandbox_hook": true,
				},
			},
		}, handler{backend: backend, hook: hook})
	}
	if verifiers != nil {
		verifiers.Register(verify.Definition{Kind: "exit_code", Description: "Verify that an execution result exit code is in the allowed set."}, verify.ExitCodeChecker{})
		verifiers.Register(verify.Definition{Kind: "output_contains", Description: "Verify that stdout or stderr contains a target substring."}, verify.OutputContainsChecker{})
	}
}

func DefaultPolicyRules() []permission.Rule {
	return []permission.Rule{
		{Permission: "shell.exec", Pattern: "mode=pipe", Action: permission.Allow},
		{Permission: "shell.exec", Pattern: "*", Action: permission.Ask},
	}
}

func asInt(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int32:
		return int(x)
	case int64:
		return int(x)
	case float32:
		return int(x)
	case float64:
		return int(x)
	default:
		return 0
	}
}
