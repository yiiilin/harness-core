package shell

import (
	"context"

	"github.com/yiiilin/harness-core/pkg/harness/action"
)

type Request struct {
	Mode           string         `json:"mode,omitempty"`
	Command        string         `json:"command"`
	CWD            string         `json:"cwd,omitempty"`
	Env            map[string]any `json:"env,omitempty"`
	TimeoutMS      int            `json:"timeout_ms,omitempty"`
	MaxOutputBytes int            `json:"max_output_bytes,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

type Backend interface {
	Execute(ctx context.Context, req Request) (action.Result, error)
}

type SandboxDecision struct {
	Action string         `json:"action"`
	Reason string         `json:"reason,omitempty"`
	Meta   map[string]any `json:"meta,omitempty"`
}

type SandboxHook interface {
	BeforeExecute(ctx context.Context, req Request) (SandboxDecision, error)
	AfterExecute(ctx context.Context, req Request, result action.Result) error
}

type NoopSandboxHook struct{}

func (NoopSandboxHook) BeforeExecute(_ context.Context, _ Request) (SandboxDecision, error) {
	return SandboxDecision{Action: "allow", Reason: "no sandbox hook configured"}, nil
}

func (NoopSandboxHook) AfterExecute(_ context.Context, _ Request, _ action.Result) error {
	return nil
}
