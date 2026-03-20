package runtime_test

import (
	"context"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/builtins"
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

func TestRegisterBuiltinsComposesModuleDefaultPolicyRules(t *testing.T) {
	opts := hruntime.Options{}
	builtins.Register(&opts)
	rt := hruntime.New(opts)

	sess := session.State{SessionID: "sess_policy", Phase: session.PhaseExecute}

	writeDecision, err := rt.EvaluatePolicy(context.Background(), sess, plan.StepSpec{
		StepID: "step_write",
		Action: action.Spec{ToolName: "fs.write", Args: map[string]any{"path": "/tmp/demo.txt", "content": "hello"}},
	})
	if err != nil {
		t.Fatalf("evaluate fs.write policy: %v", err)
	}
	if writeDecision.Action != permission.Ask {
		t.Fatalf("expected fs.write to require approval from module default rules, got %#v", writeDecision)
	}

	httpDecision, err := rt.EvaluatePolicy(context.Background(), sess, plan.StepSpec{
		StepID: "step_http_post",
		Action: action.Spec{ToolName: "http.post_json", Args: map[string]any{"url": "https://example.test", "json": map[string]any{"hello": "world"}}},
	})
	if err != nil {
		t.Fatalf("evaluate http.post_json policy: %v", err)
	}
	if httpDecision.Action != permission.Ask {
		t.Fatalf("expected http.post_json to require approval from module default rules, got %#v", httpDecision)
	}

	readDecision, err := rt.EvaluatePolicy(context.Background(), sess, plan.StepSpec{
		StepID: "step_read",
		Action: action.Spec{ToolName: "fs.read", Args: map[string]any{"path": "/tmp/demo.txt"}},
	})
	if err != nil {
		t.Fatalf("evaluate fs.read policy: %v", err)
	}
	if readDecision.Action != permission.Allow {
		t.Fatalf("expected fs.read to stay allowed via module default rules, got %#v", readDecision)
	}
}
