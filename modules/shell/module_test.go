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

type denyHook struct{}

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
