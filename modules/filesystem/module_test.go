package filesystemmodule_test

import (
	"context"
	"path/filepath"
	"testing"

	filesystemmodule "github.com/yiiilin/harness-core/modules/filesystem"
	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

func TestRegisterFilesystemModule(t *testing.T) {
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	filesystemmodule.Register(tools, verifiers)
	for _, name := range []string{"fs.exists", "fs.read", "fs.write", "fs.list"} {
		if _, ok := tools.Get(name); !ok {
			t.Fatalf("expected %s to be registered", name)
		}
	}
	for _, kind := range []string{"file_exists", "file_content_contains"} {
		found := false
		for _, def := range verifiers.List() {
			if def.Kind == kind {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected verifier %s to be registered", kind)
		}
	}
}

func TestFilesystemModuleHappyPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.txt")
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	filesystemmodule.Register(tools, verifiers)

	writeResult, err := tools.Invoke(context.Background(), action.Spec{ToolName: "fs.write", Args: map[string]any{"path": path, "content": "hello world"}})
	if err != nil {
		t.Fatalf("write invoke: %v", err)
	}
	if !writeResult.OK {
		t.Fatalf("expected write ok, got %#v", writeResult)
	}

	readResult, err := tools.Invoke(context.Background(), action.Spec{ToolName: "fs.read", Args: map[string]any{"path": path}})
	if err != nil {
		t.Fatalf("read invoke: %v", err)
	}
	if !readResult.OK {
		t.Fatalf("expected read ok, got %#v", readResult)
	}

	state := session.State{}
	res1, err := verifiers.Evaluate(context.Background(), verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{{Kind: "file_exists", Args: map[string]any{"path": path}}}}, readResult, state)
	if err != nil {
		t.Fatalf("verify file_exists: %v", err)
	}
	if !res1.Success {
		t.Fatalf("expected file_exists success, got %#v", res1)
	}
	res2, err := verifiers.Evaluate(context.Background(), verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{{Kind: "file_content_contains", Args: map[string]any{"path": path, "text": "hello"}}}}, readResult, state)
	if err != nil {
		t.Fatalf("verify file_content_contains: %v", err)
	}
	if !res2.Success {
		t.Fatalf("expected file_content_contains success, got %#v", res2)
	}
}

func TestDefaultPolicyRules(t *testing.T) {
	rules := filesystemmodule.DefaultPolicyRules()
	if len(rules) < 4 {
		t.Fatalf("expected policy rules, got %d", len(rules))
	}
}
