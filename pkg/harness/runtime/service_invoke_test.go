package runtime_test

import (
	"context"
	"errors"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
)

func TestInvokeActionRejectsDirectBypassPath(t *testing.T) {
	rt := hruntime.New(hruntime.Options{})

	result, err := rt.InvokeAction(context.Background(), action.Spec{ToolName: "shell.exec"})
	if !errors.Is(err, hruntime.ErrDirectActionInvokeUnsupported) {
		t.Fatalf("expected ErrDirectActionInvokeUnsupported, got %v", err)
	}
	if result.OK {
		t.Fatalf("expected direct invoke result to fail, got %#v", result)
	}
	if result.Error == nil || result.Error.Code != "DIRECT_ACTION_INVOKE_UNSUPPORTED" {
		t.Fatalf("expected structured direct invoke error, got %#v", result)
	}
}
