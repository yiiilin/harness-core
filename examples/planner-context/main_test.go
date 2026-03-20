package main

import (
	"context"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

func TestLayeredContextAssemblerBuildsExpectedSections(t *testing.T) {
	assembler := LayeredContextAssembler{MaxPreviewBytes: 10}
	assembled, err := assembler.Assemble(context.Background(), session.State{
		SessionID:     "sess_1",
		Phase:         session.PhasePlan,
		CurrentStepID: "step_1",
		RetryCount:    1,
	}, task.Spec{
		TaskID:      "task_1",
		TaskType:    "demo",
		Goal:        "plan the next step",
		Constraints: map[string]any{"workspace": "/tmp/work"},
		Metadata:    map[string]any{"notes": "abcdefghijklmnopqrstuvwxyz"},
	})
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}

	assembledMap := assembled.ToMap()
	for _, key := range []string{"task", "session", "derived", "compaction"} {
		if _, ok := assembledMap[key]; !ok {
			t.Fatalf("expected section %q in assembled context: %#v", key, assembledMap)
		}
	}

	compaction, _ := assembledMap["compaction"].(map[string]any)
	metadataPreview, _ := compaction["metadata_preview"].(map[string]any)
	notesPreview, _ := metadataPreview["notes"].(map[string]any)
	if truncated, _ := notesPreview["truncated"].(bool); !truncated {
		t.Fatalf("expected metadata notes to be truncated, got %#v", notesPreview)
	}
}
