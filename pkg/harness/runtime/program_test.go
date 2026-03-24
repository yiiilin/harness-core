package runtime_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/replay"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

func TestCreatePlanFromProgramTopologicallyOrdersNodesAndMergesLiteralBindings(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.message", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, messageHandler{})

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "program", "create plan from program")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "run program"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	program := execution.Program{
		ProgramID: "prog_demo",
		Nodes: []execution.ProgramNode{
			{
				NodeID:    "node_apply",
				Title:     "apply",
				Action:    action.Spec{ToolName: "demo.message"},
				DependsOn: []string{"node_prepare"},
				InputBinds: []execution.ProgramInputBinding{
					{Name: "message", Kind: execution.ProgramInputBindingLiteral, Value: "apply"},
				},
			},
			{
				NodeID: "node_prepare",
				Title:  "prepare",
				Action: action.Spec{ToolName: "demo.message"},
				InputBinds: []execution.ProgramInputBinding{
					{Name: "message", Kind: execution.ProgramInputBindingLiteral, Value: "prepare"},
				},
			},
		},
	}

	created, err := rt.CreatePlanFromProgram(attached.SessionID, "program plan", program)
	if err != nil {
		t.Fatalf("create plan from program: %v", err)
	}
	if len(created.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(created.Steps))
	}
	if created.Steps[0].StepID != "prog_demo__node_prepare" || created.Steps[1].StepID != "prog_demo__node_apply" {
		t.Fatalf("unexpected step order: %#v", created.Steps)
	}
	if got, _ := created.Steps[0].Action.Args["message"].(string); got != "prepare" {
		t.Fatalf("unexpected literal binding in first step: %#v", created.Steps[0].Action.Args)
	}
	if got, _ := created.Steps[1].Action.Args["message"].(string); got != "apply" {
		t.Fatalf("unexpected literal binding in second step: %#v", created.Steps[1].Action.Args)
	}
}

