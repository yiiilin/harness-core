package runtime_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/builtins"
	shellexec "github.com/yiiilin/harness-core/pkg/harness/executor/shell"
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type plannerCapture struct {
	seen []hruntime.ContextPackage
}

func (p *plannerCapture) PlanNext(_ context.Context, state session.State, _ task.Spec, assembled hruntime.ContextPackage) (plan.StepSpec, error) {
	p.seen = append(p.seen, assembled)
	if state.CurrentStepID != "" {
		return plan.StepSpec{}, errors.New("done")
	}
	return plan.StepSpec{
		StepID: "planned_step",
		Title:  "planned",
		Action: action.Spec{ToolName: "demo.long-output"},
	}, nil
}

func TestWithDefaultsSetsRuntimePolicyDefaultsAndPlannerProjectionRemainsExplicit(t *testing.T) {
	opts := hruntime.WithDefaults(hruntime.Options{})

	if opts.RuntimePolicy.Output.Defaults.Transport.MaxBytes <= 0 {
		t.Fatalf("expected positive transport max bytes default, got %#v", opts.RuntimePolicy)
	}
	if opts.RuntimePolicy.Output.Defaults.Inline.MaxChars <= 0 {
		t.Fatalf("expected positive inline max chars default, got %#v", opts.RuntimePolicy)
	}
	if opts.RuntimePolicy.Output.Defaults.Raw.RetentionMode != hruntime.RawRetentionBackendDefined {
		t.Fatalf("expected backend-defined raw retention by default, got %#v", opts.RuntimePolicy)
	}
	if opts.RuntimePolicy.Planner.Projection.Mode != "" {
		t.Fatalf("expected planner projection mode to stay explicit-by-config, got %#v", opts.RuntimePolicy.Planner)
	}
}

func TestCreatePlanFromPlannerRequiresExplicitPlannerProjectionPolicy(t *testing.T) {
	planner := &plannerCapture{}
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.long-output", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, longOutputHandler{stdout: "hello world"})

	rt := hruntime.New(hruntime.Options{
		Planner:   planner,
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "planner projection required", "planner projection policy must be explicit")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "plan one step"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	if _, _, err := rt.CreatePlanFromPlanner(context.Background(), attached.SessionID, "projection required", 1); !errors.Is(err, hruntime.ErrPlannerProjectionPolicyRequired) {
		t.Fatalf("expected explicit planner projection policy error, got %v", err)
	}
}

func TestProjectPlannerContextAppliesInlineProjectionWithoutMutatingRawContext(t *testing.T) {
	rt := hruntime.New(hruntime.Options{
		RuntimePolicy: hruntime.RuntimePolicy{
			Output: hruntime.OutputPolicy{
				Defaults: hruntime.OutputModePolicy{
					Transport: hruntime.TransportBudgetPolicy{MaxBytes: 4096},
					Inline:    hruntime.InlineBudgetPolicy{MaxChars: 12},
					Raw:       hruntime.RawResultPolicy{RetentionMode: hruntime.RawRetentionBackendDefined},
				},
			},
			Planner: hruntime.PlannerPolicy{
				Projection: hruntime.PlannerProjectionPolicy{Mode: hruntime.PlannerProjectionInline},
				Context:    hruntime.PlannerContextBudgetPolicy{MaxChars: 8},
			},
		},
	})

	sess := mustCreateSession(t, rt, "project planner context", "keep raw context separate from planner projection")
	tsk := mustCreateTask(t, rt, task.Spec{
		TaskType: "demo",
		Goal:     "project planner context",
		Metadata: map[string]any{"notes": "abcdefghijklmnopqrstuvwxyz"},
	})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	raw, _, state, spec, err := rt.AssembleContextForSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("assemble raw context: %v", err)
	}
	projected, err := rt.ProjectPlannerContext(context.Background(), raw, state, spec)
	if err != nil {
		t.Fatalf("project planner context: %v", err)
	}

	rawNotes, _ := raw.Metadata["notes"].(string)
	if rawNotes != "abcdefghijklmnopqrstuvwxyz" {
		t.Fatalf("expected raw context to remain untouched, got %#v", raw)
	}
	projectedNotes, _ := projected.Metadata["notes"].(string)
	if projectedNotes == "abcdefghijklmnopqrstuvwxyz" || len(projectedNotes) > 8 {
		t.Fatalf("expected projected planner context to obey inline budget, got %#v", projected)
	}
}

