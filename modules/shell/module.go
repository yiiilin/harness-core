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
	PTYManager  *PTYManager
	SandboxHook shellexec.SandboxHook
}

type handler struct {
	backend    shellexec.Backend
	ptyBackend shellexec.Backend
	hook       shellexec.SandboxHook
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
	if env := asAnyMap(args["env"]); len(env) > 0 {
		req.Env = env
	}
	if metadata := asAnyMap(args["metadata"]); len(metadata) > 0 {
		req.Metadata = metadata
	}
	if v, ok := args["timeout_ms"]; ok {
		req.TimeoutMS = asInt(v)
	}
	if req.Mode == "" {
		req.Mode = "pipe"
	}
	decision, err := h.hook.BeforeExecute(ctx, req)
	if err != nil {
		return action.Result{OK: false, Error: &action.Error{Code: "SANDBOX_HOOK_FAILED", Message: err.Error()}}, nil
	}
	if decision.Action != "allow" {
		return action.Result{OK: false, Error: &action.Error{Code: "SANDBOX_BLOCKED", Message: decision.Reason}, Data: map[string]any{"sandbox_action": decision.Action}}, nil
	}
	backend := h.backend
	if req.Mode == "pty" {
		backend = h.ptyBackend
	}
	if backend == nil {
		return action.Result{OK: false, Error: &action.Error{Code: "SHELL_MODE_UNSUPPORTED", Message: req.Mode}}, nil
	}
	result, err := backend.Execute(ctx, req)
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
	ptyManager := opts.PTYManager
	if ptyManager == nil {
		ptyManager = NewPTYManager(PTYManagerOptions{})
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
				"modes":  []string{"pipe", "pty"},
				"extensible": map[string]any{
					"backend":      true,
					"pty_manager":  true,
					"sandbox_hook": true,
				},
			},
		}, handler{backend: backend, ptyBackend: PTYBackend{Manager: ptyManager}, hook: hook})
	}
	if verifiers != nil {
		verifiers.Register(verify.Definition{Kind: "exit_code", Description: "Verify that an execution result exit code is in the allowed set."}, verify.ExitCodeChecker{})
		verifiers.Register(verify.Definition{Kind: "output_contains", Description: "Verify that stdout or stderr contains a target substring."}, verify.OutputContainsChecker{})
		verifiers.Register(verify.Definition{Kind: "pty_handle_active", Description: "Verify that a PTY-backed shell result still has an active handle."}, PTYHandleActiveChecker{Manager: ptyManager})
		verifiers.Register(verify.Definition{Kind: "pty_stream_contains", Description: "Verify that a PTY-backed shell stream contains a target substring within a timeout."}, PTYStreamContainsChecker{Manager: ptyManager})
		verifiers.Register(verify.Definition{Kind: "pty_exit_code", Description: "Verify that a PTY-backed shell process exits with an allowed code within a timeout."}, PTYExitCodeChecker{Manager: ptyManager})
	}
}

func DefaultPolicyRules() []permission.Rule {
	return []permission.Rule{
		{Permission: "shell.exec", Pattern: "mode=pipe", Action: permission.Allow},
		{Permission: "shell.exec", Pattern: "mode=pty", Action: permission.Ask},
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

func asAnyMap(v any) map[string]any {
	raw, ok := v.(map[string]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make(map[string]any, len(raw))
	for key, value := range raw {
		out[key] = value
	}
	return out
}

func cloneAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]any, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

func stringifyEnv(src map[string]any) map[string]string {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]string, len(src))
	for key, value := range src {
		if key == "" {
			continue
		}
		if text, ok := value.(string); ok {
			out[key] = text
		}
	}
	return out
}
