package httpmodule_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	httpmodule "github.com/yiiilin/harness-core/modules/http"
	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

func TestRegisterHTTPModule(t *testing.T) {
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	httpmodule.Register(tools, verifiers)
	for _, name := range []string{"http.fetch", "http.post_json"} {
		if _, ok := tools.Get(name); !ok {
			t.Fatalf("expected %s to be registered", name)
		}
	}
	for _, kind := range []string{"http_status_code", "body_contains", "json_field_equals"} {
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

func TestHTTPModuleHappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"message":"hello"}`))
	}))
	defer srv.Close()

	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	httpmodule.Register(tools, verifiers)

	result, err := tools.Invoke(context.Background(), action.Spec{ToolName: "http.fetch", Args: map[string]any{"url": srv.URL}})
	if err != nil {
		t.Fatalf("http.fetch invoke: %v", err)
	}
	if !result.OK {
		t.Fatalf("expected fetch ok, got %#v", result)
	}
	state := session.State{}
	res1, err := verifiers.Evaluate(context.Background(), verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{{Kind: "http_status_code", Args: map[string]any{"allowed": []any{200}}}}}, result, state)
	if err != nil {
		t.Fatalf("verify http_status_code: %v", err)
	}
	if !res1.Success {
		t.Fatalf("expected http_status_code success, got %#v", res1)
	}
	res2, err := verifiers.Evaluate(context.Background(), verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{{Kind: "body_contains", Args: map[string]any{"text": "hello"}}, {Kind: "json_field_equals", Args: map[string]any{"field": "ok", "expected": true}}}}, result, state)
	if err != nil {
		t.Fatalf("verify content/json: %v", err)
	}
	if !res2.Success {
		t.Fatalf("expected body/json verifiers success, got %#v", res2)
	}
}

func TestDefaultPolicyRules(t *testing.T) {
	rules := httpmodule.DefaultPolicyRules()
	if len(rules) < 2 {
		t.Fatalf("expected policy rules, got %d", len(rules))
	}
}