func TestProjectPlannerContextInlineProjectionIsDeterministicAndRuneSafe(t *testing.T) {
	rt := hruntime.New(hruntime.Options{
		RuntimePolicy: hruntime.RuntimePolicy{
			Planner: hruntime.PlannerPolicy{
				Projection: hruntime.PlannerProjectionPolicy{Mode: hruntime.PlannerProjectionInline},
				Context:    hruntime.PlannerContextBudgetPolicy{MaxChars: 3},
			},
		},
	})

	raw := hruntime.ContextPackage{
		Task: hruntime.ContextTask{TaskID: "task"},
		Metadata: map[string]any{
			"beta":  "再见",
			"alpha": "世界你好",
		},
	}

	var baseline hruntime.ContextPackage
	for i := 0; i < 20; i++ {
		projected, err := rt.ProjectPlannerContext(context.Background(), raw, session.State{}, task.Spec{})
		if err != nil {
			t.Fatalf("project planner context: %v", err)
		}
		alpha, _ := projected.Metadata["alpha"].(string)
		beta, _ := projected.Metadata["beta"].(string)
		if alpha != "世界你" {
			t.Fatalf("expected rune-safe truncation of alpha metadata, got %#v", projected.Metadata)
		}
		if beta != "" {
			t.Fatalf("expected deterministic map traversal to exhaust budget before beta, got %#v", projected.Metadata)
		}
		if !utf8.ValidString(alpha) || !utf8.ValidString(beta) {
			t.Fatalf("expected valid UTF-8 after inline projection, got alpha=%q beta=%q", alpha, beta)
		}
		if i == 0 {
			baseline = projected
			continue
		}
		if !reflect.DeepEqual(projected, baseline) {
			t.Fatalf("expected deterministic inline projection, baseline=%#v current=%#v", baseline, projected)
		}
	}
}

func TestProjectPlannerContextInlineProjectionUsesHeadTailPreview(t *testing.T) {
	rt := hruntime.New(hruntime.Options{
		RuntimePolicy: hruntime.RuntimePolicy{
			Planner: hruntime.PlannerPolicy{
				Projection: hruntime.PlannerProjectionPolicy{Mode: hruntime.PlannerProjectionInline},
				Context:    hruntime.PlannerContextBudgetPolicy{MaxChars: 13},
			},
		},
	})

	raw := hruntime.ContextPackage{
		Task: hruntime.ContextTask{TaskID: "task"},
		Metadata: map[string]any{
			"notes": "abcdefghijklmnopqrst",
		},
	}

	projected, err := rt.ProjectPlannerContext(context.Background(), raw, session.State{}, task.Spec{})
	if err != nil {
		t.Fatalf("project planner context: %v", err)
	}
	notes, _ := projected.Metadata["notes"].(string)
	assertHeadTailPreviewText(t, notes, 13, "abcd", "opqrst")
}

func TestProjectPlannerContextInlineProjectionUsesHeadTailPreviewForUTF8(t *testing.T) {
	rt := hruntime.New(hruntime.Options{
		RuntimePolicy: hruntime.RuntimePolicy{
			Planner: hruntime.PlannerPolicy{
				Projection: hruntime.PlannerProjectionPolicy{Mode: hruntime.PlannerProjectionInline},
				Context:    hruntime.PlannerContextBudgetPolicy{MaxChars: 7},
			},
		},
	})

	raw := hruntime.ContextPackage{
		Task: hruntime.ContextTask{TaskID: "task"},
		Metadata: map[string]any{
			"notes": "世界你好再见朋友",
		},
	}

	projected, err := rt.ProjectPlannerContext(context.Background(), raw, session.State{}, task.Spec{})
	if err != nil {
		t.Fatalf("project planner context: %v", err)
	}
	notes, _ := projected.Metadata["notes"].(string)
	assertHeadTailPreviewText(t, notes, 7, "世", "见朋友")
	if !utf8.ValidString(notes) {
		t.Fatalf("expected valid UTF-8 after head-tail projection, got %q", notes)
	}
}

