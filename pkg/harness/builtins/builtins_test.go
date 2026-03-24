package builtins

import (
	"context"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/execution"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
)

func TestRegisterAppliesBuiltinModules(t *testing.T) {
	opts := hruntime.Options{}
	Register(&opts)
	if opts.Tools == nil {
		t.Fatalf("expected tools registry to be initialized")
	}
	if opts.Verifiers == nil {
		t.Fatalf("expected verifier registry to be initialized")
	}
	if len(opts.Tools.List()) < 2 {
		t.Fatalf("expected builtin tools to be registered, got %d", len(opts.Tools.List()))
	}
	if len(opts.Verifiers.List()) < 2 {
		t.Fatalf("expected builtin verifiers to be registered, got %d", len(opts.Verifiers.List()))
	}
	for _, kind := range []string{"value_equals", "string_matches_at", "tcp_port_open", "http_eventually_status_code"} {
		found := false
		for _, def := range opts.Verifiers.List() {
			if def.Kind == kind {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected builtin verifier %q to be registered", kind)
		}
	}
}

func TestNewBuildsConvenienceRuntimeWithBuiltinModules(t *testing.T) {
	rt := New()
	if len(rt.ListTools()) < 2 {
		t.Fatalf("expected built-in tools to be registered")
	}
	if len(rt.ListVerifiers()) < 2 {
		t.Fatalf("expected built-in verifiers to be registered")
	}
}

func TestRegisterAlsoWiresInteractiveController(t *testing.T) {
	opts := hruntime.Options{}
	Register(&opts)
	if opts.InteractiveController == nil {
		t.Fatalf("expected builtins register to wire an interactive controller")
	}

	rt := hruntime.New(opts)
	sess, err := rt.CreateSession("builtins interactive", "exercise builtins interactive controller")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	started, err := rt.StartInteractive(context.Background(), sess.SessionID, hruntime.InteractiveStartRequest{
		Kind: "pty",
		Spec: map[string]any{"command": "cat"},
	})
	if err != nil {
		t.Fatalf("start interactive: %v", err)
	}
	if started.Handle.HandleID == "" || !started.Capabilities.View || !started.Capabilities.Write || !started.Capabilities.Close {
		t.Fatalf("unexpected builtins interactive runtime: %#v", started)
	}
	if _, err := rt.CloseInteractive(context.Background(), started.Handle.HandleID, hruntime.InteractiveCloseRequest{Reason: "builtins cleanup"}); err != nil {
		t.Fatalf("close interactive: %v", err)
	}
	handle, err := rt.GetRuntimeHandle(started.Handle.HandleID)
	if err != nil {
		t.Fatalf("get runtime handle: %v", err)
	}
	if handle.Status != execution.RuntimeHandleClosed {
		t.Fatalf("expected builtins close to persist closed handle, got %#v", handle)
	}
}