func TestRunProgramExecutesProgramThroughSessionLoop(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.message", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, messageHandler{})

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "program run", "run program")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "run program"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	out, err := rt.RunProgram(context.Background(), attached.SessionID, execution.Program{
		ProgramID: "prog_run",
		Nodes: []execution.ProgramNode{
			{
				NodeID: "node_prepare",
				Action: action.Spec{ToolName: "demo.message"},
				InputBinds: []execution.ProgramInputBinding{
					{Name: "message", Kind: execution.ProgramInputBindingLiteral, Value: "prepare"},
				},
			},
			{
				NodeID:    "node_apply",
				Action:    action.Spec{ToolName: "demo.message"},
				DependsOn: []string{"node_prepare"},
				InputBinds: []execution.ProgramInputBinding{
					{Name: "message", Kind: execution.ProgramInputBindingLiteral, Value: "apply"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("run program: %v", err)
	}
	if out.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected completed session, got %#v", out.Session)
	}
	if len(out.Executions) != 2 {
		t.Fatalf("expected 2 step executions, got %d", len(out.Executions))
	}
	if out.Executions[0].Execution.Step.StepID != "prog_run__node_prepare" || out.Executions[1].Execution.Step.StepID != "prog_run__node_apply" {
		t.Fatalf("unexpected execution order: %#v", out.Executions)
	}

	actions := mustListActions(t, rt, attached.SessionID)
	if len(actions) != 2 {
		t.Fatalf("expected 2 actions, got %#v", actions)
	}
	gotMessages := map[string]bool{}
	for _, action := range actions {
		stdout, _ := action.Result.Data["stdout"].(string)
		gotMessages[stdout] = true
	}
	if !gotMessages["prepare"] || !gotMessages["apply"] {
		t.Fatalf("expected prepare/apply action outputs, got %#v", actions)
	}
}

func TestCreatePlanFromProgramExpandsExplicitFanoutTargets(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.target", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, targetAwareHandler{})

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "program fanout", "expand explicit fanout")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "expand explicit fanout"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	created, err := rt.CreatePlanFromProgram(attached.SessionID, "", execution.Program{
		ProgramID: "prog_fanout",
		Nodes: []execution.ProgramNode{{
			NodeID: "node_apply",
			Action: action.Spec{ToolName: "demo.target"},
			Targeting: &execution.TargetSelection{
				Mode: execution.TargetSelectionFanoutExplicit,
				Targets: []execution.Target{
					{TargetID: "t1", Kind: "host", DisplayName: "host-1"},
					{TargetID: "t2", Kind: "host", DisplayName: "host-2"},
				},
			},
		}},
	})
	if err != nil {
		t.Fatalf("create plan from program: %v", err)
	}
	if len(created.Steps) != 2 {
		t.Fatalf("expected 2 fanout steps, got %#v", created.Steps)
	}
	if created.Steps[0].StepID != "prog_fanout__node_apply__t1" || created.Steps[1].StepID != "prog_fanout__node_apply__t2" {
		t.Fatalf("unexpected fanout step ids: %#v", created.Steps)
	}
	if got, _ := created.Steps[0].Metadata[execution.TargetMetadataKeyID].(string); got != "t1" {
		t.Fatalf("missing target metadata on first step: %#v", created.Steps[0].Metadata)
	}
	if _, ok := created.Steps[0].Action.Args[execution.TargetArgKey].(map[string]any); !ok {
		t.Fatalf("expected injected execution target arg, got %#v", created.Steps[0].Action.Args)
	}
}

func TestRunProgramPersistsTargetScopedFactsAndReplaySlices(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.target", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, targetAwareHandler{})

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "program target facts", "persist target facts")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "persist target facts"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	out, err := rt.RunProgram(context.Background(), attached.SessionID, execution.Program{
		ProgramID: "prog_targets",
		Nodes: []execution.ProgramNode{{
			NodeID: "node_apply",
			Action: action.Spec{ToolName: "demo.target"},
			Targeting: &execution.TargetSelection{
				Mode: execution.TargetSelectionFanoutExplicit,
				Targets: []execution.Target{
					{TargetID: "t1", Kind: "host"},
					{TargetID: "t2", Kind: "host"},
				},
			},
		}},
	})
	if err != nil {
		t.Fatalf("run program: %v", err)
	}
	if len(out.Executions) != 2 {
		t.Fatalf("expected 2 fanout executions, got %#v", out.Executions)
	}

	attempts := mustListAttempts(t, rt, attached.SessionID)
	actions := mustListActions(t, rt, attached.SessionID)
	verifications := mustListVerifications(t, rt, attached.SessionID)
	artifacts := mustListArtifacts(t, rt, attached.SessionID)
	if len(attempts) != 2 || len(actions) != 2 || len(verifications) != 2 || len(artifacts) != 2 {
		t.Fatalf("expected target-scoped facts for each target, got attempts=%d actions=%d verifications=%d artifacts=%d", len(attempts), len(actions), len(verifications), len(artifacts))
	}
	for _, action := range actions {
		if _, ok := execution.TargetRefFromMetadata(action.Metadata); !ok {
			t.Fatalf("expected target metadata on action record, got %#v", action)
		}
	}

	projection, err := replay.NewReader(rt).SessionProjection(attached.SessionID)
	if err != nil {
		t.Fatalf("session projection: %v", err)
	}
	if len(projection.Cycles) != 2 {
		t.Fatalf("expected 2 target cycles, got %#v", projection.Cycles)
	}
	for _, cycle := range projection.Cycles {
		if len(cycle.TargetSlices) != 1 {
			t.Fatalf("expected one target slice per target cycle, got %#v", cycle.TargetSlices)
		}
		if cycle.TargetSlices[0].Target.TargetID == "" {
			t.Fatalf("expected populated target slice ref, got %#v", cycle.TargetSlices[0])
		}
	}
}

func TestRunProgramFanoutContinueAllowsPartialFailureAndReturnsAggregate(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(
		tool.Definition{ToolName: "demo.target-scripted", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true},
		&targetScriptedHandler{outputs: map[string][]string{
			"host-a": {"bad"},
			"host-b": {"ok"},
		}},
	)
	verifiers := verify.NewRegistry()
	verifiers.Register(verify.Definition{Kind: "output_contains", Description: "Verify output contains substring."}, verify.OutputContainsChecker{})

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verifiers,
		Policy:    permission.DefaultEvaluator{},
		LoopBudgets: func() hruntime.LoopBudgets {
			budgets := hruntime.DefaultLoopBudgets()
			budgets.MaxRetriesPerStep = 1
			return budgets
		}(),
	})

	sess := mustCreateSession(t, rt, "program partial failure", "continue after one target fails")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "continue after one target fails"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	out, err := rt.RunProgram(context.Background(), attached.SessionID, execution.Program{
		ProgramID: "prog_partial",
		Nodes: []execution.ProgramNode{{
			NodeID: "node_apply",
			Action: action.Spec{ToolName: "demo.target-scripted"},
			Verify: &verify.Spec{
				Mode: verify.ModeAll,
				Checks: []verify.Check{{
					Kind: "output_contains",
					Args: map[string]any{"text": "ok"},
				}},
			},
			OnFail: &plan.OnFailSpec{MaxRetries: 0},
			Targeting: &execution.TargetSelection{
				Mode:             execution.TargetSelectionFanoutExplicit,
				OnPartialFailure: execution.TargetFailureContinue,
				Targets: []execution.Target{
					{TargetID: "host-a", Kind: "host"},
					{TargetID: "host-b", Kind: "host"},
				},
			},
		}},
	})
	if err != nil {
		t.Fatalf("run program: %v", err)
	}
	if out.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected complete session, got %#v", out.Session)
	}
	if len(out.Executions) != 3 {
		t.Fatalf("expected one retry plus one successful peer execution, got %#v", out.Executions)
	}
	if len(out.Aggregates) != 1 {
		t.Fatalf("expected one aggregate result, got %#v", out.Aggregates)
	}
	if out.Aggregates[0].Status != execution.AggregateStatusPartialFailed || out.Aggregates[0].Completed != 1 || out.Aggregates[0].Failed != 1 {
		t.Fatalf("expected partial_failed aggregate, got %#v", out.Aggregates[0])
	}

	aggregates, err := rt.ListAggregateResults(attached.SessionID)
	if err != nil {
		t.Fatalf("list aggregate results: %v", err)
	}
	if len(aggregates) != 1 || aggregates[0].Status != execution.AggregateStatusPartialFailed {
		t.Fatalf("expected persisted partial aggregate view, got %#v", aggregates)
	}

	latestPlans, err := rt.ListPlans(attached.SessionID)
	if err != nil {
		t.Fatalf("list plans: %v", err)
	}
	if len(latestPlans) != 1 || latestPlans[0].Status != plan.StatusCompleted {
		t.Fatalf("expected completed plan despite tolerated target failure, got %#v", latestPlans)
	}
}

