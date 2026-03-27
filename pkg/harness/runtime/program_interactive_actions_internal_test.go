package runtime

import (
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
)

func TestExtractRuntimeHandlesSkipsNativePersistedInteractiveResults(t *testing.T) {
	handles := extractRuntimeHandles(action.Result{
		Data: map[string]any{
			"runtime_handle": execution.RuntimeHandle{
				HandleID: "hdl_native_interactive",
				Kind:     "pty",
				Value:    "backend",
			},
		},
		Meta: map[string]any{
			"_native_runtime_handles_persisted": true,
		},
	}, execution.Attempt{
		SessionID: "sess_native_interactive",
		TaskID:    "task_native_interactive",
		AttemptID: "att_native_interactive",
		CycleID:   "cyc_native_interactive",
		TraceID:   "trc_native_interactive",
	}, &execution.ActionRecord{ActionID: "act_native_interactive"}, 100)

	if len(handles) != 0 {
		t.Fatalf("expected native interactive results marked as already persisted to skip runtime-handle extraction, got %#v", handles)
	}
}
