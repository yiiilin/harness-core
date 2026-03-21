// Command planner-context shows how to build a layered planner-facing context package.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

type LayeredContextAssembler struct {
	MaxPreviewBytes int
}

// Assemble returns a planner-friendly context package and adds compact previews in Extras.
func (a LayeredContextAssembler) Assemble(_ context.Context, state session.State, spec task.Spec) (hruntime.ContextPackage, error) {
	limit := a.MaxPreviewBytes
	if limit <= 0 {
		limit = 48
	}
	return hruntime.ContextPackage{
		Task: hruntime.ContextTask{
			TaskID:   spec.TaskID,
			TaskType: spec.TaskType,
			Goal:     spec.Goal,
		},
		Session: hruntime.ContextSession{
			SessionID:      state.SessionID,
			Phase:          state.Phase,
			CurrentStepID:  state.CurrentStepID,
			RetryCount:     state.RetryCount,
			ExecutionState: state.ExecutionState,
		},
		Constraints: spec.Constraints,
		Metadata:    spec.Metadata,
		Derived: map[string]any{
			"goal_word_count": len(strings.Fields(spec.Goal)),
			"has_constraints": len(spec.Constraints) > 0,
			"has_metadata":    len(spec.Metadata) > 0,
		},
		Extras: map[string]any{
			"compaction": map[string]any{
				"limit_bytes":         limit,
				"metadata_preview":    compactMap(spec.Metadata, limit),
				"constraints_preview": compactMap(spec.Constraints, limit),
			},
		},
	}, nil
}

func compactMap(input map[string]any, limit int) map[string]any {
	out := map[string]any{}
	for key, value := range input {
		switch v := value.(type) {
		case string:
			out[key] = compactString(v, limit)
		default:
			out[key] = value
		}
	}
	return out
}

func compactString(value string, limit int) map[string]any {
	if limit <= 0 || len(value) <= limit {
		return map[string]any{
			"text":           value,
			"truncated":      false,
			"original_bytes": len(value),
		}
	}
	return map[string]any{
		"text":           value[:limit],
		"truncated":      true,
		"original_bytes": len(value),
	}
}

func main() {
	assembler := LayeredContextAssembler{MaxPreviewBytes: 24}
	assembled, err := assembler.Assemble(context.Background(), session.State{
		SessionID:     "sess_demo",
		Phase:         session.PhasePlan,
		CurrentStepID: "step_prepare",
		RetryCount:    1,
	}, task.Spec{
		TaskID:      "task_demo",
		TaskType:    "demo",
		Goal:        "summarize the latest shell output and decide the next step",
		Constraints: map[string]any{"workspace": "/tmp/harness"},
		Metadata:    map[string]any{"notes": "this is a deliberately long note that will be compacted"},
	})
	if err != nil {
		panic(err)
	}
	body, _ := json.MarshalIndent(assembled.ToMap(), "", "  ")
	fmt.Println("assembled context package:")
	fmt.Println(string(body))
}