func TestRunProgramFanoutContinueFailsWhenAllTargetsFail(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(
		tool.Definition{ToolName: "demo.target-scripted", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true},
		&targetScriptedHandler{outputs: map[string][]string{
			"host-a": {"bad"},
			"host-b": {"still-bad"},
		}},
	)
	verifiers := verify.NewRegistry()
	verifiers.Register(verify.Definition{Kind: "output_contains", Description: "Verify output contains substring."}, verify.OutputContainsChecker{})

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verifiers,
		Policy:    permission.DefaultEvaluator{},
		LoopBudgets: func() hruntime.LoopBudgets {
			budgets := hruntime.DefaultLoopBudgets()
			budgets.MaxRetriesPerStep = 1
			return budgets
		}(),
	})

	sess := mustCreateSession(t, rt, "program aggregate fail", "fail when all targets fail")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "fail when all targets fail"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	out, err := rt.RunProgram(context.Background(), attached.SessionID, execution.Program{
		ProgramID: "prog_fail",
		Nodes: []execution.ProgramNode{{
			NodeID: "node_apply",
			Action: action.Spec{ToolName: "demo.target-scripted"},
			Verify: &verify.Spec{
				Mode: verify.ModeAll,
				Checks: []verify.Check{{
					Kind: "output_contains",
					Args: map[string]any{"text": "ok"},
				}},
			},
			OnFail: &plan.OnFailSpec{MaxRetries: 0},
			Targeting: &execution.TargetSelection{
				Mode:             execution.TargetSelectionFanoutExplicit,
				OnPartialFailure: execution.TargetFailureContinue,
				Targets: []execution.Target{
					{TargetID: "host-a", Kind: "host"},
					{TargetID: "host-b", Kind: "host"},
				},
			},
		}},
	})
	if err != nil {
		t.Fatalf("run program: %v", err)
	}
	if out.Session.Phase != session.PhaseFailed {
		t.Fatalf("expected failed session, got %#v", out.Session)
	}
	if len(out.Executions) != 4 {
		t.Fatalf("expected four executions with one retry per target, got %#v", out.Executions)
	}
	if len(out.Aggregates) != 1 || out.Aggregates[0].Status != execution.AggregateStatusFailed {
		t.Fatalf("expected failed aggregate, got %#v", out.Aggregates)
	}

	plans, err := rt.ListPlans(attached.SessionID)
	if err != nil {
		t.Fatalf("list plans: %v", err)
	}
	if len(plans) != 1 || plans[0].Status != plan.StatusFailed {
		t.Fatalf("expected failed plan, got %#v", plans)
	}
}

func TestRunProgramFanoutContinueRetriesEachTargetIndependently(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(
		tool.Definition{ToolName: "demo.target-scripted", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true},
		&targetScriptedHandler{outputs: map[string][]string{
			"host-a": {"bad", "ok"},
			"host-b": {"ok"},
		}},
	)
	verifiers := verify.NewRegistry()
	verifiers.Register(verify.Definition{Kind: "output_contains", Description: "Verify output contains substring."}, verify.OutputContainsChecker{})

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verifiers,
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "program retries", "retry targets independently")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "retry targets independently"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	out, err := rt.RunProgram(context.Background(), attached.SessionID, execution.Program{
		ProgramID: "prog_retry",
		Nodes: []execution.ProgramNode{{
			NodeID: "node_apply",
			Action: action.Spec{ToolName: "demo.target-scripted"},
			Verify: &verify.Spec{
				Mode: verify.ModeAll,
				Checks: []verify.Check{{
					Kind: "output_contains",
					Args: map[string]any{"text": "ok"},
				}},
			},
			OnFail: &plan.OnFailSpec{MaxRetries: 1},
			Targeting: &execution.TargetSelection{
				Mode:             execution.TargetSelectionFanoutExplicit,
				OnPartialFailure: execution.TargetFailureContinue,
				Targets: []execution.Target{
					{TargetID: "host-a", Kind: "host"},
					{TargetID: "host-b", Kind: "host"},
				},
			},
		}},
	})
	if err != nil {
		t.Fatalf("run program: %v", err)
	}
	if out.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected complete session after retry recovery, got %#v", out.Session)
	}
	if len(out.Executions) != 3 {
		t.Fatalf("expected three executions with one retry, got %#v", out.Executions)
	}
	if len(out.Aggregates) != 1 || out.Aggregates[0].Status != execution.AggregateStatusCompleted {
		t.Fatalf("expected completed aggregate after retry recovery, got %#v", out.Aggregates)
	}
	attempts := mustListAttempts(t, rt, attached.SessionID)
	if len(attempts) != 3 {
		t.Fatalf("expected three attempts including retry, got %#v", attempts)
	}
	hostARetries := 0
	for _, attempt := range attempts {
		ref, ok := execution.TargetRefFromMetadata(attempt.Metadata)
		if !ok {
			t.Fatalf("expected target-scoped attempt metadata, got %#v", attempt)
		}
		if ref.TargetID == "host-a" {
			hostARetries++
		}
	}
	if hostARetries != 2 {
		t.Fatalf("expected host-a to retry once, got %#v", attempts)
	}
}

