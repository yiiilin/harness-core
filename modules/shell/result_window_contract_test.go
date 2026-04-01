package shellmodule_test

import (
	"context"
	"testing"

	shellmodule "github.com/yiiilin/harness-core/modules/shell"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
)

func TestInteractiveViewExposesUnifiedWindowAndRawHandleContract(t *testing.T) {
	ctx := context.Background()
	manager := shellmodule.NewPTYManager(shellmodule.PTYManagerOptions{})
	t.Cleanup(func() {
		_ = manager.CloseAll(ctx, "test cleanup")
	})

	rt := hruntime.New(hruntime.Options{
		InteractiveController: shellmodule.NewInteractiveController(manager),
		RuntimePolicy: hruntime.RuntimePolicy{
			Output: hruntime.OutputPolicy{
				Defaults: hruntime.OutputModePolicy{
					Transport: hruntime.TransportBudgetPolicy{MaxBytes: 5},
					Inline:    hruntime.InlineBudgetPolicy{MaxChars: 32},
					Raw:       hruntime.RawResultPolicy{RetentionMode: hruntime.RawRetentionBackendDefined},
				},
			},
		},
	})
	sess, err := rt.CreateSession("interactive result window", "expose one PTY preview contract")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	started, err := rt.StartInteractive(ctx, sess.SessionID, hruntime.InteractiveStartRequest{
		Kind: "pty",
		Spec: map[string]any{
			"command": "printf 'hello world'; sleep 2",
		},
	})
	if err != nil {
		t.Fatalf("start interactive: %v", err)
	}

	_ = waitForInteractiveViewContains(t, rt, started.Handle.HandleID, "hello world")

	viewed, err := rt.ViewInteractive(ctx, started.Handle.HandleID, hruntime.InteractiveViewRequest{
		Offset: 0,
	})
	if err != nil {
		t.Fatalf("view interactive: %v", err)
	}
	if viewed.Window == nil || !viewed.Window.Truncated || viewed.Window.ReturnedBytes != len("hello") {
		t.Fatalf("expected unified preview window contract, got %#v", viewed)
	}
	if viewed.RawHandle == nil || viewed.RawHandle.Ref != started.Handle.HandleID || !viewed.RawHandle.Reread {
		t.Fatalf("expected unified raw handle contract, got %#v", viewed)
	}
}
