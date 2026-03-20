package runtime_test

import (
	"context"
	"reflect"
	"testing"

	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

func TestDefaultContextAssemblerProducesMinimalExpectedShape(t *testing.T) {
	assembler := hruntime.DefaultContextAssembler{}
	state := session.State{
		SessionID:     "sess_1",
		Phase:         session.PhasePlan,
		CurrentStepID: "step_1",
		RetryCount:    2,
	}
	spec := task.Spec{
		TaskID:      "task_1",
		TaskType:    "demo",
		Goal:        "echo hello",
		Constraints: map[string]any{"workspace": "/tmp/demo"},
		Metadata:    map[string]any{"priority": "high"},
	}

	assembled, err := assembler.Assemble(context.Background(), state, spec)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}

	assembledMap := assembled.ToMap()
	expectedKeys := []string{"constraints", "metadata", "session", "task"}
	gotKeys := make([]string, 0, len(assembledMap))
	for key := range assembledMap {
		gotKeys = append(gotKeys, key)
	}
	if !reflect.DeepEqual(sortStrings(gotKeys), expectedKeys) {
		t.Fatalf("expected keys %#v, got %#v", expectedKeys, sortStrings(gotKeys))
	}

	taskMap, _ := assembledMap["task"].(map[string]any)
	if taskMap["task_id"] != "task_1" || taskMap["goal"] != "echo hello" {
		t.Fatalf("unexpected task section: %#v", taskMap)
	}
	sessionMap, _ := assembledMap["session"].(map[string]any)
	if sessionMap["session_id"] != "sess_1" || sessionMap["current_step_id"] != "step_1" || sessionMap["retry_count"] != 2 {
		t.Fatalf("unexpected session section: %#v", sessionMap)
	}
}

func sortStrings(in []string) []string {
	out := append([]string(nil), in...)
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}