func TestRunProgramResolvesStructuredOutputRefsIntoLaterStepArgs(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.structured", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, structuredProducerHandler{})
	tools.Register(tool.Definition{ToolName: "demo.echo-arg", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, echoArgHandler{})

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "program output refs", "resolve structured output refs")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "resolve structured output refs"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	out, err := rt.RunProgram(context.Background(), attached.SessionID, execution.Program{
		ProgramID: "prog_refs",
		Nodes: []execution.ProgramNode{
			{
				NodeID: "node_prepare",
				Action: action.Spec{ToolName: "demo.structured"},
			},
			{
				NodeID:    "node_apply",
				Action:    action.Spec{ToolName: "demo.echo-arg"},
				DependsOn: []string{"node_prepare"},
				InputBinds: []execution.ProgramInputBinding{{
					Name: "message",
					Kind: execution.ProgramInputBindingOutputRef,
					Ref: &execution.OutputRef{
						Kind:   execution.OutputRefStructured,
						StepID: "node_prepare",
						Path:   "payload.message",
					},
				}},
			},
		},
	})
	if err != nil {
		t.Fatalf("run program: %v", err)
	}
	if out.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected completed session, got %#v", out.Session)
	}

	actions := mustListActions(t, rt, attached.SessionID)
	if len(actions) != 2 {
		t.Fatalf("expected two actions, got %#v", actions)
	}
	var apply action.Result
	for _, record := range actions {
		nodeID, _ := record.Metadata[execution.ProgramMetadataKeyNodeID].(string)
		if nodeID == "node_apply" {
			apply = record.Result
			break
		}
	}
	if got, _ := apply.Data["stdout"].(string); got != "hello-from-prepare" {
		t.Fatalf("expected resolved structured output arg, got %#v", apply)
	}
}

func TestRunProgramResolvesTargetScopedOutputRefsPerTarget(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.target", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, targetAwareHandler{})
	tools.Register(tool.Definition{ToolName: "demo.echo-arg", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, echoArgHandler{})

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "program target refs", "resolve target scoped refs")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "resolve target scoped refs"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	out, err := rt.RunProgram(context.Background(), attached.SessionID, execution.Program{
		ProgramID: "prog_target_refs",
		Nodes: []execution.ProgramNode{
			{
				NodeID: "node_prepare",
				Action: action.Spec{ToolName: "demo.target"},
				Targeting: &execution.TargetSelection{
					Mode: execution.TargetSelectionFanoutExplicit,
					Targets: []execution.Target{
						{TargetID: "host-a", Kind: "host"},
						{TargetID: "host-b", Kind: "host"},
					},
				},
			},
			{
				NodeID:    "node_apply",
				Action:    action.Spec{ToolName: "demo.echo-arg"},
				DependsOn: []string{"node_prepare"},
				Targeting: &execution.TargetSelection{
					Mode: execution.TargetSelectionFanoutExplicit,
					Targets: []execution.Target{
						{TargetID: "host-a", Kind: "host"},
						{TargetID: "host-b", Kind: "host"},
					},
				},
				InputBinds: []execution.ProgramInputBinding{{
					Name: "message",
					Kind: execution.ProgramInputBindingOutputRef,
					Ref: &execution.OutputRef{
						Kind:   execution.OutputRefText,
						StepID: "node_prepare",
					},
				}},
			},
		},
	})
	if err != nil {
		t.Fatalf("run program: %v", err)
	}
	if out.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected completed session, got %#v", out.Session)
	}

	actions := mustListActions(t, rt, attached.SessionID)
	resolved := map[string]string{}
	for _, record := range actions {
		nodeID, _ := record.Metadata[execution.ProgramMetadataKeyNodeID].(string)
		if nodeID != "node_apply" {
			continue
		}
		target, ok := execution.TargetRefFromMetadata(record.Metadata)
		if !ok {
			t.Fatalf("expected target metadata on apply action, got %#v", record)
		}
		stdout, _ := record.Result.Data["stdout"].(string)
		resolved[target.TargetID] = stdout
	}
	if resolved["host-a"] != "host-a" || resolved["host-b"] != "host-b" {
		t.Fatalf("expected per-target output ref resolution, got %#v", resolved)
	}
}

