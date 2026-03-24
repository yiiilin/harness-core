// Command program-graph demonstrates the public preplanned execution-program
// contracts with native dataflow, artifact refs, explicit fan-out, aggregate
// verification, and replay projection.
package main

import (
	"context"
	"fmt"
	"slices"

	"github.com/yiiilin/harness-core/pkg/harness"
	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type DemoResult struct {
	SessionID           string
	Phase               harness.SessionPhase
	Aggregate           harness.ExecutionAggregateResult
	ArtifactRef         execution.ArtifactRef
	DispatchOutputs     []string
	Projection          harness.ReplaySessionProjection
	TargetSliceCycleIDs []string
}

func main() {
	result, err := RunProgramGraphDemo(context.Background())
	if err != nil {
		panic(err)
	}

	fmt.Printf("session: %s\n", result.SessionID)
	fmt.Printf("phase: %s\n", result.Phase)
	fmt.Printf("aggregate: %s completed=%d failed=%d\n", result.Aggregate.Status, result.Aggregate.Completed, result.Aggregate.Failed)
	fmt.Printf("artifact_ref: %s (%s)\n", result.ArtifactRef.ArtifactID, result.ArtifactRef.Kind)
	fmt.Printf("dispatch_outputs: %v\n", result.DispatchOutputs)
	fmt.Printf("projection_cycles: %d target_slice_cycles=%d\n", len(result.Projection.Cycles), len(result.TargetSliceCycleIDs))
}

func RunProgramGraphDemo(ctx context.Context) (DemoResult, error) {
	rt := newProgramGraphRuntime()
	sess, err := rt.CreateSession("program-graph", "run a preplanned execution program")
	if err != nil {
		return DemoResult{}, err
	}
	tsk, err := rt.CreateTask(task.Spec{TaskType: "demo", Goal: "demonstrate public execution program contracts"})
	if err != nil {
		return DemoResult{}, err
	}
	sess, err = rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		return DemoResult{}, err
	}

	out, err := rt.RunProgram(ctx, sess.SessionID, execution.Program{
		ProgramID: "program_graph_demo",
		Nodes: []execution.ProgramNode{
			{
				NodeID: "prepare",
				Title:  "prepare rollout payload",
				Action: action.Spec{ToolName: "demo.prepare"},
			},
			{
				NodeID:    "inspect-artifact",
				Title:     "consume artifact ref from prior step",
				DependsOn: []string{"prepare"},
				Action:    action.Spec{ToolName: "demo.inspect_artifact"},
				InputBinds: []execution.ProgramInputBinding{{
					Name: "artifact",
					Kind: execution.ProgramInputBindingOutputRef,
					Ref: &execution.OutputRef{
						Kind:   execution.OutputRefArtifact,
						StepID: "prepare",
					},
				}},
			},
			{
				NodeID:      "dispatch",
				Title:       "fan out to explicit execution targets",
				DependsOn:   []string{"prepare"},
				Action:      action.Spec{ToolName: "demo.dispatch"},
				VerifyScope: execution.VerificationScopeAggregate,
				Verify: &verify.Spec{
					Mode: verify.ModeAll,
					Checks: []verify.Check{
						{Kind: "value_equals", Args: map[string]any{"path": "result.data.status", "expected": string(execution.AggregateStatusCompleted)}},
						{Kind: "number_compare", Args: map[string]any{"path": "result.data.completed", "op": "gte", "expected": 2}},
					},
				},
				Targeting: &execution.TargetSelection{
					Mode: execution.TargetSelectionFanoutExplicit,
					Targets: []execution.Target{
						{TargetID: "alpha", Kind: "host", DisplayName: "alpha.internal"},
						{TargetID: "beta", Kind: "host", DisplayName: "beta.internal"},
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
		return DemoResult{}, err
	}

	actions, err := rt.ListActions(sess.SessionID)
	if err != nil {
		return DemoResult{}, err
	}
	var (
		artifactRef     execution.ArtifactRef
		dispatchOutputs []string
	)
	for _, record := range actions {
		switch record.ToolName {
		case "demo.inspect_artifact":
			artifactRef = artifactRefFromAction(record.Result)
		case "demo.dispatch":
			if stdout, _ := record.Result.Data["stdout"].(string); stdout != "" {
				dispatchOutputs = append(dispatchOutputs, stdout)
			}
		}
	}
	slices.Sort(dispatchOutputs)

	projection, err := harness.NewReplayReader(rt).SessionProjection(sess.SessionID)
	if err != nil {
		return DemoResult{}, err
	}
	targetSliceCycleIDs := []string{}
	for _, cycle := range projection.Cycles {
		if len(cycle.TargetSlices) > 0 {
			targetSliceCycleIDs = append(targetSliceCycleIDs, cycle.Cycle.CycleID)
		}
	}

	var aggregate harness.ExecutionAggregateResult
	if len(out.Aggregates) > 0 {
		aggregate = out.Aggregates[0]
	}
	return DemoResult{
		SessionID:           sess.SessionID,
		Phase:               out.Session.Phase,
		Aggregate:           aggregate,
		ArtifactRef:         artifactRef,
		DispatchOutputs:     dispatchOutputs,
		Projection:          projection,
		TargetSliceCycleIDs: targetSliceCycleIDs,
	}, nil
}

func newProgramGraphRuntime() *harness.Service {
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	verify.RegisterBuiltins(verifiers)

	tools.Register(tool.Definition{
		ToolName:       "demo.prepare",
		Version:        "v1",
		CapabilityType: "executor",
		RiskLevel:      tool.RiskLow,
		Enabled:        true,
	}, toolHandlerFunc(func(_ context.Context, _ map[string]any) (action.Result, error) {
		return action.Result{
			OK: true,
			Data: map[string]any{
				"payload": map[string]any{
					"message": "ship v1.0.1",
				},
			},
		}, nil
	}))
	tools.Register(tool.Definition{
		ToolName:       "demo.inspect_artifact",
		Version:        "v1",
		CapabilityType: "executor",
		RiskLevel:      tool.RiskLow,
		Enabled:        true,
	}, toolHandlerFunc(func(_ context.Context, args map[string]any) (action.Result, error) {
		ref := artifactRefFromArgs(args["artifact"])
		return action.Result{
			OK: true,
			Data: map[string]any{
				"artifact_id":   ref.ArtifactID,
				"artifact_kind": ref.Kind,
			},
		}, nil
	}))
	tools.Register(tool.Definition{
		ToolName:       "demo.dispatch",
		Version:        "v1",
		CapabilityType: "executor",
		RiskLevel:      tool.RiskLow,
		Enabled:        true,
	}, toolHandlerFunc(func(_ context.Context, args map[string]any) (action.Result, error) {
		message, _ := args["message"].(string)
		targetID, targetName := targetFromArgs(args[execution.TargetArgKey])
		return action.Result{
			OK: true,
			Data: map[string]any{
				"stdout":    fmt.Sprintf("%s:%s", targetID, message),
				"target_id": targetID,
				"target":    targetName,
			},
		}, nil
	}))

	return harness.New(harness.Options{
		Tools:     tools,
		Verifiers: verifiers,
	})
}

type toolHandlerFunc func(context.Context, map[string]any) (action.Result, error)

func (f toolHandlerFunc) Invoke(ctx context.Context, args map[string]any) (action.Result, error) {
	return f(ctx, args)
}

func artifactRefFromAction(result action.Result) execution.ArtifactRef {
	return artifactRefFromArgs(map[string]any{
		"artifact_id": result.Data["artifact_id"],
		"kind":        result.Data["artifact_kind"],
	})
}

func artifactRefFromArgs(raw any) execution.ArtifactRef {
	switch value := raw.(type) {
	case execution.ArtifactRef:
		return value
	case map[string]any:
		ref := execution.ArtifactRef{}
		if artifactID, _ := value["artifact_id"].(string); artifactID != "" {
			ref.ArtifactID = artifactID
		}
		if kind, _ := value["kind"].(string); kind != "" {
			ref.Kind = kind
		}
		return ref
	default:
		return execution.ArtifactRef{}
	}
}

func targetFromArgs(raw any) (string, string) {
	target, ok := raw.(map[string]any)
	if !ok {
		return "", ""
	}
	targetID, _ := target[execution.TargetMetadataKeyID].(string)
	targetName, _ := target[execution.TargetMetadataKeyName].(string)
	return targetID, targetName
}
