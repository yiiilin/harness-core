package evals_test

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness"
	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

func TestWorkflowEvalExecutionProgramFanoutDataflow(t *testing.T) {
	ctx := context.Background()
	rt := newExecutionModelRuntime()
	sessionID := seedExecutionModelSession(t, rt, "program-eval", "preplanned fanout graph")

	out, err := rt.RunProgram(ctx, sessionID, execution.Program{
		ProgramID: "eval_program_graph",
		Nodes: []execution.ProgramNode{
			{
				NodeID: "prepare",
				Action: action.Spec{ToolName: "eval.prepare"},
			},
			{
				NodeID:    "artifact",
				DependsOn: []string{"prepare"},
				Action:    action.Spec{ToolName: "eval.inspect_artifact"},
				InputBinds: []execution.ProgramInputBinding{{
					Name: "artifact",
					Kind: execution.ProgramInputBindingOutputRef,
					Ref:  &execution.OutputRef{Kind: execution.OutputRefArtifact, StepID: "prepare"},
				}},
			},
			{
				NodeID:      "dispatch",
				DependsOn:   []string{"prepare"},
				Action:      action.Spec{ToolName: "eval.dispatch"},
				VerifyScope: execution.VerificationScopeAggregate,
				Verify: &verify.Spec{
					Mode: verify.ModeAll,
					Checks: []verify.Check{
						{Kind: "value_equals", Args: map[string]any{"path": "result.data.status", "expected": string(execution.AggregateStatusCompleted)}},
					},
				},
				Targeting: &execution.TargetSelection{
					Mode: execution.TargetSelectionFanoutExplicit,
					Targets: []execution.Target{
						{TargetID: "alpha", Kind: "host"},
						{TargetID: "beta", Kind: "host"},
					},
				},
				InputBinds: []execution.ProgramInputBinding{{
					Name: "message",
					Kind: execution.ProgramInputBindingOutputRef,
					Ref: &execution.OutputRef{
						Kind:   execution.OutputRefStructured,
						StepID: "prepare",
						Path:   "payload.message",
					},
				}},
			},
		},
	})
	if err != nil {
		t.Fatalf("run program: %v", err)
	}
	if out.Session.Phase != harness.SessionPhase("complete") {
		t.Fatalf("expected session complete after program execution, got %#v", out.Session)
	}
	if len(out.Aggregates) != 1 || out.Aggregates[0].Status != execution.AggregateStatusCompleted || out.Aggregates[0].Completed != 2 {
		t.Fatalf("expected one completed aggregate summary, got %#v", out.Aggregates)
	}

	actions, err := rt.ListActions(sessionID)
	if err != nil {
		t.Fatalf("list actions: %v", err)
	}
	dispatchOutputs := []string{}
	artifactSeen := false
	for _, record := range actions {
		switch record.ToolName {
		case "eval.dispatch":
			stdout, _ := record.Result.Data["stdout"].(string)
			dispatchOutputs = append(dispatchOutputs, stdout)
		case "eval.inspect_artifact":
			artifactID, _ := record.Result.Data["artifact_id"].(string)
			artifactKind, _ := record.Result.Data["artifact_kind"].(string)
			artifactSeen = artifactID != "" && artifactKind == "action_result"
		}
	}
	slices.Sort(dispatchOutputs)
	if !artifactSeen {
		t.Fatalf("expected artifact-ref consumer to observe an action_result artifact, got %#v", actions)
	}
	if !slices.Equal(dispatchOutputs, []string{"alpha:fanout-eval", "beta:fanout-eval"}) {
		t.Fatalf("unexpected dispatch outputs: %#v", dispatchOutputs)
	}

	projection, err := harness.NewReplayReader(rt).SessionProjection(sessionID)
	if err != nil {
		t.Fatalf("session projection: %v", err)
	}
	targetSliceCycles := 0
	for _, cycle := range projection.Cycles {
		if len(cycle.TargetSlices) > 0 {
			targetSliceCycles++
		}
	}
	if targetSliceCycles != 2 {
		t.Fatalf("expected target-sliced replay cycles for fanout steps, got %#v", projection.Cycles)
	}
}

func TestWorkflowEvalInteractiveRuntimeProjection(t *testing.T) {
	ctx := context.Background()
	rt := newInteractiveEvalRuntime()
	sessionID := seedInteractiveEvalSession(t, rt)

	out, err := rt.RunSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("run session: %v", err)
	}
	if out.Session.Phase != harness.SessionPhase("complete") {
		t.Fatalf("expected completed session, got %#v", out.Session)
	}

	handles, err := rt.ListRuntimeHandles(sessionID)
	if err != nil {
		t.Fatalf("list runtime handles: %v", err)
	}
	if len(handles) != 1 {
		t.Fatalf("expected one persisted runtime handle, got %#v", handles)
	}
	exitCode := 0
	interactive, err := rt.UpdateInteractiveRuntime(ctx, handles[0].HandleID, harness.InteractiveRuntimeUpdate{
		Observation: &harness.ExecutionInteractiveObservation{
			NextOffset:   64,
			Closed:       false,
			ExitCode:     &exitCode,
			Status:       "active",
			StatusReason: "remote session active",
		},
		LastOperation: &harness.ExecutionInteractiveOperation{
			Kind:   harness.ExecutionInteractiveOperationView,
			At:     1234,
			Offset: 64,
		},
	})
	if err != nil {
		t.Fatalf("update interactive runtime: %v", err)
	}
	if !interactive.Capabilities.View || !interactive.Capabilities.Write || interactive.Observation.NextOffset != 64 {
		t.Fatalf("unexpected interactive projection after update: %#v", interactive)
	}

	projection, err := harness.NewReplayReader(rt).SessionProjection(sessionID)
	if err != nil {
		t.Fatalf("session projection: %v", err)
	}
	foundInteractive := false
	for _, cycle := range projection.Cycles {
		if len(cycle.InteractiveRuntimes) > 0 {
			foundInteractive = true
		}
	}
	if !foundInteractive {
		t.Fatalf("expected replay projection to expose derived interactive runtimes, got %#v", projection)
	}
}