func TestRunProgramResolvesArtifactRefsIntoLaterStepArgs(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.structured", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, structuredProducerHandler{})
	tools.Register(tool.Definition{ToolName: "demo.artifact-ref", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, artifactRefHandler{})

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "program artifact refs", "resolve artifact refs")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "resolve artifact refs"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	out, err := rt.RunProgram(context.Background(), attached.SessionID, execution.Program{
		ProgramID: "prog_artifact_refs",
		Nodes: []execution.ProgramNode{
			{
				NodeID: "node_prepare",
				Action: action.Spec{ToolName: "demo.structured"},
			},
			{
				NodeID:    "node_apply",
				Action:    action.Spec{ToolName: "demo.artifact-ref"},
				DependsOn: []string{"node_prepare"},
				InputBinds: []execution.ProgramInputBinding{{
					Name: "artifact",
					Kind: execution.ProgramInputBindingOutputRef,
					Ref: &execution.OutputRef{
						Kind:   execution.OutputRefArtifact,
						StepID: "node_prepare",
					},
				}},
			},
		},
	})
	if err != nil {
		t.Fatalf("run program: %v", err)
	}
	if out.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected completed session, got %#v", out.Session)
	}

	actions := mustListActions(t, rt, attached.SessionID)
	var apply action.Result
	for _, record := range actions {
		nodeID, _ := record.Metadata[execution.ProgramMetadataKeyNodeID].(string)
		if nodeID == "node_apply" {
			apply = record.Result
			break
		}
	}
	artifactID, _ := apply.Data["artifact_id"].(string)
	if artifactID == "" {
		t.Fatalf("expected resolved artifact ref in later action args, got %#v", apply)
	}
}

func TestRunProgramAggregateVerifyScopeEvaluatesFanoutSummary(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.target", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, targetAwareHandler{})
	verifiers := verify.NewRegistry()
	verify.RegisterBuiltins(verifiers)

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verifiers,
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "program aggregate verify", "verify aggregate fanout summary")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "verify aggregate fanout summary"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	out, err := rt.RunProgram(context.Background(), attached.SessionID, execution.Program{
		ProgramID: "prog_aggregate_verify",
		Nodes: []execution.ProgramNode{{
			NodeID:      "node_apply",
			Action:      action.Spec{ToolName: "demo.target"},
			VerifyScope: execution.VerificationScopeAggregate,
			Verify: &verify.Spec{
				Mode: verify.ModeAll,
				Checks: []verify.Check{
					{Kind: "value_equals", Args: map[string]any{"path": "result.data.status", "expected": string(execution.AggregateStatusCompleted)}},
					{Kind: "value_equals", Args: map[string]any{"path": "result.data.completed", "expected": 2}},
				},
			},
			OnFail: &plan.OnFailSpec{Strategy: "abort"},
			Targeting: &execution.TargetSelection{
				Mode: execution.TargetSelectionFanoutExplicit,
				Targets: []execution.Target{
					{TargetID: "host-a", Kind: "host"},
					{TargetID: "host-b", Kind: "host"},
				},
			},
		}},
	})
	if err != nil {
		t.Fatalf("run program: %v", err)
	}
	if out.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected completed session, got %#v", out.Session)
	}
	verifications := mustListVerifications(t, rt, attached.SessionID)
	if len(verifications) != 2 {
		t.Fatalf("expected one verification per target execution, got %#v", verifications)
	}
	if got, _ := verifications[0].Metadata["verification_scope"].(string); got != string(execution.VerificationScopeAggregate) {
		t.Fatalf("expected aggregate scope metadata, got %#v", verifications[0])
	}
	if verifications[0].Result.Reason != "aggregate pending" {
		t.Fatalf("expected first aggregate verify to wait for remaining targets, got %#v", verifications[0].Result)
	}
	if !verifications[1].Result.Success {
		t.Fatalf("expected resolved aggregate verification to succeed, got %#v", verifications[1].Result)
	}
}

func TestRunProgramAggregateVerifyScopeCanFailLogicalFanoutStep(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.target", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, targetAwareHandler{})
	verifiers := verify.NewRegistry()
	verify.RegisterBuiltins(verifiers)

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verifiers,
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "program aggregate verify fail", "fail aggregate fanout summary")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "fail aggregate fanout summary"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	out, err := rt.RunProgram(context.Background(), attached.SessionID, execution.Program{
		ProgramID: "prog_aggregate_verify_fail",
		Nodes: []execution.ProgramNode{{
			NodeID:      "node_apply",
			Action:      action.Spec{ToolName: "demo.target"},
			VerifyScope: execution.VerificationScopeAggregate,
			Verify: &verify.Spec{
				Mode: verify.ModeAll,
				Checks: []verify.Check{
					{Kind: "value_equals", Args: map[string]any{"path": "result.data.completed", "expected": 3}},
				},
			},
			OnFail: &plan.OnFailSpec{Strategy: "abort"},
			Targeting: &execution.TargetSelection{
				Mode: execution.TargetSelectionFanoutExplicit,
				Targets: []execution.Target{
					{TargetID: "host-a", Kind: "host"},
					{TargetID: "host-b", Kind: "host"},
				},
			},
		}},
	})
	if err != nil {
		t.Fatalf("run program: %v", err)
	}
	if out.Session.Phase != session.PhaseFailed {
		t.Fatalf("expected failed session from aggregate verification, got %#v", out.Session)
	}
	verifications := mustListVerifications(t, rt, attached.SessionID)
	if len(verifications) != 2 {
		t.Fatalf("expected one verification per target execution, got %#v", verifications)
	}
	if verifications[1].Result.Success {
		t.Fatalf("expected final aggregate verification to fail, got %#v", verifications[1].Result)
	}
}