func TestCreatePlanFromPlannerUsesProjectedContext(t *testing.T) {
	planner := &plannerCapture{}
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.long-output", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, longOutputHandler{stdout: "hello world"})

	rt := hruntime.New(hruntime.Options{
		Planner:   planner,
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
		RuntimePolicy: hruntime.RuntimePolicy{
			Output: hruntime.OutputPolicy{
				Defaults: hruntime.OutputModePolicy{
					Transport: hruntime.TransportBudgetPolicy{MaxBytes: 4096},
					Inline:    hruntime.InlineBudgetPolicy{MaxChars: 12},
					Raw:       hruntime.RawResultPolicy{RetentionMode: hruntime.RawRetentionBackendDefined},
				},
			},
			Planner: hruntime.PlannerPolicy{
				Projection: hruntime.PlannerProjectionPolicy{Mode: hruntime.PlannerProjectionInline},
				Context:    hruntime.PlannerContextBudgetPolicy{MaxChars: 8},
			},
		},
	})

	sess := mustCreateSession(t, rt, "planner projected context", "planner should consume projected context")
	tsk := mustCreateTask(t, rt, task.Spec{
		TaskType: "demo",
		Goal:     "planner projected context",
		Metadata: map[string]any{"notes": "abcdefghijklmnopqrstuvwxyz"},
	})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	if _, _, err := rt.CreatePlanFromPlanner(context.Background(), attached.SessionID, "projected context", 1); err != nil {
		t.Fatalf("create plan from planner: %v", err)
	}
	if len(planner.seen) == 0 {
		t.Fatalf("expected planner to receive one projected context package")
	}
	notes, _ := planner.seen[0].Metadata["notes"].(string)
	if notes == "abcdefghijklmnopqrstuvwxyz" || len(notes) > 8 {
		t.Fatalf("expected planner to see projected context, got %#v", planner.seen[0])
	}
}

func TestRunStepUsesStepToolThenRuntimeOutputPolicyPrecedence(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.long-output", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, longOutputHandler{stdout: "abcdefghijkl"})
	tools.Register(tool.Definition{ToolName: "demo.default-output", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, longOutputHandler{stdout: "abcdefghijkl"})

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
		RuntimePolicy: hruntime.RuntimePolicy{
			Output: hruntime.OutputPolicy{
				Defaults: hruntime.OutputModePolicy{
					Transport: hruntime.TransportBudgetPolicy{MaxBytes: 4096},
					Inline:    hruntime.InlineBudgetPolicy{MaxChars: 10},
					Raw:       hruntime.RawResultPolicy{RetentionMode: hruntime.RawRetentionBackendDefined},
				},
				ToolOverrides: map[string]hruntime.OutputModePolicy{
					"demo.long-output": {
						Inline: hruntime.InlineBudgetPolicy{MaxChars: 6},
					},
				},
				StepOverrides: map[string]hruntime.OutputModePolicy{
					"step_specific": {
						Inline: hruntime.InlineBudgetPolicy{MaxChars: 4},
					},
				},
			},
		},
	})

	for _, tc := range []struct {
		stepID   string
		toolName string
		expect   string
	}{
		{stepID: "step_specific", toolName: "demo.long-output", expect: "abcd"},
		{stepID: "step_tool_only", toolName: "demo.long-output", expect: "a...kl"},
		{stepID: "step_default", toolName: "demo.default-output", expect: "ab...hijkl"},
	} {
		sess := mustCreateSession(t, rt, "output policy precedence", "step override should win over tool override and runtime default")
		tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "run precedence cases"})
		attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
		if err != nil {
			t.Fatalf("attach task %s: %v", tc.stepID, err)
		}
		pl, err := rt.CreatePlan(attached.SessionID, tc.stepID, []plan.StepSpec{{
			StepID: tc.stepID,
			Title:  tc.stepID,
			Action: action.Spec{ToolName: tc.toolName},
		}})
		if err != nil {
			t.Fatalf("create plan %s: %v", tc.stepID, err)
		}
		out, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0])
		if err != nil {
			t.Fatalf("run step %s: %v", tc.stepID, err)
		}
		stdout, _ := out.Execution.Action.Data["stdout"].(string)
		if stdout != tc.expect {
			t.Fatalf("expected %s to resolve inline policy %q, got %#v", tc.stepID, tc.expect, out.Execution.Action)
		}
	}
}