func newExecutionModelRuntime() *harness.Service {
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	verify.RegisterBuiltins(verifiers)

	tools.Register(tool.Definition{ToolName: "eval.prepare", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, evalToolFunc(func(_ context.Context, _ map[string]any) (action.Result, error) {
		return action.Result{OK: true, Data: map[string]any{"payload": map[string]any{"message": "fanout-eval"}}}, nil
	}))
	tools.Register(tool.Definition{ToolName: "eval.inspect_artifact", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, evalToolFunc(func(_ context.Context, args map[string]any) (action.Result, error) {
		ref := evalArtifactRef(args["artifact"])
		return action.Result{OK: true, Data: map[string]any{"artifact_id": ref.ArtifactID, "artifact_kind": ref.Kind}}, nil
	}))
	tools.Register(tool.Definition{ToolName: "eval.dispatch", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, evalToolFunc(func(_ context.Context, args map[string]any) (action.Result, error) {
		message, _ := args["message"].(string)
		targetID, _ := evalTarget(args[execution.TargetArgKey])
		return action.Result{OK: true, Data: map[string]any{"stdout": fmt.Sprintf("%s:%s", targetID, message)}}, nil
	}))

	return harness.New(harness.Options{Tools: tools, Verifiers: verifiers})
}

func newInteractiveEvalRuntime() *harness.Service {
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	verify.RegisterBuiltins(verifiers)

	tools.Register(tool.Definition{ToolName: "eval.interactive", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, evalToolFunc(func(_ context.Context, _ map[string]any) (action.Result, error) {
		return action.Result{
			OK: true,
			Data: map[string]any{
				"runtime_handle": execution.RuntimeHandle{
					HandleID: "hdl_eval_interactive",
					Kind:     "pty",
					Status:   execution.RuntimeHandleActive,
					Metadata: map[string]any{
						execution.InteractiveMetadataKeyEnabled:       true,
						execution.InteractiveMetadataKeySupportsView:  true,
						execution.InteractiveMetadataKeySupportsWrite: true,
						execution.InteractiveMetadataKeySupportsClose: true,
						execution.InteractiveMetadataKeyStatus:        "active",
						execution.InteractiveMetadataKeyNextOffset:    int64(0),
					},
				},
			},
		}, nil
	}))

	return harness.New(harness.Options{Tools: tools, Verifiers: verifiers})
}

func seedExecutionModelSession(t *testing.T, rt *harness.Service, title, goal string) string {
	t.Helper()
	sess, err := rt.CreateSession(title, goal)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	tsk, err := rt.CreateTask(task.Spec{TaskType: "eval", Goal: goal})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	sess, err = rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	return sess.SessionID
}

func seedInteractiveEvalSession(t *testing.T, rt *harness.Service) string {
	t.Helper()
	sess, err := rt.CreateSession("interactive-eval", "project interactive runtime state")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	tsk, err := rt.CreateTask(task.Spec{TaskType: "eval", Goal: "persist an interactive runtime handle"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	sess, err = rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	if _, err := rt.CreatePlan(sess.SessionID, "interactive-eval", []plan.StepSpec{{
		StepID: "step_interactive_eval",
		Title:  "persist interactive runtime handle",
		Action: action.Spec{ToolName: "eval.interactive"},
	}}); err != nil {
		t.Fatalf("create plan: %v", err)
	}
	return sess.SessionID
}

type evalToolFunc func(context.Context, map[string]any) (action.Result, error)

func (f evalToolFunc) Invoke(ctx context.Context, args map[string]any) (action.Result, error) {
	return f(ctx, args)
}

func evalArtifactRef(raw any) execution.ArtifactRef {
	switch value := raw.(type) {
	case execution.ArtifactRef:
		return value
	case map[string]any:
		return execution.ArtifactRef{
			ArtifactID: stringValue(value["artifact_id"]),
			Kind:       stringValue(value["kind"]),
		}
	default:
		return execution.ArtifactRef{}
	}
}

func evalTarget(raw any) (string, string) {
	value, ok := raw.(map[string]any)
	if !ok {
		return "", ""
	}
	return stringValue(value[execution.TargetMetadataKeyID]), stringValue(value[execution.TargetMetadataKeyName])
}

func stringValue(raw any) string {
	text, _ := raw.(string)
	return text
}