func TestCreatePlanFromProgramRejectsUnsupportedTargetDiscovery(t *testing.T) {
	rt := hruntime.New(hruntime.Options{})
	sess := mustCreateSession(t, rt, "program reject", "reject unsupported bindings")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "reject unsupported bindings"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	_, err = rt.CreatePlanFromProgram(attached.SessionID, "", execution.Program{
		Nodes: []execution.ProgramNode{{
			NodeID:    "node_fanout",
			Action:    action.Spec{ToolName: "demo.message"},
			Targeting: &execution.TargetSelection{Mode: execution.TargetSelectionFanoutAll},
		}},
	})
	if !errors.Is(err, hruntime.ErrProgramTargetDiscoveryUnsupported) {
		t.Fatalf("expected target discovery unsupported, got %v", err)
	}
}

func TestCreatePlanFromProgramResolvesFanoutAllTargetsViaTargetResolver(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.target", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, targetAwareHandler{})

	rt := hruntime.New(hruntime.Options{
		Tools:          tools,
		Verifiers:      verify.NewRegistry(),
		Policy:         permission.DefaultEvaluator{},
		TargetResolver: staticTargetResolver{targetsByNode: map[string][]execution.Target{"node_fanout": {{TargetID: "resolved-a", Kind: "host"}, {TargetID: "resolved-b", Kind: "host"}}}},
	})

	sess := mustCreateSession(t, rt, "program resolve targets", "resolve fanout_all targets")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "resolve fanout_all targets"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	created, err := rt.CreatePlanFromProgram(attached.SessionID, "", execution.Program{
		ProgramID: "prog_resolved_fanout",
		Nodes: []execution.ProgramNode{{
			NodeID:    "node_fanout",
			Action:    action.Spec{ToolName: "demo.target"},
			Targeting: &execution.TargetSelection{Mode: execution.TargetSelectionFanoutAll},
		}},
	})
	if err != nil {
		t.Fatalf("create plan from program: %v", err)
	}
	if len(created.Steps) != 2 {
		t.Fatalf("expected 2 resolved target steps, got %#v", created.Steps)
	}
	first, ok := execution.TargetFromStep(created.Steps[0])
	if !ok || first.TargetID != "resolved-a" {
		t.Fatalf("expected first resolved target, got %#v", created.Steps[0])
	}
	second, ok := execution.TargetFromStep(created.Steps[1])
	if !ok || second.TargetID != "resolved-b" {
		t.Fatalf("expected second resolved target, got %#v", created.Steps[1])
	}
}

func TestRunProgramExecutesFanoutAllTargetsResolvedByTargetResolver(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.target", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, targetAwareHandler{})

	rt := hruntime.New(hruntime.Options{
		Tools:          tools,
		Verifiers:      verify.NewRegistry(),
		Policy:         permission.DefaultEvaluator{},
		TargetResolver: staticTargetResolver{targetsByNode: map[string][]execution.Target{"node_fanout": {{TargetID: "resolved-a", Kind: "host"}, {TargetID: "resolved-b", Kind: "host"}}}},
	})

	sess := mustCreateSession(t, rt, "program run resolved targets", "run fanout_all targets")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "run fanout_all targets"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	out, err := rt.RunProgram(context.Background(), attached.SessionID, execution.Program{
		ProgramID: "prog_run_resolved_fanout",
		Nodes: []execution.ProgramNode{{
			NodeID:    "node_fanout",
			Action:    action.Spec{ToolName: "demo.target"},
			Targeting: &execution.TargetSelection{Mode: execution.TargetSelectionFanoutAll},
		}},
	})
	if err != nil {
		t.Fatalf("run program: %v", err)
	}
	if len(out.Executions) != 2 {
		t.Fatalf("expected 2 executions, got %#v", out.Executions)
	}
	actions := mustListActions(t, rt, attached.SessionID)
	if len(actions) != 2 {
		t.Fatalf("expected 2 actions, got %#v", actions)
	}
	targets := map[string]bool{}
	for _, item := range actions {
		ref, ok := execution.TargetRefFromMetadata(item.Metadata)
		if !ok {
			t.Fatalf("expected target metadata on action, got %#v", item)
		}
		targets[ref.TargetID] = true
	}
	if !targets["resolved-a"] || !targets["resolved-b"] {
		t.Fatalf("expected resolved target actions, got %#v", actions)
	}
}

func TestRunProgramMaterializesInlineAttachmentToTempFile(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.read_file", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, fileReaderHandler{})

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "program materialize inline attachment", "materialize inline attachment")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "materialize inline attachment"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	out, err := rt.RunProgram(context.Background(), attached.SessionID, execution.Program{
		ProgramID: "prog_inline_attachment",
		Nodes: []execution.ProgramNode{{
			NodeID: "node_read",
			Action: action.Spec{ToolName: "demo.read_file"},
			InputBinds: []execution.ProgramInputBinding{{
				Name: "path",
				Kind: execution.ProgramInputBindingAttachment,
				Attachment: &execution.AttachmentInput{
					Kind:        execution.AttachmentInputText,
					Text:        "hello-inline-attachment",
					Materialize: execution.AttachmentMaterializeTempFile,
				},
			}},
		}},
	})
	if err != nil {
		t.Fatalf("run program: %v", err)
	}
	if len(out.Executions) != 1 {
		t.Fatalf("expected 1 execution, got %#v", out.Executions)
	}
	actions := mustListActions(t, rt, attached.SessionID)
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %#v", actions)
	}
	if got, _ := actions[0].Result.Data["stdout"].(string); got != "hello-inline-attachment" {
		t.Fatalf("expected materialized attachment payload, got %#v", actions[0].Result.Data)
	}
}

