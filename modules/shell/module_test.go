package shellmodule_test

import (
	"context"
	"testing"

	shellmodule "github.com/yiiilin/harness-core/modules/shell"
	"github.com/yiiilin/harness-core/pkg/harness/action"
	shellexec "github.com/yiiilin/harness-core/pkg/harness/executor/shell"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type stubPTYBackend struct {
	lastRequest shellexec.Request
	called      bool
}

func (s *stubPTYBackend) Execute(ctx context.Context, req shellexec.Request) (action.Result, error) {
	s.called = true
	s.lastRequest = req
	return action.Result{OK: true, Data: map[string]any{"marker": "stub-pty", "mode": req.Mode, "command": req.Command}}, nil
}

type denyHook struct{}
type recordingBackend struct {
	result action.Result
	calls  int
	last   shellexec.Request
}

func (b *recordingBackend) Execute(_ context.Context, req shellexec.Request) (action.Result, error) {
	b.calls++
	b.last = req
	return b.result, nil
}

func (denyHook) BeforeExecute(_ context.Context, _ shellexec.Request) (shellexec.SandboxDecision, error) {
	return shellexec.SandboxDecision{Action: "deny", Reason: "blocked by test hook"}, nil
}

func (denyHook) AfterExecute(_ context.Context, _ shellexec.Request, _ action.Result) error {
	return nil
}

func TestRegisterShellModule(t *testing.T) {
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	shellmodule.Register(tools, verifiers)
	def, ok := tools.Get("shell.exec")
	if !ok {
		t.Fatalf("expected shell.exec to be registered")
	}
	modes, _ := def.Definition.Metadata["modes"].([]string)
	if len(modes) < 2 {
		t.Fatalf("expected shell module metadata to advertise pipe and pty modes, got %#v", def.Definition.Metadata)
	}
	if len(verifiers.List()) < 5 {
		t.Fatalf("expected shell and PTY verifiers to be registered, got %#v", verifiers.List())
	}
}

func TestRegisterShellModuleWithSandboxHook(t *testing.T) {
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	shellmodule.RegisterWithOptions(tools, verifiers, shellmodule.Options{SandboxHook: denyHook{}})
	result, err := tools.Invoke(context.Background(), action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo hello"}})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if result.OK {
		t.Fatalf("expected blocked result, got %#v", result)
	}
	if result.Error == nil || result.Error.Code != "SANDBOX_BLOCKED" {
		t.Fatalf("expected SANDBOX_BLOCKED, got %#v", result)
	}
}

func TestRegisterShellModuleWithExplicitPTYBackend(t *testing.T) {
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	remotePTY := &recordingBackend{
		result: action.Result{
			OK: true,
			Data: map[string]any{
				"mode":   "pty",
				"status": "remote",
			},
		},
	}

	shellmodule.RegisterWithOptions(tools, verifiers, shellmodule.Options{PTYBackend: remotePTY})

	result, err := tools.Invoke(context.Background(), action.Spec{
		ToolName: "shell.exec",
		Args: map[string]any{
			"mode":    "pty",
			"command": "echo remote",
		},
	})
	if err != nil {
		t.Fatalf("invoke pty shell through explicit backend: %v", err)
	}
	if !result.OK || result.Data["status"] != "remote" {
		t.Fatalf("expected explicit PTY backend result, got %#v", result)
	}
	if remotePTY.calls != 1 {
		t.Fatalf("expected explicit PTY backend to be called once, got %d", remotePTY.calls)
	}
	if remotePTY.last.Mode != "pty" || remotePTY.last.Command != "echo remote" {
		t.Fatalf("unexpected PTY backend request: %#v", remotePTY.last)
	}
	def, ok := tools.Get("shell.exec")
	if !ok {
		t.Fatalf("expected shell.exec definition")
	}
	if ptyVerifiers, _ := def.Definition.Metadata["pty_verifiers"].(bool); ptyVerifiers {
		t.Fatalf("expected pty_verifiers metadata to be false without local PTY manager, got %#v", def.Definition.Metadata)
	}

	for _, kind := range []string{"exit_code", "output_contains"} {
		if _, ok := verifiers.Get(kind); !ok {
			t.Fatalf("expected builtin shell verifier %q to remain registered", kind)
		}
	}
	for _, kind := range []string{"pty_handle_active", "pty_stream_contains", "pty_exit_code"} {
		if _, ok := verifiers.Get(kind); ok {
			t.Fatalf("did not expect PTY verifier %q without local PTY manager", kind)
		}
	}
}

func TestRegisterShellModuleWithExplicitPTYBackendAndManagerRegistersPTYVerifiers(t *testing.T) {
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	manager := shellmodule.NewPTYManager(shellmodule.PTYManagerOptions{})
	t.Cleanup(func() {
		_ = manager.CloseAll(context.Background(), "test cleanup")
	})
	remotePTY := &recordingBackend{
		result: action.Result{
			OK: true,
			Data: map[string]any{
				"mode":   "pty",
				"status": "remote",
			},
		},
	}

	shellmodule.RegisterWithOptions(tools, verifiers, shellmodule.Options{
		PTYBackend: remotePTY,
		PTYManager: manager,
	})
	def, ok := tools.Get("shell.exec")
	if !ok {
		t.Fatalf("expected shell.exec definition")
	}
	if ptyVerifiers, _ := def.Definition.Metadata["pty_verifiers"].(bool); !ptyVerifiers {
		t.Fatalf("expected pty_verifiers metadata to be true with local PTY manager, got %#v", def.Definition.Metadata)
	}

	for _, kind := range []string{"pty_handle_active", "pty_stream_contains", "pty_exit_code"} {
		if _, ok := verifiers.Get(kind); !ok {
			t.Fatalf("expected PTY verifier %q with explicit local PTY manager", kind)
		}
	}
}

func TestDefaultPolicyRules(t *testing.T) {
	rules := shellmodule.DefaultPolicyRules()
	if len(rules) < 3 {
		t.Fatalf("expected explicit pipe/pty policy rules, got %#v", rules)
	}
	if rules[0].Pattern != "mode=pipe" || rules[0].Action != "allow" {
		t.Fatalf("expected pipe mode allow rule first, got %#v", rules[0])
	}
	if rules[1].Pattern != "mode=pty" || rules[1].Action != "ask" {
		t.Fatalf("expected PTY mode ask rule second, got %#v", rules[1])
	}
}