func TestRunStepAppliesHeadTailPreviewToInlineResultStrings(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(
		tool.Definition{ToolName: "demo.long-output", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true},
		longOutputHandler{stdout: "abcdefghijklmnopqrst"},
	)

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
		RuntimePolicy: hruntime.RuntimePolicy{
			Output: hruntime.OutputPolicy{
				Defaults: hruntime.OutputModePolicy{
					Transport: hruntime.TransportBudgetPolicy{MaxBytes: 4096},
					Inline:    hruntime.InlineBudgetPolicy{MaxChars: 13},
					Raw:       hruntime.RawResultPolicy{RetentionMode: hruntime.RawRetentionBackendDefined},
				},
			},
		},
	})

	sess := mustCreateSession(t, rt, "head-tail inline result", "inline preview should preserve both head and tail")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "head-tail inline result"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(attached.SessionID, "head-tail inline", []plan.StepSpec{{
		StepID: "step_head_tail_inline",
		Title:  "head-tail inline",
		Action: action.Spec{ToolName: "demo.long-output"},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	out, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}

	stdout, _ := out.Execution.Action.Data["stdout"].(string)
	assertHeadTailPreviewText(t, stdout, 13, "abcd", "opqrst")
}

func TestRunStepAppliesHeadTailPreviewToInlineMetaAndErrorStrings(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(
		tool.Definition{ToolName: "demo.structured-output", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true},
		structuredLongOutputHandler{
			stdout:  "ok",
			meta:    "abcdefghijklmnopqrst",
			message: "abcdefghijklmnopqrst",
		},
	)

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
		RuntimePolicy: hruntime.RuntimePolicy{
			Output: hruntime.OutputPolicy{
				Defaults: hruntime.OutputModePolicy{
					Transport: hruntime.TransportBudgetPolicy{MaxBytes: 4096},
					Inline:    hruntime.InlineBudgetPolicy{MaxChars: 13},
					Raw:       hruntime.RawResultPolicy{RetentionMode: hruntime.RawRetentionBackendDefined},
				},
			},
		},
	})

	sess := mustCreateSession(t, rt, "head-tail meta error", "inline preview should cover meta and error strings")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "head-tail meta and error"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(attached.SessionID, "head-tail meta error", []plan.StepSpec{{
		StepID: "step_head_tail_meta_error",
		Title:  "head-tail meta error",
		Action: action.Spec{ToolName: "demo.structured-output"},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	out, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}

	note, _ := out.Execution.Action.Meta["note"].(string)
	assertHeadTailPreviewText(t, note, 13, "abcd", "opqrst")
	if out.Execution.Action.Error == nil {
		t.Fatalf("expected action error to remain present, got %#v", out.Execution.Action)
	}
	assertHeadTailPreviewText(t, out.Execution.Action.Error.Message, 13, "abcd", "opqrst")
	if !utf8.ValidString(out.Execution.Action.Error.Message) {
		t.Fatalf("expected UTF-8 safe error preview, got %#v", out.Execution.Action.Error)
	}
}

func TestRunStepKeepsUTF8ValidWhenInlinePreviewTruncatesMetaAndErrorStrings(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(
		tool.Definition{ToolName: "demo.structured-utf8", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true},
		structuredLongOutputHandler{
			stdout:  "世界你好再见朋友",
			meta:    "世界你好再见朋友",
			message: "世界你好再见朋友",
		},
	)

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
		RuntimePolicy: hruntime.RuntimePolicy{
			Output: hruntime.OutputPolicy{
				Defaults: hruntime.OutputModePolicy{
					Transport: hruntime.TransportBudgetPolicy{MaxBytes: 4096},
					Inline:    hruntime.InlineBudgetPolicy{MaxChars: 7},
					Raw:       hruntime.RawResultPolicy{RetentionMode: hruntime.RawRetentionBackendDefined},
				},
			},
		},
	})

	sess := mustCreateSession(t, rt, "utf8 inline trim", "inline preview must stay UTF-8 safe")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "utf8 inline trim"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(attached.SessionID, "utf8 inline trim", []plan.StepSpec{{
		StepID: "step_utf8_inline_trim",
		Title:  "utf8 inline trim",
		Action: action.Spec{ToolName: "demo.structured-utf8"},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	out, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}

	stdout, _ := out.Execution.Action.Data["stdout"].(string)
	assertHeadTailPreviewText(t, stdout, 7, "世", "见朋友")
	if !utf8.ValidString(stdout) {
		t.Fatalf("expected UTF-8 safe data preview, got %q", stdout)
	}

	note, _ := out.Execution.Action.Meta["note"].(string)
	assertHeadTailPreviewText(t, note, 7, "世", "见朋友")
	if !utf8.ValidString(note) {
		t.Fatalf("expected UTF-8 safe meta preview, got %q", note)
	}
	if out.Execution.Action.Error == nil {
		t.Fatalf("expected action error to remain present, got %#v", out.Execution.Action)
	}
	assertHeadTailPreviewText(t, out.Execution.Action.Error.Message, 7, "世", "见朋友")
	if !utf8.ValidString(out.Execution.Action.Error.Message) {
		t.Fatalf("expected UTF-8 safe error preview, got %#v", out.Execution.Action.Error)
	}
}

func TestRunStepExposesUnifiedResultWindowAndRawHandle(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(
		tool.Definition{ToolName: "demo.long-output", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true},
		longOutputHandler{stdout: "line-1\nline-2\nline-3\nline-4\n"},
	)

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
		RuntimePolicy: hruntime.RuntimePolicy{
			Output: hruntime.OutputPolicy{
				Defaults: hruntime.OutputModePolicy{
					Transport: hruntime.TransportBudgetPolicy{MaxBytes: 5},
					Inline:    hruntime.InlineBudgetPolicy{MaxChars: 8},
					Raw:       hruntime.RawResultPolicy{RetentionMode: hruntime.RawRetentionBackendDefined},
				},
			},
		},
	})

	sess := mustCreateSession(t, rt, "unified result window", "expose one raw handle/window contract")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "unify runtime output contract"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(attached.SessionID, "unified window", []plan.StepSpec{{
		StepID: "step_unified_window",
		Title:  "unified window",
		Action: action.Spec{ToolName: "demo.long-output"},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	out, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	if out.Execution.Action.Window == nil || !out.Execution.Action.Window.Truncated {
		t.Fatalf("expected unified result window metadata, got %#v", out.Execution.Action)
	}
	if out.Execution.Action.RawHandle == nil || out.Execution.Action.RawHandle.Ref == "" || !out.Execution.Action.RawHandle.Reread {
		t.Fatalf("expected stable raw handle contract, got %#v", out.Execution.Action)
	}

	byteWindow, err := rt.ReadArtifact(out.Execution.Action.RawHandle.Ref, hruntime.ArtifactReadRequest{
		Path:   "data.stdout",
		Offset: 7,
	})
	if err != nil {
		t.Fatalf("read artifact byte window: %v", err)
	}
	if byteWindow.Window == nil || !byteWindow.Window.HasMore || byteWindow.Window.NextOffset <= 7 {
		t.Fatalf("expected artifact reread to reuse unified window contract, got %#v", byteWindow)
	}
	if byteWindow.RawHandle == nil || byteWindow.RawHandle.Ref != out.Execution.Action.RawHandle.Ref {
		t.Fatalf("expected artifact reread to reuse unified raw handle contract, got %#v", byteWindow)
	}

	defaultWindow, err := rt.ReadArtifact(out.Execution.Action.RawHandle.Ref, hruntime.ArtifactReadRequest{
		MaxBytes: 24,
	})
	if err != nil {
		t.Fatalf("read artifact default raw window: %v", err)
	}
	if defaultWindow.Data == "" || defaultWindow.Window == nil || !defaultWindow.Window.HasMore {
		t.Fatalf("expected raw handle reread without schema path to expose a default payload window, got %#v", defaultWindow)
	}
	alignedWindow, err := rt.ReadArtifact(out.Execution.Action.RawHandle.Ref, hruntime.ArtifactReadRequest{
		MaxBytes: int(out.Execution.Action.Window.NextOffset),
	})
	if err != nil {
		t.Fatalf("read artifact aligned preview window: %v", err)
	}
	if alignedWindow.Window == nil || alignedWindow.Window.ReturnedBytes != int(out.Execution.Action.Window.NextOffset) {
		t.Fatalf("expected action window offset to align with default raw reread stream, got action=%#v artifact=%#v", out.Execution.Action.Window, alignedWindow)
	}
}

type structuredLongOutputHandler struct {
	stdout  string
	meta    string
	message string
}

func (h structuredLongOutputHandler) Invoke(_ context.Context, _ map[string]any) (action.Result, error) {
	return action.Result{
		OK: false,
		Data: map[string]any{
			"stdout": h.stdout,
		},
		Meta: map[string]any{
			"note": h.meta,
		},
		Error: &action.Error{
			Code:    "LONG_MESSAGE",
			Message: h.message,
		},
	}, nil
}

func assertHeadTailPreviewText(t *testing.T, got string, limit int, head string, tail string) {
	t.Helper()
	if !strings.HasPrefix(got, head) {
		t.Fatalf("expected preview head %q, got %q", head, got)
	}
	if !strings.HasSuffix(got, tail) {
		t.Fatalf("expected preview tail %q, got %q", tail, got)
	}
	if !strings.Contains(got, "...") {
		t.Fatalf("expected middle truncation marker, got %q", got)
	}
	if utf8.RuneCountInString(got) > limit {
		t.Fatalf("expected preview to stay within limit %d, got %q", limit, got)
	}
}

func TestRunStepAppliesRuntimeTransportBudgetToPipeExecution(t *testing.T) {
	opts := hruntime.Options{
		RuntimePolicy: hruntime.RuntimePolicy{
			Output: hruntime.OutputPolicy{
				Defaults: hruntime.OutputModePolicy{
					Transport: hruntime.TransportBudgetPolicy{MaxBytes: 5},
					Inline:    hruntime.InlineBudgetPolicy{MaxChars: 64},
					Raw:       hruntime.RawResultPolicy{RetentionMode: hruntime.RawRetentionBackendDefined},
				},
			},
		},
		Policy: permission.DefaultEvaluator{},
	}
	builtins.Register(&opts)
	rt := hruntime.New(opts)

	sess := mustCreateSession(t, rt, "runtime transport budget", "runtime transport policy should bound pipe previews")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "apply runtime transport budget to shell pipe execution"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(attached.SessionID, "runtime transport budget", []plan.StepSpec{{
		StepID: "step_pipe_transport_budget",
		Title:  "pipe transport budget",
		Action: action.Spec{
			ToolName: "shell.exec",
			Args: map[string]any{
				"mode":       "pipe",
				"command":    "printf 'hello world'",
				"timeout_ms": 5000,
			},
		},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	out, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	stdout, _ := out.Execution.Action.Data["stdout"].(string)
	if stdout != "h...d" {
		t.Fatalf("expected pipe preview to honor runtime transport budget, got %#v", out.Execution.Action)
	}
	if out.Execution.Action.Raw == nil {
		t.Fatalf("expected pipe preview to preserve recoverable raw result, got %#v", out.Execution.Action)
	}
	rawStdout, _ := out.Execution.Action.Raw.Data["stdout"].(string)
	if rawStdout != "hello world" {
		t.Fatalf("expected raw pipe output to remain intact behind transport preview, got %#v", out.Execution.Action.Raw)
	}
}

func TestRunStepAppliesRuntimeTransportBudgetToDirectPipeExecutor(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		shellexec.PipeExecutor{},
	)

	rt := hruntime.New(hruntime.Options{
		Tools:  tools,
		Policy: permission.DefaultEvaluator{},
		RuntimePolicy: hruntime.RuntimePolicy{
			Output: hruntime.OutputPolicy{
				Defaults: hruntime.OutputModePolicy{
					Transport: hruntime.TransportBudgetPolicy{MaxBytes: 5},
					Inline:    hruntime.InlineBudgetPolicy{MaxChars: 64},
					Raw:       hruntime.RawResultPolicy{RetentionMode: hruntime.RawRetentionBackendDefined},
				},
			},
		},
	})

	sess := mustCreateSession(t, rt, "direct pipe transport budget", "runtime transport policy should also reach direct pipe executors")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "apply runtime transport budget to direct pipe executor"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(attached.SessionID, "direct pipe transport budget", []plan.StepSpec{{
		StepID: "step_direct_pipe_transport_budget",
		Title:  "direct pipe transport budget",
		Action: action.Spec{
			ToolName: "shell.exec",
			Args: map[string]any{
				"mode":       "pipe",
				"command":    "printf 'hello world'",
				"timeout_ms": 5000,
			},
		},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	out, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	stdout, _ := out.Execution.Action.Data["stdout"].(string)
	if stdout != "h...d" {
		t.Fatalf("expected direct pipe executor preview to honor runtime transport budget, got %#v", out.Execution.Action)
	}
	if out.Execution.Action.Raw == nil {
		t.Fatalf("expected direct pipe executor preview to preserve recoverable raw result, got %#v", out.Execution.Action)
	}
	rawStdout, _ := out.Execution.Action.Raw.Data["stdout"].(string)
	if rawStdout != "hello world" {
		t.Fatalf("expected raw direct pipe output to remain intact, got %#v", out.Execution.Action.Raw)
	}
}