func TestRunProgramMaterializesArtifactAttachmentToTempFile(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.read_file", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, fileReaderHandler{})

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "program materialize artifact attachment", "materialize artifact attachment")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "materialize artifact attachment"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	artifact, err := rt.Artifacts.Create(execution.Artifact{
		ArtifactID: "art_inline_payload",
		SessionID:  attached.SessionID,
		TaskID:     attached.TaskID,
		Name:       "payload.txt",
		Kind:       "text/plain",
		Payload: map[string]any{
			"text": "hello-artifact-attachment",
		},
	})
	if err != nil {
		t.Fatalf("seed artifact: %v", err)
	}

	out, err := rt.RunProgram(context.Background(), attached.SessionID, execution.Program{
		ProgramID: "prog_artifact_attachment",
		Nodes: []execution.ProgramNode{{
			NodeID: "node_read",
			Action: action.Spec{ToolName: "demo.read_file"},
			InputBinds: []execution.ProgramInputBinding{{
				Name: "path",
				Kind: execution.ProgramInputBindingAttachment,
				Attachment: &execution.AttachmentInput{
					Kind:        execution.AttachmentInputArtifactRef,
					ArtifactID:  artifact.ArtifactID,
					Materialize: execution.AttachmentMaterializeTempFile,
				},
			}},
		}},
	})
	if err != nil {
		t.Fatalf("run program: %v", err)
	}
	if len(out.Executions) != 1 {
		t.Fatalf("expected 1 execution, got %#v", out.Executions)
	}
	actions := mustListActions(t, rt, attached.SessionID)
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %#v", actions)
	}
	if got, _ := actions[0].Result.Data["stdout"].(string); got != "hello-artifact-attachment" {
		t.Fatalf("expected materialized artifact payload, got %#v", actions[0].Result.Data)
	}
}

func TestCreatePlanFromProgramRejectsDependencyCycles(t *testing.T) {
	rt := hruntime.New(hruntime.Options{})
	sess := mustCreateSession(t, rt, "program cycle", "detect dependency cycle")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "detect dependency cycle"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	_, err = rt.CreatePlanFromProgram(attached.SessionID, "", execution.Program{
		Nodes: []execution.ProgramNode{
			{NodeID: "node_a", Action: action.Spec{ToolName: "demo.message"}, DependsOn: []string{"node_b"}},
			{NodeID: "node_b", Action: action.Spec{ToolName: "demo.message"}, DependsOn: []string{"node_a"}},
		},
	})
	if !errors.Is(err, hruntime.ErrProgramCycleDetected) {
		t.Fatalf("expected program cycle error, got %v", err)
	}
}

type targetAwareHandler struct{}

func (targetAwareHandler) Invoke(_ context.Context, args map[string]any) (action.Result, error) {
	target, _ := args[execution.TargetArgKey].(map[string]any)
	targetID, _ := target[execution.TargetMetadataKeyID].(string)
	return action.Result{
		OK: true,
		Data: map[string]any{
			"target_id": targetID,
			"stdout":    targetID,
		},
	}, nil
}

type targetScriptedHandler struct {
	mu      sync.Mutex
	outputs map[string][]string
	calls   map[string]int
}

func (h *targetScriptedHandler) Invoke(_ context.Context, args map[string]any) (action.Result, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.calls == nil {
		h.calls = map[string]int{}
	}
	target, _ := args[execution.TargetArgKey].(map[string]any)
	targetID, _ := target[execution.TargetMetadataKeyID].(string)
	callIndex := h.calls[targetID]
	h.calls[targetID] = callIndex + 1

	outputs := h.outputs[targetID]
	output := ""
	if len(outputs) > 0 {
		if callIndex >= len(outputs) {
			output = outputs[len(outputs)-1]
		} else {
			output = outputs[callIndex]
		}
	}
	return action.Result{
		OK: true,
		Data: map[string]any{
			"target_id": targetID,
			"stdout":    output,
		},
	}, nil
}

type structuredProducerHandler struct{}

func (structuredProducerHandler) Invoke(_ context.Context, _ map[string]any) (action.Result, error) {
	return action.Result{
		OK: true,
		Data: map[string]any{
			"payload": map[string]any{
				"message": "hello-from-prepare",
			},
			"stdout": "hello-from-prepare",
		},
	}, nil
}

type echoArgHandler struct{}

func (echoArgHandler) Invoke(_ context.Context, args map[string]any) (action.Result, error) {
	return action.Result{
		OK: true,
		Data: map[string]any{
			"stdout": fmt.Sprint(args["message"]),
		},
	}, nil
}

type artifactRefHandler struct{}

func (artifactRefHandler) Invoke(_ context.Context, args map[string]any) (action.Result, error) {
	var ref execution.ArtifactRef
	switch typed := args["artifact"].(type) {
	case execution.ArtifactRef:
		ref = typed
	case map[string]any:
		ref.ArtifactID, _ = typed["artifact_id"].(string)
		ref.Name, _ = typed["name"].(string)
		ref.Kind, _ = typed["kind"].(string)
	}
	return action.Result{
		OK: true,
		Data: map[string]any{
			"artifact_id":   ref.ArtifactID,
			"artifact_kind": ref.Kind,
		},
	}, nil
}

type staticTargetResolver struct {
	targetsByNode map[string][]execution.Target
}

func (r staticTargetResolver) ResolveTargets(_ context.Context, _ session.State, _ task.Record, _ execution.Program, node execution.ProgramNode) ([]execution.Target, error) {
	return append([]execution.Target(nil), r.targetsByNode[node.NodeID]...), nil
}

type fileReaderHandler struct{}

func (fileReaderHandler) Invoke(_ context.Context, args map[string]any) (action.Result, error) {
	path, _ := args["path"].(string)
	data, err := os.ReadFile(path)
	if err != nil {
		return action.Result{OK: false, Error: &action.Error{Code: "READ_FAILED", Message: err.Error()}}, err
	}
	return action.Result{
		OK: true,
		Data: map[string]any{
			"stdout": string(data),
			"path":   path,
		},
	}, nil
}

func TestCreatePlanFromProgramExpandsExplicitFanoutTargetsIntoTargetScopedSteps(t *testing.T) {
	rt := hruntime.New(hruntime.Options{})
	sess := mustCreateSession(t, rt, "program fanout", "expand explicit targets")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "expand explicit targets"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	created, err := rt.CreatePlanFromProgram(attached.SessionID, "", execution.Program{
		Nodes: []execution.ProgramNode{{
			NodeID: "node_fanout",
			Action: action.Spec{ToolName: "demo.message"},
			Targeting: &execution.TargetSelection{
				Mode: execution.TargetSelectionFanoutExplicit,
				Targets: []execution.Target{
					{TargetID: "host-a", Kind: "host"},
					{TargetID: "host-b", Kind: "host"},
				},
			},
		}},
	})
	if err != nil {
		t.Fatalf("create plan from program: %v", err)
	}
	if len(created.Steps) != 2 {
		t.Fatalf("expected 2 target-scoped steps, got %#v", created.Steps)
	}
	firstTarget, ok := execution.TargetFromStep(created.Steps[0])
	if !ok || firstTarget.TargetID != "host-a" {
		t.Fatalf("expected first compiled target host-a, got %#v", created.Steps[0])
	}
	secondTarget, ok := execution.TargetFromStep(created.Steps[1])
	if !ok || secondTarget.TargetID != "host-b" {
		t.Fatalf("expected second compiled target host-b, got %#v", created.Steps[1])
	}
}

func TestRunProgramPersistsTargetScopedFactsForFanoutSteps(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.message", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, messageHandler{})

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "program target facts", "persist target scoped facts")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "persist target scoped facts"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	out, err := rt.RunProgram(context.Background(), attached.SessionID, execution.Program{
		Nodes: []execution.ProgramNode{{
			NodeID: "node_fanout",
			Action: action.Spec{ToolName: "demo.message", Args: map[string]any{"message": "fanout"}},
			Targeting: &execution.TargetSelection{
				Mode: execution.TargetSelectionFanoutExplicit,
				Targets: []execution.Target{
					{TargetID: "host-a", Kind: "host"},
					{TargetID: "host-b", Kind: "host"},
				},
			},
		}},
	})
	if err != nil {
		t.Fatalf("run program: %v", err)
	}
	if len(out.Executions) != 2 {
		t.Fatalf("expected 2 target-scoped executions, got %#v", out.Executions)
	}

	actions := mustListActions(t, rt, attached.SessionID)
	if len(actions) != 2 {
		t.Fatalf("expected 2 actions, got %#v", actions)
	}
	targets := map[string]bool{}
	for _, item := range actions {
		ref, ok := execution.TargetRefFromMetadata(item.Metadata)
		if !ok {
			t.Fatalf("expected target-scoped action metadata, got %#v", item)
		}
		targets[ref.TargetID] = true
	}
	if !targets["host-a"] || !targets["host-b"] {
		t.Fatalf("expected target-scoped action facts, got %#v", actions)
	}

	verifications := mustListVerifications(t, rt, attached.SessionID)
	if len(verifications) != 2 {
		t.Fatalf("expected 2 verifications, got %#v", verifications)
	}
	for _, item := range verifications {
		if _, ok := execution.TargetRefFromMetadata(item.Metadata); !ok {
			t.Fatalf("expected verification target, got %#v", item)
		}
	}

	artifacts := mustListArtifacts(t, rt, attached.SessionID)
	if len(artifacts) == 0 {
		t.Fatalf("expected artifacts, got none")
	}
	for _, item := range artifacts {
		if _, ok := execution.TargetRefFromMetadata(item.Metadata); !ok {
			t.Fatalf("expected artifact target, got %#v", item)
		}
	}
}
