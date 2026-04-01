package runtime_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	goruntime "runtime"
	"strings"
	"sync"
	"testing"
	"time"

	shellmodule "github.com/yiiilin/harness-core/modules/shell"
	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
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

func TestCreatePlanKeepsAttachedProgramReadyGroupsScopedToParentStep(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.message", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, messageHandler{})

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "attached program groups", "scope attached program ready groups to their parent step")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "scope attached program ready groups"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	program := execution.Program{
		ProgramID: "prog_shared",
		Nodes: []execution.ProgramNode{{
			NodeID: "node_prepare",
			Action: action.Spec{ToolName: "demo.message"},
			InputBinds: []execution.ProgramInputBinding{
				{Name: "message", Kind: execution.ProgramInputBindingLiteral, Value: "prepare"},
			},
		}},
	}

	created, err := rt.CreatePlan(attached.SessionID, "attached programs", []plan.StepSpec{
		execution.AttachProgram(plan.StepSpec{StepID: "parent_a", Title: "parent a"}, program),
		execution.AttachProgram(plan.StepSpec{StepID: "parent_b", Title: "parent b"}, program),
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if len(created.Steps) != 2 {
		t.Fatalf("expected 2 compiled steps, got %#v", created.Steps)
	}

	groupByStep := map[string]string{}
	for _, step := range created.Steps {
		group, _ := step.Metadata["program_group_id"].(string)
		groupByStep[step.StepID] = group
	}
	if groupByStep["parent_a__prog_shared__node_prepare"] == "" || groupByStep["parent_b__prog_shared__node_prepare"] == "" {
		t.Fatalf("expected non-empty program group ids per attached program instance, got %#v", groupByStep)
	}
	if groupByStep["parent_a__prog_shared__node_prepare"] == groupByStep["parent_b__prog_shared__node_prepare"] {
		t.Fatalf("expected attached programs with the same ProgramID to keep distinct ready groups, got %#v", groupByStep)
	}
}

func TestCreatePlanFromProgramDoesNotInheritStaleConcurrencyMetadataWithoutExplicitPolicy(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.message", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, messageHandler{})

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "program metadata hygiene", "clear stale concurrency metadata when no explicit policy is set")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "clear stale concurrency metadata when no explicit policy is set"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	created, err := rt.CreatePlanFromProgram(attached.SessionID, "program metadata hygiene", execution.Program{
		ProgramID: "prog_metadata_hygiene",
		Nodes: []execution.ProgramNode{
			{
				NodeID:   "node_a",
				Action:   action.Spec{ToolName: "demo.message"},
				Metadata: map[string]any{execution.ProgramMetadataKeyMaxConcurrency: 1, execution.ProgramMetadataKeyNodeMaxConcurrency: 1},
			},
		},
	})
	if err != nil {
		t.Fatalf("create plan from program: %v", err)
	}
	if len(created.Steps) != 1 {
		t.Fatalf("expected one compiled step, got %#v", created.Steps)
	}
	if _, ok := created.Steps[0].Metadata[execution.ProgramMetadataKeyMaxConcurrency]; ok {
		t.Fatalf("expected stale program concurrency metadata to be cleared, got %#v", created.Steps[0].Metadata)
	}
	if _, ok := created.Steps[0].Metadata[execution.ProgramMetadataKeyNodeMaxConcurrency]; ok {
		t.Fatalf("expected stale node concurrency metadata to be cleared, got %#v", created.Steps[0].Metadata)
	}
}

func TestCreatePlanDoesNotInheritParentConcurrencyMetadataIntoAttachedProgramWithoutExplicitPolicy(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.message", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, messageHandler{})

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "attached metadata hygiene", "clear stale parent concurrency metadata when attached program has no explicit policy")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "clear stale parent concurrency metadata when attached program has no explicit policy"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	program := execution.Program{
		ProgramID: "prog_attached_metadata_hygiene",
		Nodes: []execution.ProgramNode{
			{
				NodeID: "node_a",
				Action: action.Spec{ToolName: "demo.message"},
			},
		},
	}

	created, err := rt.CreatePlan(attached.SessionID, "attached metadata hygiene", []plan.StepSpec{
		execution.AttachProgram(plan.StepSpec{
			StepID:   "parent",
			Title:    "parent",
			Metadata: map[string]any{execution.ProgramMetadataKeyMaxConcurrency: 1, execution.ProgramMetadataKeyNodeMaxConcurrency: 1},
		}, program),
	})
	if err != nil {
		t.Fatalf("create attached program plan: %v", err)
	}
	if len(created.Steps) != 1 {
		t.Fatalf("expected one compiled attached step, got %#v", created.Steps)
	}
	if _, ok := created.Steps[0].Metadata[execution.ProgramMetadataKeyMaxConcurrency]; ok {
		t.Fatalf("expected stale parent program concurrency metadata to be cleared, got %#v", created.Steps[0].Metadata)
	}
	if _, ok := created.Steps[0].Metadata[execution.ProgramMetadataKeyNodeMaxConcurrency]; ok {
		t.Fatalf("expected stale parent node concurrency metadata to be cleared, got %#v", created.Steps[0].Metadata)
	}
}

func TestCreatePlanFromProgramDoesNotInheritStaleProgramLineageMetadataWithoutExplicitLineage(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.message", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, messageHandler{})

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "program lineage hygiene", "clear stale lineage metadata when no explicit lineage is set")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "clear stale lineage metadata when no explicit lineage is set"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	created, err := rt.CreatePlanFromProgram(attached.SessionID, "program lineage hygiene", execution.Program{
		ProgramID: "prog_lineage_hygiene",
		Nodes: []execution.ProgramNode{
			{
				NodeID: "node_a",
				Action: action.Spec{ToolName: "demo.message"},
				Metadata: map[string]any{
					execution.ProgramMetadataKeyGroupID:      "stale_group",
					execution.ProgramMetadataKeyParentStepID: "stale_parent",
					execution.ProgramMetadataKeyDependsOn:    []string{"stale_dep"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("create plan from program: %v", err)
	}
	if len(created.Steps) != 1 {
		t.Fatalf("expected one compiled step, got %#v", created.Steps)
	}
	if got, _ := created.Steps[0].Metadata[execution.ProgramMetadataKeyGroupID].(string); got != "runtime/program:prog_lineage_hygiene" {
		t.Fatalf("expected runtime program group id, got %#v", created.Steps[0].Metadata)
	}
	if _, ok := created.Steps[0].Metadata[execution.ProgramMetadataKeyParentStepID]; ok {
		t.Fatalf("expected stale parent step id to be cleared, got %#v", created.Steps[0].Metadata)
	}
	if _, ok := created.Steps[0].Metadata[execution.ProgramMetadataKeyDependsOn]; ok {
		t.Fatalf("expected stale depends_on lineage to be cleared, got %#v", created.Steps[0].Metadata)
	}
}

func TestCreatePlanDoesNotInheritParentProgramLineageMetadataIntoAttachedProgramWithoutExplicitLineage(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.message", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, messageHandler{})

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "attached lineage hygiene", "clear stale parent lineage metadata when attached program has no explicit lineage")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "clear stale parent lineage metadata when attached program has no explicit lineage"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	program := execution.Program{
		ProgramID: "prog_attached_lineage_hygiene",
		Nodes: []execution.ProgramNode{
			{
				NodeID: "node_a",
				Action: action.Spec{ToolName: "demo.message"},
			},
		},
	}

	created, err := rt.CreatePlan(attached.SessionID, "attached lineage hygiene", []plan.StepSpec{
		execution.AttachProgram(plan.StepSpec{
			StepID: "parent",
			Title:  "parent",
			Metadata: map[string]any{
				execution.ProgramMetadataKeyGroupID:      "stale_group",
				execution.ProgramMetadataKeyParentStepID: "stale_parent",
				execution.ProgramMetadataKeyDependsOn:    []string{"stale_dep"},
			},
		}, program),
	})
	if err != nil {
		t.Fatalf("create attached program plan: %v", err)
	}
	if len(created.Steps) != 1 {
		t.Fatalf("expected one compiled attached step, got %#v", created.Steps)
	}
	if got, _ := created.Steps[0].Metadata[execution.ProgramMetadataKeyGroupID].(string); got != "parent__prog_attached_lineage_hygiene" {
		t.Fatalf("expected attached program group id, got %#v", created.Steps[0].Metadata)
	}
	if got, _ := created.Steps[0].Metadata[execution.ProgramMetadataKeyParentStepID].(string); got != "parent" {
		t.Fatalf("expected attached parent step id, got %#v", created.Steps[0].Metadata)
	}
	if _, ok := created.Steps[0].Metadata[execution.ProgramMetadataKeyDependsOn]; ok {
		t.Fatalf("expected stale depends_on lineage to be cleared on attached child, got %#v", created.Steps[0].Metadata)
	}
}

func TestCreatePlanFromProgramRejectsStepSelectorBindingsWithoutExplicitDependencies(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.message", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, messageHandler{})

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	cases := []struct {
		name    string
		binding execution.ProgramInputBinding
	}{
		{
			name: "output ref step selector",
			binding: execution.ProgramInputBinding{
				Name: "message",
				Kind: execution.ProgramInputBindingOutputRef,
				Ref: &execution.OutputRef{
					Kind:   execution.OutputRefText,
					StepID: "node_prepare",
				},
			},
		},
		{
			name: "runtime handle step selector",
			binding: execution.ProgramInputBinding{
				Name: "handle",
				Kind: execution.ProgramInputBindingRuntimeHandleRef,
				RuntimeHandle: &execution.RuntimeHandleRef{
					StepID: "node_prepare",
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sess := mustCreateSession(t, rt, "program binding dependency", "reject implicit step-selector dataflow")
			tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "reject implicit step-selector dataflow"})
			attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
			if err != nil {
				t.Fatalf("attach task: %v", err)
			}

			_, err = rt.CreatePlanFromProgram(attached.SessionID, "binding dependency", execution.Program{
				ProgramID: "prog_binding_dependency",
				Nodes: []execution.ProgramNode{
					{
						NodeID: "node_prepare",
						Action: action.Spec{ToolName: "demo.message"},
						InputBinds: []execution.ProgramInputBinding{
							{Name: "message", Kind: execution.ProgramInputBindingLiteral, Value: "prepare"},
						},
					},
					{
						NodeID: "node_apply",
						Action: action.Spec{ToolName: "demo.message"},
						InputBinds: []execution.ProgramInputBinding{
							tc.binding,
						},
					},
				},
			})
			if err == nil || !strings.Contains(err.Error(), "explicit dependency") {
				t.Fatalf("expected explicit dependency validation error, got %v", err)
			}
		})
	}
}

func TestCreatePlanRejectsAttachedProgramStepSelectorBindingsWithoutExplicitDependencies(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.message", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, messageHandler{})

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "attached binding dependency", "reject implicit step-selector dataflow in attached programs")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "reject implicit step-selector dataflow in attached programs"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	_, err = rt.CreatePlan(attached.SessionID, "attached binding dependency", []plan.StepSpec{
		execution.AttachProgram(plan.StepSpec{StepID: "parent"}, execution.Program{
			ProgramID: "prog_attached_binding_dependency",
			Nodes: []execution.ProgramNode{
				{
					NodeID: "node_prepare",
					Action: action.Spec{ToolName: "demo.message"},
					InputBinds: []execution.ProgramInputBinding{
						{Name: "message", Kind: execution.ProgramInputBindingLiteral, Value: "prepare"},
					},
				},
				{
					NodeID: "node_apply",
					Action: action.Spec{ToolName: "demo.message"},
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
		}),
	})
	if err == nil || !strings.Contains(err.Error(), "explicit dependency") {
		t.Fatalf("expected explicit dependency validation error for attached program, got %v", err)
	}
}

func TestCreatePlanFromProgramAllowsDirectKernelOwnedRefsWithoutDependencies(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.message", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, messageHandler{})

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	cases := []struct {
		name    string
		binding execution.ProgramInputBinding
	}{
		{
			name: "artifact id selector",
			binding: execution.ProgramInputBinding{
				Name: "artifact",
				Kind: execution.ProgramInputBindingOutputRef,
				Ref: &execution.OutputRef{
					Kind:       execution.OutputRefArtifact,
					ArtifactID: "art_seed",
				},
			},
		},
		{
			name: "runtime handle id selector",
			binding: execution.ProgramInputBinding{
				Name: "handle",
				Kind: execution.ProgramInputBindingRuntimeHandleRef,
				RuntimeHandle: &execution.RuntimeHandleRef{
					HandleID: "hdl_seed",
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sess := mustCreateSession(t, rt, "program direct kernel refs", "allow direct kernel-owned refs without dependency edges")
			tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "allow direct kernel-owned refs without dependency edges"})
			attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
			if err != nil {
				t.Fatalf("attach task: %v", err)
			}

			created, err := rt.CreatePlanFromProgram(attached.SessionID, "direct kernel refs", execution.Program{
				ProgramID: "prog_direct_kernel_refs",
				Nodes: []execution.ProgramNode{
					{
						NodeID: "node_prepare",
						Action: action.Spec{ToolName: "demo.message"},
						InputBinds: []execution.ProgramInputBinding{
							{Name: "message", Kind: execution.ProgramInputBindingLiteral, Value: "prepare"},
						},
					},
					{
						NodeID: "node_apply",
						Action: action.Spec{ToolName: "demo.message"},
						InputBinds: []execution.ProgramInputBinding{
							tc.binding,
						},
					},
				},
			})
			if err != nil {
				t.Fatalf("create plan from program: %v", err)
			}
			if len(created.Steps) != 2 {
				t.Fatalf("expected compiled steps for direct kernel refs, got %#v", created.Steps)
			}
		})
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

func TestRunProgramContinuesIntoDependentNodeAfterToleratedPartialFanoutFailure(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(
		tool.Definition{ToolName: "demo.target-scripted", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true},
		&targetScriptedHandler{outputs: map[string][]string{
			"host-a": {"bad"},
			"host-b": {"ok"},
		}},
	)
	tools.Register(
		tool.Definition{ToolName: "demo.message", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true},
		messageHandler{},
	)
	verifiers := verify.NewRegistry()
	verifiers.Register(verify.Definition{Kind: "output_contains", Description: "Verify output contains substring."}, verify.OutputContainsChecker{})

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verifiers,
		Policy:    permission.DefaultEvaluator{},
		LoopBudgets: func() hruntime.LoopBudgets {
			budgets := hruntime.DefaultLoopBudgets()
			budgets.MaxRetriesPerStep = 0
			return budgets
		}(),
	})

	sess := mustCreateSession(t, rt, "program dependent partial failure", "continue into dependent node after tolerated partial fanout failure")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "continue into dependent node after tolerated partial fanout failure"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	out, err := rt.RunProgram(context.Background(), attached.SessionID, execution.Program{
		ProgramID: "prog_partial_then_join",
		Nodes: []execution.ProgramNode{
			{
				NodeID: "node_apply",
				Action: action.Spec{ToolName: "demo.target-scripted"},
				Verify: &verify.Spec{
					Mode: verify.ModeAll,
					Checks: []verify.Check{{
						Kind: "output_contains",
						Args: map[string]any{"text": "ok"},
					}},
				},
				Targeting: &execution.TargetSelection{
					Mode:             execution.TargetSelectionFanoutExplicit,
					OnPartialFailure: execution.TargetFailureContinue,
					Targets: []execution.Target{
						{TargetID: "host-a", Kind: "host"},
						{TargetID: "host-b", Kind: "host"},
					},
				},
			},
			{
				NodeID:    "node_join",
				Action:    action.Spec{ToolName: "demo.message", Args: map[string]any{"message": "joined"}},
				DependsOn: []string{"node_apply"},
			},
		},
	})
	if err != nil {
		t.Fatalf("run program: %v", err)
	}
	if out.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected complete session, got %#v", out.Session)
	}
	if len(out.Executions) < 3 {
		t.Fatalf("expected fanout executions plus dependent join, got %#v", out.Executions)
	}
	if out.Executions[len(out.Executions)-1].Execution.Step.StepID != "prog_partial_then_join__node_join" {
		t.Fatalf("expected dependent node to execute after tolerated partial fanout failure, got %#v", out.Executions)
	}
	if len(out.Aggregates) != 1 || out.Aggregates[0].Status != execution.AggregateStatusPartialFailed {
		t.Fatalf("expected partial_failed aggregate while continuing to dependent node, got %#v", out.Aggregates)
	}
}

func TestRunProgramFanoutConsumesMaxConcurrency(t *testing.T) {
	handler := newConcurrencyProbeHandler(2)
	tools := tool.NewRegistry()
	tools.Register(
		tool.Definition{ToolName: "demo.concurrent-target", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true},
		handler,
	)

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "program max concurrency", "consume target max concurrency")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "consume target max concurrency"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go func() {
		select {
		case <-handler.concurrent:
			close(handler.release)
		case <-ctx.Done():
		}
	}()

	out, err := rt.RunProgram(ctx, attached.SessionID, execution.Program{
		ProgramID: "prog_concurrent_fanout",
		Nodes: []execution.ProgramNode{{
			NodeID: "node_apply",
			Action: action.Spec{ToolName: "demo.concurrent-target"},
			Targeting: &execution.TargetSelection{
				Mode:           execution.TargetSelectionFanoutExplicit,
				MaxConcurrency: 2,
				Targets: []execution.Target{
					{TargetID: "host-a", Kind: "host"},
					{TargetID: "host-b", Kind: "host"},
					{TargetID: "host-c", Kind: "host"},
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
	if got := handler.maxObserved(); got != 2 {
		t.Fatalf("expected runtime to cap concurrent target execution at 2, got %d", got)
	}
}

func TestRunProgramExecutesReadySiblingNodesConcurrentlyBeyondFanoutGroups(t *testing.T) {
	handler := newConcurrencyProbeHandler(2)
	tools := tool.NewRegistry()
	tools.Register(
		tool.Definition{ToolName: "demo.concurrent-ready", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true},
		handler,
	)
	tools.Register(
		tool.Definition{ToolName: "demo.message", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true},
		messageHandler{},
	)

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "program ready siblings", "execute dependency-ready sibling nodes concurrently")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "execute ready sibling nodes beyond fanout groups"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go func() {
		select {
		case <-handler.concurrent:
			close(handler.release)
		case <-ctx.Done():
		}
	}()

	out, err := rt.RunProgram(ctx, attached.SessionID, execution.Program{
		ProgramID: "prog_ready_siblings",
		Nodes: []execution.ProgramNode{
			{
				NodeID: "node_prepare",
				Action: action.Spec{ToolName: "demo.concurrent-ready"},
			},
			{
				NodeID:    "node_apply",
				Action:    action.Spec{ToolName: "demo.message", Args: map[string]any{"message": "apply"}},
				DependsOn: []string{"node_prepare"},
			},
			{
				NodeID: "node_collect",
				Action: action.Spec{ToolName: "demo.concurrent-ready"},
			},
			{
				NodeID:    "node_join",
				Action:    action.Spec{ToolName: "demo.message", Args: map[string]any{"message": "join"}},
				DependsOn: []string{"node_apply", "node_collect"},
			},
		},
	})
	if err != nil {
		t.Fatalf("run program: %v", err)
	}
	if out.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected complete session, got %#v", out.Session)
	}
	if got := handler.maxObserved(); got != 2 {
		t.Fatalf("expected ready sibling nodes to run concurrently at width 2, got %d", got)
	}
	if len(out.Executions) != 4 {
		t.Fatalf("expected all four nodes to execute, got %#v", out.Executions)
	}
	if out.Executions[0].Execution.Step.StepID != "prog_ready_siblings__node_prepare" ||
		out.Executions[1].Execution.Step.StepID != "prog_ready_siblings__node_collect" ||
		out.Executions[2].Execution.Step.StepID != "prog_ready_siblings__node_apply" ||
		out.Executions[3].Execution.Step.StepID != "prog_ready_siblings__node_join" {
		t.Fatalf("expected ready sibling round to run prepare/collect before apply/join, got %#v", out.Executions)
	}
}

func TestRunProgramUsesExplicitProgramConcurrencyPolicyForReadySiblingRounds(t *testing.T) {
	handler := newConcurrencyProbeHandler(1)
	tools := tool.NewRegistry()
	tools.Register(
		tool.Definition{ToolName: "demo.concurrent-program-limit", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true},
		handler,
	)

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "program explicit concurrency", "respect explicit program concurrency policy")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "respect explicit program concurrency policy"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go func() {
		select {
		case <-handler.concurrent:
			close(handler.release)
		case <-ctx.Done():
		}
	}()

	out, err := rt.RunProgram(ctx, attached.SessionID, execution.Program{
		ProgramID: "prog_program_concurrency_policy",
		Concurrency: &execution.ConcurrencyPolicy{
			MaxConcurrency: 1,
		},
		Nodes: []execution.ProgramNode{
			{NodeID: "node_a", Action: action.Spec{ToolName: "demo.concurrent-program-limit"}},
			{NodeID: "node_b", Action: action.Spec{ToolName: "demo.concurrent-program-limit"}},
		},
	})
	if err != nil {
		t.Fatalf("run program: %v", err)
	}
	if out.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected complete session, got %#v", out.Session)
	}
	if got := handler.maxObserved(); got != 1 {
		t.Fatalf("expected explicit program concurrency policy to cap ready siblings at 1, got %d", got)
	}
}

func TestRunProgramUsesExplicitNodeConcurrencyPolicyForPureFanout(t *testing.T) {
	handler := newConcurrencyProbeHandler(1)
	tools := tool.NewRegistry()
	tools.Register(
		tool.Definition{ToolName: "demo.concurrent-node-limit", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true},
		handler,
	)

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "program node concurrency", "respect explicit node concurrency policy for fanout")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "respect explicit node concurrency policy for fanout"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go func() {
		select {
		case <-handler.concurrent:
			close(handler.release)
		case <-ctx.Done():
		}
	}()

	out, err := rt.RunProgram(ctx, attached.SessionID, execution.Program{
		ProgramID: "prog_node_concurrency_policy",
		Nodes: []execution.ProgramNode{
			{
				NodeID: "node_apply",
				Action: action.Spec{ToolName: "demo.concurrent-node-limit"},
				Concurrency: &execution.ConcurrencyPolicy{
					MaxConcurrency: 1,
				},
				Targeting: &execution.TargetSelection{
					Mode:           execution.TargetSelectionFanoutExplicit,
					MaxConcurrency: 3,
					Targets: []execution.Target{
						{TargetID: "host-a", Kind: "host"},
						{TargetID: "host-b", Kind: "host"},
						{TargetID: "host-c", Kind: "host"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("run program: %v", err)
	}
	if out.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected complete session, got %#v", out.Session)
	}
	if got := handler.maxObserved(); got != 1 {
		t.Fatalf("expected explicit node concurrency policy to override fanout max concurrency, got %d", got)
	}
}

func TestRunProgramNodeConcurrencyOverridesProgramDefaultInMixedReadyRound(t *testing.T) {
	previousMaxProcs := goruntime.GOMAXPROCS(1)
	defer goruntime.GOMAXPROCS(previousMaxProcs)

	handler := newConcurrencyProbeHandler(2)
	tools := tool.NewRegistry()
	tools.Register(
		tool.Definition{ToolName: "demo.concurrent-override", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true},
		handler,
	)
	tools.Register(
		tool.Definition{ToolName: "demo.message", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true},
		messageHandler{},
	)

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "program concurrency override", "allow node concurrency override to narrow mixed ready rounds")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "allow node concurrency override to narrow mixed ready rounds"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go func() {
		select {
		case <-handler.concurrent:
			close(handler.release)
		case <-ctx.Done():
		}
	}()

	out, err := rt.RunProgram(ctx, attached.SessionID, execution.Program{
		ProgramID: "prog_concurrency_override",
		Concurrency: &execution.ConcurrencyPolicy{
			MaxConcurrency: 3,
		},
		Nodes: []execution.ProgramNode{
			{
				NodeID: "node_apply",
				Action: action.Spec{ToolName: "demo.concurrent-override"},
				Concurrency: &execution.ConcurrencyPolicy{
					MaxConcurrency: 1,
				},
				Targeting: &execution.TargetSelection{
					Mode:           execution.TargetSelectionFanoutExplicit,
					MaxConcurrency: 2,
					Targets: []execution.Target{
						{TargetID: "host-a", Kind: "host"},
						{TargetID: "host-b", Kind: "host"},
					},
				},
			},
			{
				NodeID: "node_collect",
				Action: action.Spec{ToolName: "demo.concurrent-override"},
			},
			{
				NodeID:    "node_join",
				Action:    action.Spec{ToolName: "demo.message", Args: map[string]any{"message": "join"}},
				DependsOn: []string{"node_apply", "node_collect"},
			},
		},
	})
	if err != nil {
		t.Fatalf("run program: %v", err)
	}
	if out.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected complete session, got %#v", out.Session)
	}
	if got := handler.maxObserved(); got != 2 {
		t.Fatalf("expected node concurrency override to narrow mixed ready round width to 2, got %d", got)
	}
}

func TestRunProgramNodeConcurrencyOverrideDoesNotLetSiblingTargetsConsumeProgramSlots(t *testing.T) {
	previousMaxProcs := goruntime.GOMAXPROCS(1)
	defer goruntime.GOMAXPROCS(previousMaxProcs)

	handler := newLabeledBlockingHandler()
	tools := tool.NewRegistry()
	tools.Register(
		tool.Definition{ToolName: "demo.concurrent-slot-test", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true},
		handler,
	)

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "program concurrency slot test", "do not let throttled sibling targets consume program slots")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "do not let throttled sibling targets consume program slots"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	defer handler.releaseAll()

	done := make(chan error, 1)
	go func() {
		_, err := rt.RunProgram(ctx, attached.SessionID, execution.Program{
			ProgramID: "prog_concurrency_slot_test",
			Concurrency: &execution.ConcurrencyPolicy{
				MaxConcurrency: 2,
			},
			Nodes: []execution.ProgramNode{
				{
					NodeID: "node_apply",
					Action: action.Spec{ToolName: "demo.concurrent-slot-test"},
					Concurrency: &execution.ConcurrencyPolicy{
						MaxConcurrency: 1,
					},
					Targeting: &execution.TargetSelection{
						Mode:           execution.TargetSelectionFanoutExplicit,
						MaxConcurrency: 2,
						Targets: []execution.Target{
							{TargetID: "host-a", Kind: "host"},
							{TargetID: "host-b", Kind: "host"},
						},
					},
				},
				{
					NodeID: "node_collect",
					Action: action.Spec{ToolName: "demo.concurrent-slot-test", Args: map[string]any{"label": "collect"}},
				},
			},
		})
		done <- err
	}()

	first := handler.waitForStart(t, time.Second)
	second := handler.waitForStart(t, 100*time.Millisecond)
	started := map[string]bool{
		first:  true,
		second: true,
	}
	if started["host-b"] {
		t.Fatalf("expected throttled second target not to consume a program slot before the independent sibling starts, got %q then %q", first, second)
	}
	if !started["collect"] {
		t.Fatalf("expected independent sibling to start within the first two slots, got %q then %q", first, second)
	}

	handler.releaseAll()
	if err := <-done; err != nil {
		t.Fatalf("run program: %v", err)
	}
}

func TestRunProgramExecutesReadyFanoutAndNonFanoutSiblingsConcurrently(t *testing.T) {
	handler := newConcurrencyProbeHandler(3)
	tools := tool.NewRegistry()
	tools.Register(
		tool.Definition{ToolName: "demo.concurrent-mixed", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true},
		handler,
	)
	tools.Register(
		tool.Definition{ToolName: "demo.message", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true},
		messageHandler{},
	)

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "program mixed ready siblings", "execute ready fanout and non-fanout siblings concurrently")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "execute ready fanout and non-fanout siblings concurrently"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go func() {
		select {
		case <-handler.concurrent:
			close(handler.release)
		case <-ctx.Done():
		}
	}()

	out, err := rt.RunProgram(ctx, attached.SessionID, execution.Program{
		ProgramID: "prog_mixed_ready_siblings",
		Nodes: []execution.ProgramNode{
			{
				NodeID: "node_apply",
				Action: action.Spec{ToolName: "demo.concurrent-mixed"},
				Targeting: &execution.TargetSelection{
					Mode:           execution.TargetSelectionFanoutExplicit,
					MaxConcurrency: 2,
					Targets: []execution.Target{
						{TargetID: "host-a", Kind: "host"},
						{TargetID: "host-b", Kind: "host"},
					},
				},
			},
			{
				NodeID: "node_collect",
				Action: action.Spec{ToolName: "demo.concurrent-mixed"},
			},
			{
				NodeID:    "node_join",
				Action:    action.Spec{ToolName: "demo.message", Args: map[string]any{"message": "join"}},
				DependsOn: []string{"node_apply", "node_collect"},
			},
		},
	})
	if err != nil {
		t.Fatalf("run program: %v", err)
	}
	if out.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected complete session, got %#v", out.Session)
	}
	if got := handler.maxObserved(); got != 3 {
		t.Fatalf("expected mixed ready siblings to run concurrently at width 3, got %d", got)
	}
	if len(out.Executions) != 4 {
		t.Fatalf("expected two fanout executions, one sibling, and join, got %#v", out.Executions)
	}
	if out.Executions[len(out.Executions)-1].Execution.Step.StepID != "prog_mixed_ready_siblings__node_join" {
		t.Fatalf("expected join to execute after mixed ready round, got %#v", out.Executions)
	}
}

func TestRunProgramMixedReadyRoundDoesNotHeadOfLineBlockOtherGroups(t *testing.T) {
	handler := newConcurrencyProbeHandler(2)
	tools := tool.NewRegistry()
	tools.Register(
		tool.Definition{ToolName: "demo.concurrent-limited", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true},
		handler,
	)
	tools.Register(
		tool.Definition{ToolName: "demo.message", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true},
		messageHandler{},
	)

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "program mixed ready head of line", "do not head-of-line block other ready groups")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "do not head-of-line block other ready groups"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go func() {
		select {
		case <-handler.concurrent:
			close(handler.release)
		case <-ctx.Done():
		}
	}()

	out, err := rt.RunProgram(ctx, attached.SessionID, execution.Program{
		ProgramID: "prog_mixed_head_of_line",
		Nodes: []execution.ProgramNode{
			{
				NodeID: "node_apply",
				Action: action.Spec{ToolName: "demo.concurrent-limited"},
				Targeting: &execution.TargetSelection{
					Mode:           execution.TargetSelectionFanoutExplicit,
					MaxConcurrency: 1,
					Targets: []execution.Target{
						{TargetID: "host-a", Kind: "host"},
						{TargetID: "host-b", Kind: "host"},
					},
				},
			},
			{
				NodeID: "node_collect",
				Action: action.Spec{ToolName: "demo.concurrent-limited"},
			},
			{
				NodeID:    "node_join",
				Action:    action.Spec{ToolName: "demo.message", Args: map[string]any{"message": "join"}},
				DependsOn: []string{"node_apply", "node_collect"},
			},
		},
	})
	if err != nil {
		t.Fatalf("run program: %v", err)
	}
	if out.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected complete session, got %#v", out.Session)
	}
	if got := handler.maxObserved(); got != 2 {
		t.Fatalf("expected limited fanout group to overlap with other ready work at width 2, got %d", got)
	}
}

func TestRunProgramFinalizesAggregateVerificationForEachReadyFanoutNode(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.target", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, targetAwareHandler{})
	verifiers := verify.NewRegistry()
	verify.RegisterBuiltins(verifiers)

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verifiers,
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "program multiple aggregate verify", "finalize aggregate verification for each ready fanout node")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "finalize aggregate verification for each ready fanout node"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	out, err := rt.RunProgram(context.Background(), attached.SessionID, execution.Program{
		ProgramID: "prog_multi_aggregate_verify",
		Nodes: []execution.ProgramNode{
			{
				NodeID:      "node_alpha",
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
						{TargetID: "alpha-a", Kind: "host"},
						{TargetID: "alpha-b", Kind: "host"},
					},
				},
			},
			{
				NodeID:      "node_beta",
				Action:      action.Spec{ToolName: "demo.target"},
				VerifyScope: execution.VerificationScopeAggregate,
				Verify: &verify.Spec{
					Mode: verify.ModeAll,
					Checks: []verify.Check{
						{Kind: "value_equals", Args: map[string]any{"path": "result.data.completed", "expected": 2}},
					},
				},
				OnFail: &plan.OnFailSpec{Strategy: "abort"},
				Targeting: &execution.TargetSelection{
					Mode: execution.TargetSelectionFanoutExplicit,
					Targets: []execution.Target{
						{TargetID: "beta-a", Kind: "host"},
						{TargetID: "beta-b", Kind: "host"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("run program: %v", err)
	}
	if out.Session.Phase != session.PhaseFailed {
		t.Fatalf("expected failed session when one aggregate verify fails, got %#v", out.Session)
	}

	verifications := mustListVerifications(t, rt, attached.SessionID)
	resolved := map[string]bool{}
	for _, record := range verifications {
		if got, _ := record.Metadata["verification_scope"].(string); got != string(execution.VerificationScopeAggregate) {
			continue
		}
		nodeID, _ := record.Metadata[execution.ProgramMetadataKeyNodeID].(string)
		if nodeID == "" || record.Result.Reason == "aggregate pending" {
			continue
		}
		resolved[nodeID] = true
	}
	if !resolved["node_alpha"] || !resolved["node_beta"] {
		t.Fatalf("expected each aggregate node to get a resolved aggregate verification, got %#v from %#v", resolved, verifications)
	}
}

func TestRunProgramAttributesMixedReadyRoundFailureToFailingSibling(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(
		tool.Definition{ToolName: "demo.scripted-message", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true},
		scriptedMessageHandler{},
	)

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "program mixed failure attribution", "attribute mixed ready round failure to the failing sibling")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "attribute mixed ready round failure to the failing sibling"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	out, err := rt.RunProgram(context.Background(), attached.SessionID, execution.Program{
		ProgramID: "prog_mixed_failure_attr",
		Nodes: []execution.ProgramNode{
			{
				NodeID: "node_fail",
				Action: action.Spec{ToolName: "demo.scripted-message", Args: map[string]any{"message": "fail"}},
				OnFail: &plan.OnFailSpec{Strategy: "abort"},
			},
			{
				NodeID: "node_ok",
				Action: action.Spec{ToolName: "demo.scripted-message", Args: map[string]any{"message": "ok"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("run program: %v", err)
	}
	if out.Session.Phase != session.PhaseFailed {
		t.Fatalf("expected failed session, got %#v", out.Session)
	}
	if out.Session.CurrentStepID != "prog_mixed_failure_attr__node_fail" {
		t.Fatalf("expected failed session to point at failing sibling, got %#v", out.Session)
	}

	events := mustListAuditEvents(t, rt, attached.SessionID)
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type != audit.EventStateChanged {
			continue
		}
		if to := fmt.Sprint(events[i].Payload["to"]); to == string(hruntime.TransitionFailed) {
			if events[i].StepID != "prog_mixed_failure_attr__node_fail" {
				t.Fatalf("expected final failed state-change event to point at failing sibling, got %#v", events[i])
			}
			return
		}
	}
	t.Fatalf("expected failed state-change event, got %#v", events)
}

func TestRunProgramAttributesExhaustedContinueAggregateFailureToExhaustingTarget(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(
		tool.Definition{ToolName: "demo.target-error", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true},
		targetErrorHandler{failTargets: map[string]bool{"host-a": true, "host-b": true}},
	)
	tools.Register(
		tool.Definition{ToolName: "demo.allow-tool", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true},
		messageHandler{},
	)

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
		LoopBudgets: func() hruntime.LoopBudgets {
			budgets := hruntime.DefaultLoopBudgets()
			budgets.MaxRetriesPerStep = 0
			return budgets
		}(),
	})

	sess := mustCreateSession(t, rt, "program exhausted continue aggregate attribution", "attribute exhausted continue aggregate failure to the exhausting target")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "attribute exhausted continue aggregate failure to the exhausting target"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	out, err := rt.RunProgram(context.Background(), attached.SessionID, execution.Program{
		ProgramID: "prog_continue_aggregate_attr",
		Nodes: []execution.ProgramNode{
			{
				NodeID: "node_apply",
				Action: action.Spec{ToolName: "demo.target-error"},
				Targeting: &execution.TargetSelection{
					Mode:             execution.TargetSelectionFanoutExplicit,
					OnPartialFailure: execution.TargetFailureContinue,
					Targets: []execution.Target{
						{TargetID: "host-a", Kind: "host"},
						{TargetID: "host-b", Kind: "host"},
					},
				},
			},
			{
				NodeID: "node_collect",
				Action: action.Spec{ToolName: "demo.allow-tool", Args: map[string]any{"message": "collect"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("run program: %v", err)
	}
	if out.Session.Phase != session.PhaseFailed {
		t.Fatalf("expected exhausted continue aggregate to fail the session, got %#v", out.Session)
	}
	if out.Session.CurrentStepID != "prog_continue_aggregate_attr__node_apply__host-b" {
		t.Fatalf("expected final attribution to point at the exhausting target step, got %#v", out.Session)
	}

	events := mustListAuditEvents(t, rt, attached.SessionID)
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type != audit.EventStateChanged {
			continue
		}
		if to := fmt.Sprint(events[i].Payload["to"]); to == string(hruntime.TransitionFailed) {
			if events[i].StepID != "prog_continue_aggregate_attr__node_apply__host-b" {
				t.Fatalf("expected final failed state-change event to point at the exhausting target step, got %#v", events[i])
			}
			return
		}
	}
	t.Fatalf("expected final failed state-change event, got %#v", events)
}

func TestRunProgramExecutesAllowedReadySiblingBeforeEarlierApprovalRequiredSibling(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(
		tool.Definition{ToolName: "demo.ask-tool", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		messageHandler{},
	)
	tools.Register(
		tool.Definition{ToolName: "demo.allow-tool", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true},
		messageHandler{},
	)

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	}).WithPolicyEvaluator(toolScopedAskPolicy{askTools: map[string]bool{"demo.ask-tool": true}})

	sess := mustCreateSession(t, rt, "program mixed approval order", "execute allowed sibling before earlier approval-required sibling")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "execute allowed sibling before earlier approval-required sibling"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	out, err := rt.RunProgram(context.Background(), attached.SessionID, execution.Program{
		ProgramID: "prog_mixed_approval_order",
		Nodes: []execution.ProgramNode{
			{
				NodeID: "node_needs_approval",
				Action: action.Spec{ToolName: "demo.ask-tool", Args: map[string]any{"message": "approval"}},
			},
			{
				NodeID: "node_collect",
				Action: action.Spec{ToolName: "demo.allow-tool", Args: map[string]any{"message": "collect"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("run program: %v", err)
	}
	if out.Session.PendingApprovalID == "" || out.Session.ExecutionState != session.ExecutionAwaitingApproval {
		t.Fatalf("expected session to block on approval after running allowed sibling, got %#v", out.Session)
	}
	if len(out.Executions) == 0 {
		t.Fatalf("expected at least one execution before approval block, got %#v", out.Executions)
	}
	if out.Executions[0].Execution.Step.StepID != "prog_mixed_approval_order__node_collect" {
		t.Fatalf("expected allowed sibling to execute before approval-required sibling blocks, got %#v", out.Executions)
	}
}

type targetScopedAskPolicy struct {
	askTargets map[string]bool
}

func (p targetScopedAskPolicy) Evaluate(_ context.Context, _ session.State, step plan.StepSpec) (permission.Decision, error) {
	targetID, _ := step.Metadata[execution.TargetMetadataKeyID].(string)
	if p.askTargets != nil && p.askTargets[targetID] {
		return permission.Decision{
			Action:      permission.Ask,
			Reason:      "approval required",
			MatchedRule: "test/ask:" + targetID,
		}, nil
	}
	return permission.Decision{Action: permission.Allow, Reason: "allowed", MatchedRule: "test/allow"}, nil
}

func TestRunProgramPureTargetFanoutDoesNotUseReadyRoundApprovalSkipping(t *testing.T) {
	tools := tool.NewRegistry()
	handler := &countingHandler{}
	tools.Register(
		tool.Definition{ToolName: "demo.target-approval", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true},
		handler,
	)

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	}).WithPolicyEvaluator(targetScopedAskPolicy{askTargets: map[string]bool{"host-a": true}})

	sess := mustCreateSession(t, rt, "program pure fanout approval", "keep pure target fanout on the original approval path")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "keep pure target fanout on the original approval path"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	out, err := rt.RunProgram(context.Background(), attached.SessionID, execution.Program{
		ProgramID: "prog_pure_fanout_approval",
		Nodes: []execution.ProgramNode{
			{
				NodeID: "node_apply",
				Action: action.Spec{ToolName: "demo.target-approval"},
				Targeting: &execution.TargetSelection{
					Mode: execution.TargetSelectionFanoutExplicit,
					Targets: []execution.Target{
						{TargetID: "host-a", Kind: "host"},
						{TargetID: "host-b", Kind: "host"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("run program: %v", err)
	}
	if out.Session.PendingApprovalID == "" || out.Session.ExecutionState != session.ExecutionAwaitingApproval {
		t.Fatalf("expected pure fanout program to block on the approval-requiring target, got %#v", out.Session)
	}
	if handler.calls != 0 {
		t.Fatalf("expected no target executions before the approval gate resolves, got %d calls", handler.calls)
	}
	actions := mustListActions(t, rt, attached.SessionID)
	if len(actions) != 0 {
		t.Fatalf("expected no persisted target actions before approval, got %#v", actions)
	}
}

func TestBlockedRuntimeProjectionPreservesApprovalTargetLinkageForFanout(t *testing.T) {
	tools := tool.NewRegistry()
	handler := &countingHandler{}
	tools.Register(
		tool.Definition{ToolName: "demo.target-approval", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true},
		handler,
	)

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	}).WithPolicyEvaluator(targetScopedAskPolicy{askTargets: map[string]bool{"host-a": true}})

	sess := mustCreateSession(t, rt, "program fanout blocked projection", "preserve approval target linkage in blocked projection")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "preserve approval target linkage in blocked projection"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	out, err := rt.RunProgram(context.Background(), attached.SessionID, execution.Program{
		ProgramID: "prog_pure_fanout_projection",
		Nodes: []execution.ProgramNode{
			{
				NodeID: "node_apply",
				Action: action.Spec{ToolName: "demo.target-approval"},
				Targeting: &execution.TargetSelection{
					Mode: execution.TargetSelectionFanoutExplicit,
					Targets: []execution.Target{
						{TargetID: "host-a", Kind: "host"},
						{TargetID: "host-b", Kind: "host"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("run program: %v", err)
	}
	if out.Session.PendingApprovalID == "" || out.Session.ExecutionState != session.ExecutionAwaitingApproval {
		t.Fatalf("expected pending approval, got %#v", out)
	}

	view, err := rt.GetBlockedRuntimeProjection(attached.SessionID)
	if err != nil {
		t.Fatalf("get blocked runtime projection: %v", err)
	}
	if view.Wait.Target.TargetID != "host-a" || view.Wait.Target.Kind != "host" {
		t.Fatalf("expected blocked wait target linkage, got %#v", view.Wait)
	}
	if view.Runtime.Target.TargetID != "host-a" || view.Runtime.Target.Kind != "host" {
		t.Fatalf("expected blocked runtime target linkage, got %#v", view.Runtime)
	}
	if view.BlockedRuntimeLinkage == nil || view.BlockedRuntimeLinkage.Target.TargetID != "host-a" || view.BlockedRuntimeLinkage.Target.Kind != "host" {
		t.Fatalf("expected blocked runtime typed target linkage, got %#v", view)
	}
}

func TestRunProgramInterruptedReadyRoundUsesPreparedSiblingAsInFlightAnchorWhenAskSiblingWasSkipped(t *testing.T) {
	boom := errors.New("boom:plan.update")
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := &nthFailingPlanUpdateStore{
		Store:            plan.NewMemoryStore(),
		updateErr:        boom,
		failOnUpdateCall: 1,
	}
	tools := tool.NewRegistry()
	handler := &countingHandler{}

	tools.Register(
		tool.Definition{ToolName: "demo.ask-skip", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		messageHandler{},
	)
	tools.Register(
		tool.Definition{ToolName: "demo.allow-count", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true},
		handler,
	)

	rt := hruntime.New(hruntime.Options{
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	}).WithPolicyEvaluator(toolScopedAskPolicy{askTools: map[string]bool{"demo.ask-skip": true}})

	sess := mustCreateSession(t, rt, "program ready round in-flight anchor", "use prepared sibling as in-flight anchor when ask sibling is skipped")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "use prepared sibling as in-flight anchor when ask sibling is skipped"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	if _, err := rt.RunProgram(context.Background(), attached.SessionID, execution.Program{
		ProgramID: "prog_ready_round_anchor_subset",
		Nodes: []execution.ProgramNode{
			{
				NodeID: "node_needs_approval",
				Action: action.Spec{ToolName: "demo.ask-skip", Args: map[string]any{"message": "approval"}},
			},
			{
				NodeID: "node_collect",
				Action: action.Spec{ToolName: "demo.allow-count"},
			},
		},
	}); !errors.Is(err, boom) {
		t.Fatalf("expected interrupted ready round to surface plan update error, got %v", err)
	}
	if handler.calls != 1 {
		t.Fatalf("expected only the allowed sibling to execute before interruption, got %d calls", handler.calls)
	}

	stored, err := rt.GetSession(attached.SessionID)
	if err != nil {
		t.Fatalf("get interrupted session: %v", err)
	}
	if stored.InFlightStepID != "prog_ready_round_anchor_subset__node_collect" {
		t.Fatalf("expected in-flight anchor to point at the actually prepared sibling, got %#v", stored)
	}
}

func TestRecoverSessionDoesNotReplayInterruptedProgramReadyRound(t *testing.T) {
	boom := errors.New("boom:plan.update")
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := &nthFailingPlanUpdateStore{
		Store:            plan.NewMemoryStore(),
		updateErr:        boom,
		failOnUpdateCall: 1,
	}
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	audits := audit.NewMemoryStore()
	handler := &countingHandler{}

	tools.Register(
		tool.Definition{ToolName: "demo.count", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true},
		handler,
	)

	opts := hruntime.Options{
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Tools:     tools,
		Verifiers: verifiers,
		Audit:     audits,
		Policy:    permission.DefaultEvaluator{},
	}

	rt1 := hruntime.New(opts)
	sess := mustCreateSession(t, rt1, "program ready round recovery", "do not replay interrupted ready round")
	tsk := mustCreateTask(t, rt1, task.Spec{TaskType: "demo", Goal: "do not replay interrupted ready round"})
	attached, err := rt1.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	if _, err := rt1.RunProgram(context.Background(), attached.SessionID, execution.Program{
		ProgramID: "prog_ready_round_recovery",
		Nodes: []execution.ProgramNode{
			{NodeID: "node_a", Action: action.Spec{ToolName: "demo.count"}},
			{NodeID: "node_b", Action: action.Spec{ToolName: "demo.count"}},
		},
	}); !errors.Is(err, boom) {
		t.Fatalf("expected interrupted ready round to surface plan update error, got %v", err)
	}
	if handler.calls != 2 {
		t.Fatalf("expected initial ready round to invoke both siblings once before interruption, got %d calls", handler.calls)
	}

	rt2 := hruntime.New(opts)
	recovered, err := rt2.RecoverSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("recover session: %v", err)
	}
	if handler.calls != 2 {
		t.Fatalf("expected recovery not to replay interrupted ready round, got %d calls", handler.calls)
	}
	if recovered.Session.Phase != session.PhaseFailed {
		t.Fatalf("expected interrupted ready round recovery to fail closed instead of replaying, got %#v", recovered.Session)
	}
}

func TestRecoverSessionInterruptedReadyRoundFailsOnlyPreparedSubsetWhenAskSiblingWasSkipped(t *testing.T) {
	boom := errors.New("boom:plan.update")
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := &nthFailingPlanUpdateStore{
		Store:            plan.NewMemoryStore(),
		updateErr:        boom,
		failOnUpdateCall: 1,
	}
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	audits := audit.NewMemoryStore()
	handler := &countingHandler{}

	tools.Register(
		tool.Definition{ToolName: "demo.ask-skip", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		messageHandler{},
	)
	tools.Register(
		tool.Definition{ToolName: "demo.allow-count", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true},
		handler,
	)

	opts := hruntime.Options{
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Tools:     tools,
		Verifiers: verifiers,
		Audit:     audits,
		Policy:    permission.DefaultEvaluator{},
	}

	rt1 := hruntime.New(opts).WithPolicyEvaluator(toolScopedAskPolicy{askTools: map[string]bool{"demo.ask-skip": true}})
	sess := mustCreateSession(t, rt1, "program ready round recovery ask subset", "fail close only prepared subset when ask sibling was skipped")
	tsk := mustCreateTask(t, rt1, task.Spec{TaskType: "demo", Goal: "fail close only prepared subset when ask sibling was skipped"})
	attached, err := rt1.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	if _, err := rt1.RunProgram(context.Background(), attached.SessionID, execution.Program{
		ProgramID: "prog_ready_round_recovery_ask_subset",
		Nodes: []execution.ProgramNode{
			{
				NodeID: "node_needs_approval",
				Action: action.Spec{ToolName: "demo.ask-skip", Args: map[string]any{"message": "approval"}},
			},
			{
				NodeID: "node_collect",
				Action: action.Spec{ToolName: "demo.allow-count"},
			},
		},
	}); !errors.Is(err, boom) {
		t.Fatalf("expected interrupted ready round to surface plan update error, got %v", err)
	}
	if handler.calls != 1 {
		t.Fatalf("expected only the allowed sibling to execute before interruption, got %d calls", handler.calls)
	}

	rt2 := hruntime.New(opts).WithPolicyEvaluator(toolScopedAskPolicy{askTools: map[string]bool{"demo.ask-skip": true}})
	recovered, err := rt2.RecoverSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("recover session: %v", err)
	}
	if handler.calls != 1 {
		t.Fatalf("expected recovery not to replay interrupted ready round, got %d calls", handler.calls)
	}
	if recovered.Session.Phase != session.PhaseFailed || recovered.Session.CurrentStepID != "prog_ready_round_recovery_ask_subset__node_collect" {
		t.Fatalf("expected recovery to fail-close on the actually prepared sibling, got %#v", recovered.Session)
	}

	latest := mustListPlans(t, rt2, attached.SessionID)
	stored := latest[len(latest)-1]
	statusByStep := map[string]plan.StepStatus{}
	for _, step := range stored.Steps {
		statusByStep[step.StepID] = step.Status
	}
	if statusByStep["prog_ready_round_recovery_ask_subset__node_collect"] != plan.StepFailed {
		t.Fatalf("expected prepared sibling to be failed on recovery, got %#v", stored.Steps)
	}
	if statusByStep["prog_ready_round_recovery_ask_subset__node_needs_approval"] != plan.StepPending {
		t.Fatalf("expected skipped approval sibling to remain pending, got %#v", stored.Steps)
	}
}

func TestRunSessionAttributesProgramReadyRoundFailureToCurrentRoundInsteadOfOlderRetryFailure(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(
		tool.Definition{ToolName: "demo.scripted-message", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true},
		scriptedMessageHandler{},
	)

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "program round attribution with older failure", "attribute current round failure instead of older retry failure")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "attribute current round failure instead of older retry failure"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	_, err = rt.CreatePlan(attached.SessionID, "program round attribution with older failure", []plan.StepSpec{
		{
			StepID:  "step_older_retry_failure",
			Title:   "older retry failure",
			Status:  plan.StepFailed,
			Attempt: 1,
			OnFail:  plan.OnFailSpec{Strategy: "retry"},
			Action:  action.Spec{ToolName: "demo.scripted-message", Args: map[string]any{"message": "ok"}},
		},
		{
			StepID: "prog_attr_older_failure__node_fail",
			Title:  "current round fail",
			Action: action.Spec{ToolName: "demo.scripted-message", Args: map[string]any{"message": "fail"}},
			OnFail: plan.OnFailSpec{Strategy: "abort"},
			Metadata: map[string]any{
				"program_group_id":                 "prog_attr_older_failure",
				execution.ProgramMetadataKeyNodeID: "node_fail",
			},
		},
		{
			StepID: "prog_attr_older_failure__node_ok",
			Title:  "current round ok",
			Action: action.Spec{ToolName: "demo.scripted-message", Args: map[string]any{"message": "ok"}},
			Metadata: map[string]any{
				"program_group_id":                 "prog_attr_older_failure",
				execution.ProgramMetadataKeyNodeID: "node_ok",
			},
		},
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	out, err := rt.RunSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("run session: %v", err)
	}
	if out.Session.Phase != session.PhaseFailed || out.Session.CurrentStepID != "prog_attr_older_failure__node_fail" {
		t.Fatalf("expected current round failing sibling to own final attribution, got %#v", out.Session)
	}

	events := mustListAuditEvents(t, rt, attached.SessionID)
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type != audit.EventStateChanged {
			continue
		}
		if to := fmt.Sprint(events[i].Payload["to"]); to == string(hruntime.TransitionFailed) {
			if events[i].StepID != "prog_attr_older_failure__node_fail" {
				t.Fatalf("expected final failed state-change event to point at current round failure, got %#v", events[i])
			}
			return
		}
	}
	t.Fatalf("expected final failed state-change event, got %#v", events)
}

func TestRunProgramAggregateVerifyCanSucceedAfterRepresentativeTargetActionFailure(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(
		tool.Definition{ToolName: "demo.target-error", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true},
		targetErrorHandler{failTargets: map[string]bool{"host-b": true}},
	)
	verifiers := verify.NewRegistry()
	verify.RegisterBuiltins(verifiers)

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verifiers,
		Policy:    permission.DefaultEvaluator{},
		LoopBudgets: func() hruntime.LoopBudgets {
			budgets := hruntime.DefaultLoopBudgets()
			budgets.MaxRetriesPerStep = 0
			return budgets
		}(),
	})

	sess := mustCreateSession(t, rt, "aggregate verify after target action failure", "aggregate verify can succeed after representative target action failure")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "aggregate verify can succeed after representative target action failure"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	out, err := rt.RunProgram(context.Background(), attached.SessionID, execution.Program{
		ProgramID: "prog_aggregate_after_action_failure",
		Nodes: []execution.ProgramNode{{
			NodeID:      "node_apply",
			Action:      action.Spec{ToolName: "demo.target-error"},
			VerifyScope: execution.VerificationScopeAggregate,
			Verify: &verify.Spec{
				Mode: verify.ModeAll,
				Checks: []verify.Check{
					{Kind: "value_equals", Args: map[string]any{"path": "result.data.status", "expected": string(execution.AggregateStatusPartialFailed)}},
					{Kind: "value_equals", Args: map[string]any{"path": "result.data.completed", "expected": 1}},
					{Kind: "value_equals", Args: map[string]any{"path": "result.data.failed", "expected": 1}},
				},
			},
			OnFail: &plan.OnFailSpec{Strategy: "abort"},
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
		t.Fatalf("expected aggregate summary verification to succeed despite representative target action failure, got %#v", out.Session)
	}

	verifications := mustListVerifications(t, rt, attached.SessionID)
	if len(verifications) != 2 {
		t.Fatalf("expected one verification per target execution, got %#v", verifications)
	}
	last := verifications[len(verifications)-1]
	if !last.Result.Success || last.Result.Reason == "aggregate pending" {
		t.Fatalf("expected resolved aggregate verification to succeed, got %#v", last)
	}
}

func TestRunProgramResolvesEarlierAggregateSiblingFailureAfterLaterSiblingSucceeds(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(
		tool.Definition{ToolName: "demo.target-error", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true},
		targetErrorHandler{failTargets: map[string]bool{"host-a": true}},
	)
	verifiers := verify.NewRegistry()
	verify.RegisterBuiltins(verifiers)

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verifiers,
		Policy:    permission.DefaultEvaluator{},
		LoopBudgets: func() hruntime.LoopBudgets {
			budgets := hruntime.DefaultLoopBudgets()
			budgets.MaxRetriesPerStep = 0
			return budgets
		}(),
	})

	sess := mustCreateSession(t, rt, "aggregate verify resolves earlier sibling failure", "resolve earlier aggregate sibling failures after a later sibling succeeds")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "resolve earlier aggregate sibling failures after a later sibling succeeds"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	out, err := rt.RunProgram(context.Background(), attached.SessionID, execution.Program{
		ProgramID: "prog_aggregate_first_target_failure",
		Nodes: []execution.ProgramNode{{
			NodeID:      "node_apply",
			Action:      action.Spec{ToolName: "demo.target-error"},
			VerifyScope: execution.VerificationScopeAggregate,
			Verify: &verify.Spec{
				Mode: verify.ModeAll,
				Checks: []verify.Check{
					{Kind: "value_equals", Args: map[string]any{"path": "result.data.status", "expected": string(execution.AggregateStatusPartialFailed)}},
					{Kind: "value_equals", Args: map[string]any{"path": "result.data.completed", "expected": 1}},
					{Kind: "value_equals", Args: map[string]any{"path": "result.data.failed", "expected": 1}},
				},
			},
			OnFail: &plan.OnFailSpec{Strategy: "abort"},
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
		t.Fatalf("expected aggregate summary verification to complete the session, got %#v", out.Session)
	}
	if len(out.Aggregates) != 1 || out.Aggregates[0].Status != execution.AggregateStatusPartialFailed || out.Aggregates[0].Completed != 1 || out.Aggregates[0].Failed != 1 {
		t.Fatalf("expected partial_failed aggregate to remain visible after sibling resolution, got %#v", out.Aggregates)
	}
	executionByStep := map[string]hruntime.StepRunOutput{}
	for _, executionOut := range out.Executions {
		executionByStep[executionOut.Execution.Step.StepID] = executionOut
	}
	hostAFinal, ok := executionByStep["prog_aggregate_first_target_failure__node_apply__host-a"]
	if !ok {
		t.Fatalf("expected host-a execution output, got %#v", out.Executions)
	}
	if hostAFinal.Execution.Step.Status != plan.StepCompleted {
		t.Fatalf("expected host-a execution step to converge to completed, got %#v", hostAFinal.Execution.Step)
	}
	if !hostAFinal.Execution.Verify.Success || hostAFinal.Execution.Verify.Reason == "aggregate pending" {
		t.Fatalf("expected host-a execution verify result to converge to the resolved aggregate result, got %#v", hostAFinal.Execution.Verify)
	}

	latestPlans := mustListPlans(t, rt, attached.SessionID)
	if len(latestPlans) == 0 {
		t.Fatalf("expected stored plans, got none")
	}
	latest := latestPlans[len(latestPlans)-1]
	statusByStep := map[string]plan.StepStatus{}
	for _, step := range latest.Steps {
		statusByStep[step.StepID] = step.Status
	}
	if statusByStep["prog_aggregate_first_target_failure__node_apply__host-a"] != plan.StepCompleted {
		t.Fatalf("expected earlier failed sibling to converge to completed after aggregate verify success, got %#v", latest.Steps)
	}
	if statusByStep["prog_aggregate_first_target_failure__node_apply__host-b"] != plan.StepCompleted {
		t.Fatalf("expected later successful sibling to stay completed, got %#v", latest.Steps)
	}

	attempts := mustListAttempts(t, rt, attached.SessionID)
	attemptByStep := map[string]execution.Attempt{}
	for _, attempt := range attempts {
		attemptByStep[attempt.StepID] = attempt
	}
	hostAAttempt, ok := attemptByStep["prog_aggregate_first_target_failure__node_apply__host-a"]
	if !ok {
		t.Fatalf("expected persisted attempt for host-a, got %#v", attempts)
	}
	if hostAAttempt.Status != execution.AttemptCompleted || hostAAttempt.Step.Status != plan.StepCompleted {
		t.Fatalf("expected host-a attempt to converge to completed after aggregate verify success, got %#v", hostAAttempt)
	}

	verifications := mustListVerifications(t, rt, attached.SessionID)
	verificationByStep := map[string]execution.VerificationRecord{}
	for _, record := range verifications {
		verificationByStep[record.StepID] = record
	}
	hostAVerification, ok := verificationByStep["prog_aggregate_first_target_failure__node_apply__host-a"]
	if !ok {
		t.Fatalf("expected persisted verification for host-a, got %#v", verifications)
	}
	if hostAVerification.Status != execution.VerificationCompleted || !hostAVerification.Result.Success || hostAVerification.Result.Reason == "target failed" {
		t.Fatalf("expected host-a verification to converge to aggregate success after sibling resolution, got %#v", hostAVerification)
	}
}

func TestRunSessionDoesNotExecutePendingProgramNodeBeforeDependenciesComplete(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.message", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, messageHandler{})

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "program dependency gate", "do not run pending program nodes before their dependencies complete")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "gate pending program nodes on dependency completion"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	created, err := rt.CreatePlan(attached.SessionID, "program dependency gate", []plan.StepSpec{
		{
			StepID: "node_prepare",
			Title:  "prepare",
			Status: plan.StepFailed,
			OnFail: plan.OnFailSpec{Strategy: "abort"},
			Metadata: map[string]any{
				"program_group_id":                 "manual_group",
				execution.ProgramMetadataKeyNodeID: "node_prepare",
			},
		},
		{
			StepID: "node_apply",
			Title:  "apply",
			Action: action.Spec{ToolName: "demo.message", Args: map[string]any{"message": "apply"}},
			Metadata: map[string]any{
				"program_group_id":                 "manual_group",
				execution.ProgramMetadataKeyNodeID: "node_apply",
				"program_depends_on":               []string{"node_prepare"},
			},
		},
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	out, err := rt.RunSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("run session: %v", err)
	}
	if len(out.Executions) != 0 {
		t.Fatalf("expected no execution while dependency remains incomplete, got %#v", out.Executions)
	}
	if out.Session.Phase != session.PhaseFailed {
		t.Fatalf("expected dependency-gated session to surface the blocking failure, got %#v", out.Session)
	}
	if actions := mustListActions(t, rt, attached.SessionID); len(actions) != 0 {
		t.Fatalf("expected no action records while dependency remains incomplete, got %#v", actions)
	}
	stored, err := rt.GetSession(attached.SessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if stored.Phase != session.PhaseFailed {
		t.Fatalf("expected stored session to be failed after dependency gate reconciliation, got %#v", stored)
	}
	if latest := mustListPlans(t, rt, attached.SessionID); len(latest) == 0 {
		t.Fatalf("expected stored plan revisions, got none")
	} else {
		planState := latest[len(latest)-1]
		if len(planState.Steps) != len(created.Steps) {
			t.Fatalf("expected stored plan to retain both steps, got %#v", planState.Steps)
		}
		if planState.Status != plan.StatusFailed {
			t.Fatalf("expected plan status failed after dependency gate reconciliation, got %#v", planState)
		}
		if planState.Steps[1].Status != plan.StepPending {
			t.Fatalf("expected dependent program step to remain pending, got %#v", planState.Steps[1])
		}
	}
}

func TestRunSessionDoesNotExecuteLegacyProgramNodeBeforeDependenciesComplete(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.message", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, messageHandler{})

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "legacy program dependency gate", "do not run legacy pending program nodes before their dependencies complete")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "gate legacy pending program nodes on dependency completion"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	_, err = rt.CreatePlan(attached.SessionID, "legacy program dependency gate", []plan.StepSpec{
		{
			StepID: "node_prepare",
			Title:  "prepare",
			Status: plan.StepFailed,
			OnFail: plan.OnFailSpec{Strategy: "abort"},
			Metadata: map[string]any{
				execution.ProgramMetadataKeyID:     "legacy_prog",
				execution.ProgramMetadataKeyNodeID: "node_prepare",
			},
		},
		{
			StepID: "node_apply",
			Title:  "apply",
			Action: action.Spec{ToolName: "demo.message", Args: map[string]any{"message": "apply"}},
			Metadata: map[string]any{
				execution.ProgramMetadataKeyID:     "legacy_prog",
				execution.ProgramMetadataKeyNodeID: "node_apply",
				"program_depends_on":               []string{"node_prepare"},
			},
		},
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	out, err := rt.RunSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("run session: %v", err)
	}
	if len(out.Executions) != 0 {
		t.Fatalf("expected no execution for legacy metadata while dependency remains incomplete, got %#v", out.Executions)
	}
	if out.Session.Phase != session.PhaseFailed {
		t.Fatalf("expected legacy dependency-gated session to surface the blocking failure, got %#v", out.Session)
	}
	if actions := mustListActions(t, rt, attached.SessionID); len(actions) != 0 {
		t.Fatalf("expected no action records for legacy metadata while dependency remains incomplete, got %#v", actions)
	}
	stored, err := rt.GetSession(attached.SessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if stored.Phase != session.PhaseFailed {
		t.Fatalf("expected stored session to be failed for legacy dependency gate reconciliation, got %#v", stored)
	}
	if latest := mustListPlans(t, rt, attached.SessionID); len(latest) == 0 {
		t.Fatalf("expected stored plan revisions, got none")
	} else if latest[len(latest)-1].Status != plan.StatusFailed {
		t.Fatalf("expected legacy plan status failed after dependency gate reconciliation, got %#v", latest[len(latest)-1])
	}
}

func TestRunSessionFailsWhenExhaustedContinueDependencyBlocksProgramNode(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.message", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, messageHandler{})

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})
	exhaustedAttempt := hruntime.DefaultLoopBudgets().MaxRetriesPerStep + 1

	sess := mustCreateSession(t, rt, "continue dependency gate", "fail when exhausted continue dependency blocks a program node")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "fail when exhausted continue dependency blocks a program node"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	_, err = rt.CreatePlan(attached.SessionID, "continue dependency gate", []plan.StepSpec{
		{
			StepID:  "node_prepare",
			Title:   "prepare",
			Status:  plan.StepFailed,
			Attempt: exhaustedAttempt,
			OnFail:  plan.OnFailSpec{Strategy: "continue"},
			Metadata: map[string]any{
				"program_group_id":                 "continue_group",
				execution.ProgramMetadataKeyNodeID: "node_prepare",
			},
		},
		{
			StepID: "node_apply",
			Title:  "apply",
			Action: action.Spec{ToolName: "demo.message", Args: map[string]any{"message": "apply"}},
			Metadata: map[string]any{
				"program_group_id":                 "continue_group",
				execution.ProgramMetadataKeyNodeID: "node_apply",
				"program_depends_on":               []string{"node_prepare"},
			},
		},
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	out, err := rt.RunSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("run session: %v", err)
	}
	if len(out.Executions) != 0 {
		t.Fatalf("expected no execution when exhausted continue dependency blocks the program node, got %#v", out.Executions)
	}
	if out.Session.Phase != session.PhaseFailed {
		t.Fatalf("expected exhausted continue dependency to fail the session, got %#v", out.Session)
	}
	stored, err := rt.GetSession(attached.SessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if stored.Phase != session.PhaseFailed {
		t.Fatalf("expected stored session to be failed after continue-deadlock reconciliation, got %#v", stored)
	}
	if latest := mustListPlans(t, rt, attached.SessionID); len(latest) == 0 {
		t.Fatalf("expected stored plan revisions, got none")
	} else if latest[len(latest)-1].Status != plan.StatusFailed {
		t.Fatalf("expected continue-deadlock plan status failed after reconciliation, got %#v", latest[len(latest)-1])
	}
}

func TestRunSessionFailsWhenProgramDependencyNodeIsMissing(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.message", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, messageHandler{})

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "missing dependency gate", "fail when a program dependency node is missing")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "fail when a program dependency node is missing"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	_, err = rt.CreatePlan(attached.SessionID, "missing dependency gate", []plan.StepSpec{
		{
			StepID: "node_apply",
			Title:  "apply",
			Action: action.Spec{ToolName: "demo.message", Args: map[string]any{"message": "apply"}},
			Metadata: map[string]any{
				"program_group_id":                 "missing_group",
				execution.ProgramMetadataKeyNodeID: "node_apply",
				"program_depends_on":               []string{"node_missing"},
			},
		},
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	out, err := rt.RunSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("run session: %v", err)
	}
	if len(out.Executions) != 0 {
		t.Fatalf("expected no execution when the dependency node is missing, got %#v", out.Executions)
	}
	if out.Session.Phase != session.PhaseFailed {
		t.Fatalf("expected missing dependency node to fail the session, got %#v", out.Session)
	}
	if latest := mustListPlans(t, rt, attached.SessionID); len(latest) == 0 {
		t.Fatalf("expected stored plan revisions, got none")
	} else if latest[len(latest)-1].Status != plan.StatusFailed {
		t.Fatalf("expected missing-dependency plan status failed after reconciliation, got %#v", latest[len(latest)-1])
	}
}

func TestRunSessionAttributesDependencyDeadlockFailureToBlockedProgramStep(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.message", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, messageHandler{})

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "dependency deadlock attribution", "attribute dependency deadlock failure to the blocked program step")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "attribute dependency deadlock failure to the blocked program step"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	_, err = rt.CreatePlan(attached.SessionID, "dependency deadlock attribution", []plan.StepSpec{
		{
			StepID: "node_apply",
			Title:  "apply",
			Action: action.Spec{ToolName: "demo.message", Args: map[string]any{"message": "apply"}},
			Metadata: map[string]any{
				"program_group_id":                 "deadlock_group",
				execution.ProgramMetadataKeyNodeID: "node_apply",
				"program_depends_on":               []string{"node_missing"},
			},
		},
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	out, err := rt.RunSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("run session: %v", err)
	}
	if out.Session.Phase != session.PhaseFailed {
		t.Fatalf("expected missing dependency to fail the session, got %#v", out.Session)
	}
	if out.Session.CurrentStepID != "node_apply" {
		t.Fatalf("expected deadlock attribution to point at the blocked program step, got %#v", out.Session)
	}

	events := mustListAuditEvents(t, rt, attached.SessionID)
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type != audit.EventStateChanged {
			continue
		}
		if to := fmt.Sprint(events[i].Payload["to"]); to == string(hruntime.TransitionFailed) {
			if events[i].StepID != "node_apply" {
				t.Fatalf("expected final failed state-change event to point at the blocked program step, got %#v", events[i])
			}
			return
		}
	}
	t.Fatalf("expected final failed state-change event, got %#v", events)
}

func TestRunSessionAttributesDependencyDeadlockFailureToBlockedProgramStepWhenCurrentStepIsStale(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.message", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, messageHandler{})

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "dependency deadlock stale attribution", "attribute dependency deadlock to the blocked step instead of a stale completed step")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "attribute dependency deadlock to the blocked step instead of a stale completed step"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	_, err = rt.CreatePlan(attached.SessionID, "dependency deadlock stale attribution", []plan.StepSpec{
		{
			StepID: "node_prepare",
			Title:  "prepare",
			Status: plan.StepCompleted,
			Action: action.Spec{ToolName: "demo.message", Args: map[string]any{"message": "prepare"}},
			Metadata: map[string]any{
				"program_group_id":                 "deadlock_group_stale",
				execution.ProgramMetadataKeyNodeID: "node_prepare",
			},
		},
		{
			StepID: "node_apply",
			Title:  "apply",
			Action: action.Spec{ToolName: "demo.message", Args: map[string]any{"message": "apply"}},
			Metadata: map[string]any{
				"program_group_id":                 "deadlock_group_stale",
				execution.ProgramMetadataKeyNodeID: "node_apply",
				"program_depends_on":               []string{"node_missing"},
			},
		},
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	stored, err := rt.GetSession(attached.SessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	stored.CurrentStepID = "node_prepare"
	stored.Version++
	if err := rt.Sessions.Update(stored); err != nil {
		t.Fatalf("update session with stale current step: %v", err)
	}

	out, err := rt.RunSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("run session: %v", err)
	}
	if out.Session.Phase != session.PhaseFailed {
		t.Fatalf("expected dependency deadlock with stale current step to fail the session, got %#v", out.Session)
	}
	if out.Session.CurrentStepID != "node_apply" {
		t.Fatalf("expected stale current step to be ignored in favor of the blocked program step, got %#v", out.Session)
	}

	events := mustListAuditEvents(t, rt, attached.SessionID)
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type != audit.EventStateChanged {
			continue
		}
		if to := fmt.Sprint(events[i].Payload["to"]); to == string(hruntime.TransitionFailed) {
			if events[i].StepID != "node_apply" {
				t.Fatalf("expected final failed state-change event to point at the blocked program step instead of the stale completed step, got %#v", events[i])
			}
			return
		}
	}
	t.Fatalf("expected final failed state-change event, got %#v", events)
}

func TestRunSessionAttributesDependencyDeadlockFailureToBlockedProgramStepWhenEarlierContinueFailureIsExhausted(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.message", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, messageHandler{})

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
		LoopBudgets: func() hruntime.LoopBudgets {
			budgets := hruntime.DefaultLoopBudgets()
			budgets.MaxRetriesPerStep = 0
			return budgets
		}(),
	})

	sess := mustCreateSession(t, rt, "dependency deadlock exhausted continue attribution", "attribute dependency deadlock to the blocked step instead of an exhausted continue failure")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "attribute dependency deadlock to the blocked step instead of an exhausted continue failure"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	_, err = rt.CreatePlan(attached.SessionID, "dependency deadlock exhausted continue attribution", []plan.StepSpec{
		{
			StepID:  "node_failed",
			Title:   "failed",
			Status:  plan.StepFailed,
			Attempt: 1,
			OnFail:  plan.OnFailSpec{Strategy: "continue"},
			Action:  action.Spec{ToolName: "demo.message", Args: map[string]any{"message": "failed"}},
			Metadata: map[string]any{
				"program_group_id":                 "deadlock_group_exhausted",
				execution.ProgramMetadataKeyNodeID: "node_failed",
			},
		},
		{
			StepID: "node_apply",
			Title:  "apply",
			Action: action.Spec{ToolName: "demo.message", Args: map[string]any{"message": "apply"}},
			Metadata: map[string]any{
				"program_group_id":                 "deadlock_group_exhausted",
				execution.ProgramMetadataKeyNodeID: "node_apply",
				"program_depends_on":               []string{"node_missing"},
			},
		},
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	out, err := rt.RunSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("run session: %v", err)
	}
	if out.Session.Phase != session.PhaseFailed {
		t.Fatalf("expected dependency deadlock to fail the session, got %#v", out.Session)
	}
	if out.Session.CurrentStepID != "node_apply" {
		t.Fatalf("expected exhausted continue failure to be skipped in favor of the blocked program step, got %#v", out.Session)
	}

	events := mustListAuditEvents(t, rt, attached.SessionID)
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type != audit.EventStateChanged {
			continue
		}
		if to := fmt.Sprint(events[i].Payload["to"]); to == string(hruntime.TransitionFailed) {
			if events[i].StepID != "node_apply" {
				t.Fatalf("expected final failed state-change event to point at the blocked program step instead of the exhausted continue failure, got %#v", events[i])
			}
			return
		}
	}
	t.Fatalf("expected final failed state-change event, got %#v", events)
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

func TestRunProgramResolvesTargetScopedOutputRefsFromRawActionResultsWhenInlineTrimmed(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.target-long", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, targetLongOutputHandler{
		outputs: map[string]string{
			"host-a": "host-a-" + strings.Repeat("x", 12) + "-tail-a",
			"host-b": "host-b-" + strings.Repeat("y", 12) + "-tail-b",
		},
	})
	tools.Register(tool.Definition{ToolName: "demo.inspect-arg", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, inspectArgHandler{})

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
		LoopBudgets: hruntime.LoopBudgets{
			MaxSteps:           8,
			MaxRetriesPerStep:  3,
			MaxPlanRevisions:   8,
			MaxTotalRuntimeMS:  60000,
			MaxToolOutputChars: 8,
		},
	})

	sess := mustCreateSession(t, rt, "program target raw refs", "resolve target scoped refs from raw action output")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "resolve target scoped refs from raw action output"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	expected := map[string]string{
		"host-a": "host-a-" + strings.Repeat("x", 12) + "-tail-a",
		"host-b": "host-b-" + strings.Repeat("y", 12) + "-tail-b",
	}

	out, err := rt.RunProgram(context.Background(), attached.SessionID, execution.Program{
		ProgramID: "prog_target_raw_refs",
		Nodes: []execution.ProgramNode{
			{
				NodeID: "node_prepare",
				Action: action.Spec{ToolName: "demo.target-long"},
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
				Action:    action.Spec{ToolName: "demo.inspect-arg"},
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
	resolvedLengths := map[string]int{}
	resolvedSuffixes := map[string]string{}
	for _, record := range actions {
		nodeID, _ := record.Metadata[execution.ProgramMetadataKeyNodeID].(string)
		if nodeID != "node_apply" {
			continue
		}
		target, ok := execution.TargetRefFromMetadata(record.Metadata)
		if !ok {
			t.Fatalf("expected target metadata on target-scoped action, got %#v", record)
		}
		length, _ := record.Result.Data["message_length"].(int)
		suffix, _ := record.Result.Data["message_suffix"].(string)
		resolvedLengths[target.TargetID] = length
		resolvedSuffixes[target.TargetID] = suffix
	}
	if len(resolvedLengths) != len(expected) {
		t.Fatalf("expected resolved results for every target, got %#v", resolvedLengths)
	}
	for targetID, text := range expected {
		if resolvedLengths[targetID] != len(text) {
			t.Fatalf("expected target %s to receive full raw text length %d, got %#v", targetID, len(text), resolvedLengths)
		}
		if resolvedSuffixes[targetID] != text[len(text)-6:] {
			t.Fatalf("expected target %s to receive raw tail %q, got %#v", targetID, text[len(text)-6:], resolvedSuffixes)
		}
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

func TestRunProgramResolvesRuntimeHandleRefsIntoLaterStepArgs(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.handle.producer", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, runtimeHandleProducerHandler{})
	tools.Register(tool.Definition{ToolName: "demo.handle-ref", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, runtimeHandleRefHandler{})

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "program runtime handle refs", "resolve runtime handle refs")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "resolve runtime handle refs"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	out, err := rt.RunProgram(context.Background(), attached.SessionID, execution.Program{
		ProgramID: "prog_runtime_handle_refs",
		Nodes: []execution.ProgramNode{
			{
				NodeID: "node_start",
				Action: action.Spec{ToolName: "demo.handle.producer"},
			},
			{
				NodeID:    "node_use",
				Action:    action.Spec{ToolName: "demo.handle-ref"},
				DependsOn: []string{"node_start"},
				InputBinds: []execution.ProgramInputBinding{{
					Name: "handle",
					Kind: execution.ProgramInputBindingRuntimeHandleRef,
					RuntimeHandle: &execution.RuntimeHandleRef{
						StepID: "node_start",
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
		if nodeID == "node_use" {
			apply = record.Result
			break
		}
	}
	if got, _ := apply.Data["handle_id"].(string); got != "hdl_node_start" {
		t.Fatalf("expected resolved runtime handle id, got %#v", apply)
	}
	if got, _ := apply.Data["kind"].(string); got != "pty" {
		t.Fatalf("expected resolved runtime handle kind, got %#v", apply)
	}
	if got, _ := apply.Data["arg_type"].(string); got != "typed" {
		t.Fatalf("expected typed runtime handle ref arg, got %#v", apply)
	}
}

func TestRunProgramResolvesTargetScopedRuntimeHandleRefsPerTarget(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.handle.producer", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, runtimeHandleProducerHandler{})
	tools.Register(tool.Definition{ToolName: "demo.handle-ref", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, runtimeHandleRefHandler{})

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "program target runtime handle refs", "resolve target scoped runtime handle refs")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "resolve target scoped runtime handle refs"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	out, err := rt.RunProgram(context.Background(), attached.SessionID, execution.Program{
		ProgramID: "prog_target_runtime_handle_refs",
		Nodes: []execution.ProgramNode{
			{
				NodeID: "node_start",
				Action: action.Spec{ToolName: "demo.handle.producer"},
				Targeting: &execution.TargetSelection{
					Mode: execution.TargetSelectionFanoutExplicit,
					Targets: []execution.Target{
						{TargetID: "host-a", Kind: "host"},
						{TargetID: "host-b", Kind: "host"},
					},
				},
			},
			{
				NodeID:    "node_use",
				Action:    action.Spec{ToolName: "demo.handle-ref"},
				DependsOn: []string{"node_start"},
				Targeting: &execution.TargetSelection{
					Mode: execution.TargetSelectionFanoutExplicit,
					Targets: []execution.Target{
						{TargetID: "host-a", Kind: "host"},
						{TargetID: "host-b", Kind: "host"},
					},
				},
				InputBinds: []execution.ProgramInputBinding{{
					Name: "handle",
					Kind: execution.ProgramInputBindingRuntimeHandleRef,
					RuntimeHandle: &execution.RuntimeHandleRef{
						StepID: "node_start",
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
		if nodeID != "node_use" {
			continue
		}
		target, ok := execution.TargetRefFromMetadata(record.Metadata)
		if !ok {
			t.Fatalf("expected target metadata on runtime handle consumer action, got %#v", record)
		}
		handleID, _ := record.Result.Data["handle_id"].(string)
		resolved[target.TargetID] = handleID
	}
	if resolved["host-a"] != "hdl_host-a" || resolved["host-b"] != "hdl_host-b" {
		t.Fatalf("expected per-target runtime handle ref resolution, got %#v", resolved)
	}
}

func TestRunProgramSupportsMixedShellToolArtifactAndInteractiveOperations(t *testing.T) {
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	manager := shellmodule.NewPTYManager(shellmodule.PTYManagerOptions{})
	t.Cleanup(func() {
		_ = manager.CloseAll(context.Background(), "test cleanup")
	})
	shellmodule.RegisterWithOptions(tools, verifiers, shellmodule.Options{PTYManager: manager})
	tools.Register(tool.Definition{ToolName: "demo.mixed.interop", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, mixedInteropHandler{})
	tools.Register(tool.Definition{ToolName: "demo.artifact-ref", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, artifactRefHandler{})

	rt := hruntime.New(hruntime.Options{
		Tools:                 tools,
		Verifiers:             verifiers,
		Policy:                permission.RulesEvaluator{Rules: []permission.Rule{{Permission: "shell.exec", Pattern: "mode=pty", Action: permission.Allow}}, Fallback: permission.DefaultEvaluator{}},
		InteractiveController: shellmodule.NewInteractiveController(manager),
	})

	sess := mustCreateSession(t, rt, "program mixed operations", "support shell tool artifact and interactive operations in one program")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "support shell tool artifact and interactive operations in one program"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	out, err := rt.RunProgram(context.Background(), attached.SessionID, execution.Program{
		ProgramID: "prog_mixed_operations",
		Nodes: []execution.ProgramNode{
			{
				NodeID: "node_shell_pipe",
				Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "printf shell-pipe", "timeout_ms": 5000}},
			},
			{
				NodeID: "node_shell_pty",
				Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pty", "command": "cat", "timeout_ms": 5000}},
			},
			{
				NodeID:    "node_tool",
				Action:    action.Spec{ToolName: "demo.mixed.interop"},
				DependsOn: []string{"node_shell_pipe", "node_shell_pty"},
				InputBinds: []execution.ProgramInputBinding{
					{
						Name: "message",
						Kind: execution.ProgramInputBindingOutputRef,
						Ref: &execution.OutputRef{
							Kind:   execution.OutputRefText,
							StepID: "node_shell_pipe",
						},
					},
					{
						Name: "handle",
						Kind: execution.ProgramInputBindingRuntimeHandleRef,
						RuntimeHandle: &execution.RuntimeHandleRef{
							StepID: "node_shell_pty",
						},
					},
				},
			},
			{
				NodeID:    "node_artifact",
				Action:    action.Spec{ToolName: "demo.artifact-ref"},
				DependsOn: []string{"node_tool"},
				InputBinds: []execution.ProgramInputBinding{{
					Name: "artifact",
					Kind: execution.ProgramInputBindingOutputRef,
					Ref: &execution.OutputRef{
						Kind:   execution.OutputRefArtifact,
						StepID: "node_tool",
					},
				}},
			},
			{
				NodeID:    "node_view",
				Action:    action.Spec{ToolName: hruntime.ProgramInteractiveViewToolName, Args: map[string]any{"offset": int64(0), "max_bytes": 64}},
				DependsOn: []string{"node_tool"},
				InputBinds: []execution.ProgramInputBinding{{
					Name: "handle",
					Kind: execution.ProgramInputBindingRuntimeHandleRef,
					RuntimeHandle: &execution.RuntimeHandleRef{
						StepID: "node_shell_pty",
					},
				}},
			},
			{
				NodeID:    "node_write",
				Action:    action.Spec{ToolName: hruntime.ProgramInteractiveWriteToolName, Args: map[string]any{"input": "status\n"}},
				DependsOn: []string{"node_view"},
				InputBinds: []execution.ProgramInputBinding{{
					Name: "handle",
					Kind: execution.ProgramInputBindingRuntimeHandleRef,
					RuntimeHandle: &execution.RuntimeHandleRef{
						StepID: "node_shell_pty",
					},
				}},
			},
			{
				NodeID:    "node_close",
				Action:    action.Spec{ToolName: hruntime.ProgramInteractiveCloseToolName, Args: map[string]any{"reason": "mixed program done"}},
				DependsOn: []string{"node_write", "node_artifact"},
				InputBinds: []execution.ProgramInputBinding{{
					Name: "handle",
					Kind: execution.ProgramInputBindingRuntimeHandleRef,
					RuntimeHandle: &execution.RuntimeHandleRef{
						StepID: "node_shell_pty",
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
	if len(out.Executions) != 7 {
		t.Fatalf("expected seven raw executions, got %#v", out.Executions)
	}

	results := map[string]action.Result{}
	for _, item := range out.Executions {
		nodeID, _ := item.Execution.Step.Metadata[execution.ProgramMetadataKeyNodeID].(string)
		results[nodeID] = item.Execution.Action
	}
	if len(results) != 7 {
		t.Fatalf("expected seven unique node results, got %#v", out.Executions)
	}

	if got, _ := results["node_tool"].Data["message"].(string); got != "shell-pipe" {
		t.Fatalf("expected mixed tool to consume shell output ref, got %#v", results["node_tool"].Data)
	}
	handleID, _ := results["node_tool"].Data["handle_id"].(string)
	if handleID == "" {
		t.Fatalf("expected mixed tool to consume typed runtime handle ref, got %#v", results["node_tool"].Data)
	}
	if got, _ := results["node_artifact"].Data["artifact_id"].(string); got == "" {
		t.Fatalf("expected artifact consumer to receive typed artifact ref, got %#v", results["node_artifact"].Data)
	}
	if viewed, ok := results["node_view"].Data["runtime_handle"].(execution.RuntimeHandle); !ok || viewed.HandleID != handleID || viewed.Status != execution.RuntimeHandleActive {
		t.Fatalf("expected interactive view to preserve PTY handle identity, got %#v", results["node_view"].Data)
	}
	if written, ok := results["node_write"].Data["runtime_handle"].(execution.RuntimeHandle); !ok || written.HandleID != handleID || written.Status != execution.RuntimeHandleActive {
		t.Fatalf("expected interactive write to preserve PTY handle identity, got %#v", results["node_write"].Data)
	}
	if closed, ok := results["node_close"].Data["runtime_handle"].(execution.RuntimeHandle); !ok || closed.HandleID != handleID || closed.Status != execution.RuntimeHandleClosed {
		t.Fatalf("expected interactive close to close the PTY handle, got %#v", results["node_close"].Data)
	}

	handles := mustListRuntimeHandles(t, rt, attached.SessionID)
	if len(handles) != 1 || handles[0].HandleID != handleID || handles[0].Status != execution.RuntimeHandleClosed {
		t.Fatalf("expected one closed mixed-program runtime handle, got %#v", handles)
	}
	artifacts := mustListArtifacts(t, rt, attached.SessionID)
	if len(artifacts) < 4 {
		t.Fatalf("expected mixed program to persist multiple action-result artifacts, got %#v", artifacts)
	}
}

func TestRunProgramRejectsCrossTargetRuntimeHandleRefFallback(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.handle.producer", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, runtimeHandleProducerHandler{
		skipTargets: map[string]bool{"host-b": true},
	})
	tools.Register(tool.Definition{ToolName: "demo.handle-ref", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, runtimeHandleRefHandler{})

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "program reject cross target runtime handle ref", "reject cross target runtime handle fallback")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "reject cross target runtime handle fallback"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	_, err = rt.RunProgram(context.Background(), attached.SessionID, execution.Program{
		ProgramID: "prog_reject_cross_target_runtime_handle_ref",
		Nodes: []execution.ProgramNode{
			{
				NodeID: "node_start",
				Action: action.Spec{ToolName: "demo.handle.producer"},
				Targeting: &execution.TargetSelection{
					Mode: execution.TargetSelectionFanoutExplicit,
					Targets: []execution.Target{
						{TargetID: "host-a", Kind: "host"},
						{TargetID: "host-b", Kind: "host"},
					},
				},
			},
			{
				NodeID:    "node_use",
				Action:    action.Spec{ToolName: "demo.handle-ref"},
				DependsOn: []string{"node_start"},
				Targeting: &execution.TargetSelection{
					Mode: execution.TargetSelectionFanoutExplicit,
					Targets: []execution.Target{
						{TargetID: "host-a", Kind: "host"},
						{TargetID: "host-b", Kind: "host"},
					},
				},
				InputBinds: []execution.ProgramInputBinding{{
					Name: "handle",
					Kind: execution.ProgramInputBindingRuntimeHandleRef,
					RuntimeHandle: &execution.RuntimeHandleRef{
						StepID: "node_start",
					},
				}},
			},
		},
	})
	if !errors.Is(err, hruntime.ErrProgramBindingResolveFailed) {
		t.Fatalf("expected missing target-local runtime handle to fail binding resolution, got %v", err)
	}
}

func TestRunProgramRejectsRuntimeHandleRefContractMismatches(t *testing.T) {
	cases := []struct {
		name string
		ref  execution.RuntimeHandleRef
	}{
		{
			name: "kind mismatch",
			ref: execution.RuntimeHandleRef{
				StepID: "node_start",
				Kind:   "ssh",
			},
		},
		{
			name: "status mismatch",
			ref: execution.RuntimeHandleRef{
				StepID: "node_start",
				Status: execution.RuntimeHandleClosed,
			},
		},
		{
			name: "version mismatch",
			ref: execution.RuntimeHandleRef{
				StepID:  "node_start",
				Version: 99,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tools := tool.NewRegistry()
			tools.Register(tool.Definition{ToolName: "demo.handle.producer", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, runtimeHandleProducerHandler{})
			tools.Register(tool.Definition{ToolName: "demo.handle-ref", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, runtimeHandleRefHandler{})

			rt := hruntime.New(hruntime.Options{
				Tools:     tools,
				Verifiers: verify.NewRegistry(),
				Policy:    permission.DefaultEvaluator{},
			})

			sess := mustCreateSession(t, rt, "program reject runtime handle ref mismatch", "reject runtime handle ref mismatch")
			tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "reject runtime handle ref mismatch"})
			attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
			if err != nil {
				t.Fatalf("attach task: %v", err)
			}

			_, err = rt.RunProgram(context.Background(), attached.SessionID, execution.Program{
				ProgramID: "prog_reject_runtime_handle_ref_mismatch",
				Nodes: []execution.ProgramNode{
					{
						NodeID: "node_start",
						Action: action.Spec{ToolName: "demo.handle.producer"},
					},
					{
						NodeID:    "node_use",
						Action:    action.Spec{ToolName: "demo.handle-ref"},
						DependsOn: []string{"node_start"},
						InputBinds: []execution.ProgramInputBinding{{
							Name:          "handle",
							Kind:          execution.ProgramInputBindingRuntimeHandleRef,
							RuntimeHandle: &tc.ref,
						}},
					},
				},
			})
			if !errors.Is(err, hruntime.ErrProgramBindingResolveFailed) {
				t.Fatalf("expected runtime handle ref mismatch to fail binding resolution, got %v", err)
			}
		})
	}
}

func TestRunProgramSupportsNativeInteractiveLifecycleActions(t *testing.T) {
	controller := &stubInteractiveController{}
	verifiers := verify.NewRegistry()
	shellmodule.RegisterWithOptions(nil, verifiers, shellmodule.Options{
		PTYInspector: programTestPTYInspector{
			inspect: map[string]shellmodule.PTYInspectResult{
				"hdl_program_native": {Status: "active"},
			},
			read: map[string]shellmodule.PTYReadResult{
				"hdl_program_native": {
					Status:     "active",
					Data:       "hello from verifier",
					NextOffset: 19,
				},
			},
		},
	})
	rt := hruntime.New(hruntime.Options{
		InteractiveController: controller,
		Verifiers:             verifiers,
		Policy:                permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "program interactive lifecycle", "run interactive lifecycle natively inside program")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "run interactive lifecycle natively inside program"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	out, err := rt.RunProgram(context.Background(), attached.SessionID, execution.Program{
		ProgramID: "prog_native_interactive",
		Nodes: []execution.ProgramNode{
			{
				NodeID: "node_start",
				Action: action.Spec{
					ToolName: hruntime.ProgramInteractiveStartToolName,
					Args: map[string]any{
						"handle_id": "hdl_program_native",
						"kind":      "stub",
						"spec":      map[string]any{"command": "demo"},
						"metadata": map[string]any{
							"origin": "program",
						},
					},
				},
			},
			{
				NodeID:    "node_view",
				Action:    action.Spec{ToolName: hruntime.ProgramInteractiveViewToolName, Args: map[string]any{"offset": int64(0), "max_bytes": 32}},
				DependsOn: []string{"node_start"},
				InputBinds: []execution.ProgramInputBinding{{
					Name: "handle",
					Kind: execution.ProgramInputBindingRuntimeHandleRef,
					RuntimeHandle: &execution.RuntimeHandleRef{
						StepID: "node_start",
					},
				}},
			},
			{
				NodeID:    "node_write",
				Action:    action.Spec{ToolName: hruntime.ProgramInteractiveWriteToolName, Args: map[string]any{"input": "status\n"}},
				DependsOn: []string{"node_view"},
				InputBinds: []execution.ProgramInputBinding{{
					Name: "handle",
					Kind: execution.ProgramInputBindingRuntimeHandleRef,
					RuntimeHandle: &execution.RuntimeHandleRef{
						StepID: "node_start",
					},
				}},
			},
			{
				NodeID:    "node_verify",
				Action:    action.Spec{ToolName: hruntime.ProgramInteractiveVerifyToolName},
				DependsOn: []string{"node_write"},
				InputBinds: []execution.ProgramInputBinding{{
					Name: "handle",
					Kind: execution.ProgramInputBindingRuntimeHandleRef,
					RuntimeHandle: &execution.RuntimeHandleRef{
						StepID: "node_start",
					},
				}},
			},
			{
				NodeID:    "node_close",
				Action:    action.Spec{ToolName: hruntime.ProgramInteractiveCloseToolName, Args: map[string]any{"reason": "program done"}},
				DependsOn: []string{"node_verify"},
				InputBinds: []execution.ProgramInputBinding{{
					Name: "handle",
					Kind: execution.ProgramInputBindingRuntimeHandleRef,
					RuntimeHandle: &execution.RuntimeHandleRef{
						StepID: "node_start",
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
	if controller.startCalls != 1 || controller.viewCalls != 1 || controller.writeCalls != 1 || controller.closeCalls != 1 {
		t.Fatalf("expected native interactive lifecycle calls start/view/write/close once, got start=%d view=%d write=%d close=%d", controller.startCalls, controller.viewCalls, controller.writeCalls, controller.closeCalls)
	}

	actions := mustListActions(t, rt, attached.SessionID)
	if len(actions) != 5 {
		t.Fatalf("expected five interactive lifecycle actions, got %#v", actions)
	}
	results := map[string]action.Result{}
	for _, record := range actions {
		nodeID, _ := record.Metadata[execution.ProgramMetadataKeyNodeID].(string)
		results[nodeID] = record.Result
	}
	for _, nodeID := range []string{"node_start", "node_view", "node_write", "node_verify", "node_close"} {
		handle, ok := results[nodeID].Data["runtime_handle"].(execution.RuntimeHandle)
		if !ok {
			t.Fatalf("expected %s to expose a typed runtime_handle contract, got %#v", nodeID, results[nodeID].Data)
		}
		if handle.HandleID != "hdl_program_native" {
			t.Fatalf("expected %s runtime_handle to preserve handle identity, got %#v", nodeID, handle)
		}
	}
	startHandleID, _ := results["node_start"].Data["handle_id"].(string)
	viewHandleID, _ := results["node_view"].Data["handle_id"].(string)
	writeHandleID, _ := results["node_write"].Data["handle_id"].(string)
	verifyHandleID, _ := results["node_verify"].Data["handle_id"].(string)
	closeHandleID, _ := results["node_close"].Data["handle_id"].(string)
	if startHandleID == "" || startHandleID != viewHandleID || startHandleID != writeHandleID || startHandleID != verifyHandleID || startHandleID != closeHandleID {
		t.Fatalf("expected interactive lifecycle to preserve one handle identity, got start=%q view=%q write=%q verify=%q close=%q", startHandleID, viewHandleID, writeHandleID, verifyHandleID, closeHandleID)
	}
	if got, _ := results["node_view"].Data["data"].(string); got != "hello" {
		t.Fatalf("expected native interactive view result data, got %#v", results["node_view"])
	}
	if got, _ := results["node_write"].Data["bytes"].(int64); got != int64(len("status\n")) {
		t.Fatalf("expected native interactive write byte count, got %#v", results["node_write"])
	}
	if got, _ := results["node_verify"].Data["status"].(string); got != "active" {
		t.Fatalf("expected native interactive verify to return active status, got %#v", results["node_verify"])
	}
	if got, _ := results["node_close"].Data["status"].(string); got != "closed" {
		t.Fatalf("expected native interactive close to return closed status, got %#v", results["node_close"])
	}
	activeVerify, err := rt.EvaluateVerify(context.Background(), verify.Spec{
		Checks: []verify.Check{{Kind: "pty_handle_active"}},
	}, results["node_view"], out.Session)
	if err != nil {
		t.Fatalf("evaluate pty_handle_active against native interactive result: %v", err)
	}
	if !activeVerify.Success {
		t.Fatalf("expected pty_handle_active to succeed against native interactive result, got %#v", activeVerify)
	}
	streamVerify, err := rt.EvaluateVerify(context.Background(), verify.Spec{
		Checks: []verify.Check{{
			Kind: "pty_stream_contains",
			Args: map[string]any{"text": "verifier", "timeout_ms": 50},
		}},
	}, results["node_write"], out.Session)
	if err != nil {
		t.Fatalf("evaluate pty_stream_contains against native interactive result: %v", err)
	}
	if !streamVerify.Success {
		t.Fatalf("expected pty_stream_contains to succeed against native interactive result, got %#v", streamVerify)
	}

	handles, err := rt.ListRuntimeHandles(attached.SessionID)
	if err != nil {
		t.Fatalf("list runtime handles: %v", err)
	}
	if len(handles) != 1 || handles[0].HandleID != startHandleID || handles[0].Status != execution.RuntimeHandleClosed {
		t.Fatalf("expected one closed runtime handle after native lifecycle, got %#v", handles)
	}
	projectedRuntime, err := rt.GetInteractiveRuntime(startHandleID)
	if err != nil {
		t.Fatalf("get projected interactive runtime: %v", err)
	}
	if projectedRuntime.Lineage == nil || projectedRuntime.Lineage.Program == nil {
		t.Fatalf("expected interactive runtime lineage from persisted handle, got %#v", projectedRuntime)
	}
	if projectedRuntime.Lineage.Program.ProgramID != "prog_native_interactive" || projectedRuntime.Lineage.Program.GroupID != "runtime/program:prog_native_interactive" || projectedRuntime.Lineage.Program.NodeID != "node_start" {
		t.Fatalf("expected persisted interactive runtime to retain full program lineage, got %#v", projectedRuntime.Lineage)
	}
	if controller.lastStart.AttemptID == "" || controller.lastStart.CycleID == "" || controller.lastStart.TraceID == "" {
		t.Fatalf("expected native interactive start to receive execution linkage, got %#v", controller.lastStart)
	}
}

func TestRunProgramNativeInteractiveAuditEventsPreserveOrderAndCausation(t *testing.T) {
	controller := &stubInteractiveController{}
	rt := hruntime.New(hruntime.Options{
		InteractiveController: controller,
		Audit:                 audit.NewMemoryStore(),
		Verifiers:             verify.NewRegistry(),
		Policy:                permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "program interactive audit", "native interactive audit events should stay ordered and causally linked")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "native interactive audit order"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	out, err := rt.RunProgram(context.Background(), attached.SessionID, execution.Program{
		ProgramID: "prog_native_interactive_audit",
		Nodes: []execution.ProgramNode{
			{
				NodeID: "node_start",
				Action: action.Spec{
					ToolName: hruntime.ProgramInteractiveStartToolName,
					Args: map[string]any{
						"handle_id": "hdl_program_native_audit",
						"kind":      "stub",
					},
				},
			},
			{
				NodeID:    "node_view",
				Action:    action.Spec{ToolName: hruntime.ProgramInteractiveViewToolName, Args: map[string]any{"offset": int64(0), "max_bytes": 32}},
				DependsOn: []string{"node_start"},
				InputBinds: []execution.ProgramInputBinding{{
					Name: "handle",
					Kind: execution.ProgramInputBindingRuntimeHandleRef,
					RuntimeHandle: &execution.RuntimeHandleRef{
						StepID: "node_start",
					},
				}},
			},
			{
				NodeID:    "node_write",
				Action:    action.Spec{ToolName: hruntime.ProgramInteractiveWriteToolName, Args: map[string]any{"input": "status\n"}},
				DependsOn: []string{"node_view"},
				InputBinds: []execution.ProgramInputBinding{{
					Name: "handle",
					Kind: execution.ProgramInputBindingRuntimeHandleRef,
					RuntimeHandle: &execution.RuntimeHandleRef{
						StepID: "node_start",
					},
				}},
			},
			{
				NodeID:    "node_close",
				Action:    action.Spec{ToolName: hruntime.ProgramInteractiveCloseToolName, Args: map[string]any{"reason": "audit done"}},
				DependsOn: []string{"node_write"},
				InputBinds: []execution.ProgramInputBinding{{
					Name: "handle",
					Kind: execution.ProgramInputBindingRuntimeHandleRef,
					RuntimeHandle: &execution.RuntimeHandleRef{
						StepID: "node_start",
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
	if controller.startCalls != 1 || controller.viewCalls != 1 || controller.writeCalls != 1 || controller.closeCalls != 1 {
		t.Fatalf("expected native interactive lifecycle calls start/view/write/close once, got start=%d view=%d write=%d close=%d", controller.startCalls, controller.viewCalls, controller.writeCalls, controller.closeCalls)
	}

	events := mustListAuditEvents(t, rt, attached.SessionID)
	handleEvents := make([]audit.Event, 0, 4)
	for _, event := range events {
		if event.CausationID != "hdl_program_native_audit" {
			continue
		}
		switch event.Type {
		case audit.EventRuntimeHandleCreated, audit.EventRuntimeHandleUpdated, audit.EventRuntimeHandleClosed:
			handleEvents = append(handleEvents, event)
		}
	}
	if len(handleEvents) != 4 {
		t.Fatalf("expected runtime handle created/updated/updated/closed events, got %#v", events)
	}

	expectedTypes := []string{
		audit.EventRuntimeHandleCreated,
		audit.EventRuntimeHandleUpdated,
		audit.EventRuntimeHandleUpdated,
		audit.EventRuntimeHandleClosed,
	}
	expectedVersions := []int64{1, 2, 3, 4}
	traceID := ""
	cycleID := ""
	for i, event := range handleEvents {
		if event.Type != expectedTypes[i] {
			t.Fatalf("expected runtime handle audit event %d to be %q, got %#v", i, expectedTypes[i], handleEvents)
		}
		if event.Sequence == 0 || (i > 0 && event.Sequence <= handleEvents[i-1].Sequence) {
			t.Fatalf("expected strictly increasing runtime handle audit sequences, got %#v", handleEvents)
		}
		if event.CausationID != "hdl_program_native_audit" {
			t.Fatalf("expected runtime handle audit causation to stay on the handle id, got %#v", handleEvents)
		}
		if handleID, _ := event.Payload["handle_id"].(string); handleID != "hdl_program_native_audit" {
			t.Fatalf("expected runtime handle audit payload to expose handle id, got %#v", handleEvents)
		}
		if version, _ := event.Payload["version"].(int64); version != expectedVersions[i] {
			t.Fatalf("expected runtime handle audit versions %v, got %#v", expectedVersions, handleEvents)
		}
		if event.AttemptID == "" || event.TraceID == "" || event.CycleID == "" {
			t.Fatalf("expected runtime handle audit event to retain attempt/trace/cycle linkage, got %#v", handleEvents)
		}
		if i == 0 {
			traceID = event.TraceID
			cycleID = event.CycleID
			continue
		}
		if event.TraceID != traceID || event.CycleID != cycleID {
			t.Fatalf("expected runtime handle audit events to preserve stable trace/cycle lineage, got %#v", handleEvents)
		}
	}
	if status := fmt.Sprint(handleEvents[0].Payload["status"]); status != string(execution.RuntimeHandleActive) {
		t.Fatalf("expected create event to record active handle status, got %#v", handleEvents[0])
	}
	if status := fmt.Sprint(handleEvents[3].Payload["status"]); status != string(execution.RuntimeHandleClosed) {
		t.Fatalf("expected close event to record closed handle status, got %#v", handleEvents[3])
	}
}

func TestRunStepNativeInteractiveLifecycleFailsClosedWithoutInteractiveController(t *testing.T) {
	rt := hruntime.New(hruntime.Options{
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "interactive lifecycle disabled", "fail closed without interactive controller")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "fail closed without interactive controller"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(attached.SessionID, "native interactive disabled", []plan.StepSpec{{
		StepID: "step_native_interactive_disabled",
		Title:  "disabled native interactive start",
		Action: action.Spec{
			ToolName: hruntime.ProgramInteractiveStartToolName,
			Args: map[string]any{
				"kind": "stub",
			},
		},
		OnFail: plan.OnFailSpec{Strategy: "abort"},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	out, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	if out.Session.Phase != session.PhaseFailed {
		t.Fatalf("expected step execution to fail closed without interactive controller, got %#v", out.Session)
	}
	if out.Execution.Action.Error == nil || out.Execution.Action.Error.Code != "CAPABILITY_DISABLED" {
		t.Fatalf("expected native interactive step execution to fail with CAPABILITY_DISABLED, got %#v", out.Execution.Action)
	}
	if out.Execution.Step.Status != plan.StepFailed {
		t.Fatalf("expected disabled native interactive step to be marked failed, got %#v", out.Execution.Step)
	}
	if snapshots := mustListCapabilitySnapshots(t, rt, attached.SessionID); len(snapshots) != 0 {
		t.Fatalf("expected no capability snapshot to persist for disabled native interactive execution, got %#v", snapshots)
	}
	if handles, err := rt.ListRuntimeHandles(attached.SessionID); err != nil {
		t.Fatalf("list runtime handles: %v", err)
	} else if len(handles) != 0 {
		t.Fatalf("expected no runtime handles to be created when native interactive execution is disabled, got %#v", handles)
	}
}

func TestRunProgramNativeInteractiveLifecycleFailsClosedWithoutInteractiveController(t *testing.T) {
	rt := hruntime.New(hruntime.Options{
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "program interactive lifecycle disabled", "fail closed without interactive controller")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "program should fail closed without interactive controller"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	out, err := rt.RunProgram(context.Background(), attached.SessionID, execution.Program{
		ProgramID: "prog_native_interactive_disabled",
		Nodes: []execution.ProgramNode{{
			NodeID: "node_start",
			Action: action.Spec{
				ToolName: hruntime.ProgramInteractiveStartToolName,
				Args: map[string]any{
					"kind": "stub",
				},
			},
			OnFail: &plan.OnFailSpec{Strategy: "abort"},
		}},
	})
	if err != nil {
		t.Fatalf("run program: %v", err)
	}
	if out.Session.Phase != session.PhaseFailed {
		t.Fatalf("expected program execution to fail closed without interactive controller, got %#v", out.Session)
	}
	if len(out.Executions) != 1 || out.Executions[0].Execution.Action.Error == nil || out.Executions[0].Execution.Action.Error.Code != "CAPABILITY_DISABLED" {
		t.Fatalf("expected native interactive program execution to fail with CAPABILITY_DISABLED, got %#v", out.Executions)
	}
	if out.Executions[0].Execution.Step.Status != plan.StepFailed {
		t.Fatalf("expected disabled native interactive program node to be marked failed, got %#v", out.Executions[0].Execution.Step)
	}
	if snapshots := mustListCapabilitySnapshots(t, rt, attached.SessionID); len(snapshots) != 0 {
		t.Fatalf("expected no capability snapshot to persist for disabled native interactive program execution, got %#v", snapshots)
	}
	if handles, err := rt.ListRuntimeHandles(attached.SessionID); err != nil {
		t.Fatalf("list runtime handles: %v", err)
	} else if len(handles) != 0 {
		t.Fatalf("expected no runtime handles to be created when native interactive program execution is disabled, got %#v", handles)
	}
}

func TestRunProgramFanoutNativeInteractiveLifecycleFailsClosedWithoutInteractiveController(t *testing.T) {
	rt := hruntime.New(hruntime.Options{
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "fanout program interactive lifecycle disabled", "fanout should fail closed without interactive controller")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "fanout program should fail closed without interactive controller"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	out, err := rt.RunProgram(context.Background(), attached.SessionID, execution.Program{
		ProgramID: "prog_native_interactive_disabled_fanout",
		Nodes: []execution.ProgramNode{{
			NodeID: "node_start",
			Action: action.Spec{
				ToolName: hruntime.ProgramInteractiveStartToolName,
				Args: map[string]any{
					"kind": "stub",
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
		t.Fatalf("expected fanout program execution to fail closed without interactive controller, got %#v", out.Session)
	}
	if len(out.Executions) != 2 {
		t.Fatalf("expected two fanout executions, got %#v", out.Executions)
	}
	for _, executionOut := range out.Executions {
		if executionOut.Execution.Action.Error == nil || executionOut.Execution.Action.Error.Code != "CAPABILITY_DISABLED" {
			t.Fatalf("expected fanout native interactive execution to fail with CAPABILITY_DISABLED, got %#v", executionOut.Execution.Action)
		}
		if executionOut.Execution.Step.Status != plan.StepFailed {
			t.Fatalf("expected disabled fanout native interactive node to be marked failed, got %#v", executionOut.Execution.Step)
		}
	}
	if snapshots := mustListCapabilitySnapshots(t, rt, attached.SessionID); len(snapshots) != 0 {
		t.Fatalf("expected no capability snapshots to persist for disabled fanout native interactive execution, got %#v", snapshots)
	}
	if handles, err := rt.ListRuntimeHandles(attached.SessionID); err != nil {
		t.Fatalf("list runtime handles: %v", err)
	} else if len(handles) != 0 {
		t.Fatalf("expected no runtime handles to be created when fanout native interactive execution is disabled, got %#v", handles)
	}
}

func TestProgramInteractiveLifecyclePreservesHandleIdentityCycleAndVersionAcrossRuntimeReinit(t *testing.T) {
	controller := &stubInteractiveController{}
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	attempts := execution.NewMemoryAttemptStore()
	actions := execution.NewMemoryActionStore()
	verifications := execution.NewMemoryVerificationStore()
	artifacts := execution.NewMemoryArtifactStore()
	runtimeHandles := execution.NewMemoryRuntimeHandleStore()

	opts := hruntime.Options{
		Sessions:              sessions,
		Tasks:                 tasks,
		Plans:                 plans,
		Attempts:              attempts,
		Actions:               actions,
		Verifications:         verifications,
		Artifacts:             artifacts,
		RuntimeHandles:        runtimeHandles,
		InteractiveController: controller,
		Verifiers:             verify.NewRegistry(),
		Policy:                permission.DefaultEvaluator{},
	}

	rt1 := hruntime.New(opts)
	sess := mustCreateSession(t, rt1, "interactive lifecycle reinit", "preserve handle identity across runtime restart")
	tsk := mustCreateTask(t, rt1, task.Spec{TaskType: "demo", Goal: "preserve interactive handle identity, cycle, and version across restart"})
	attached, err := rt1.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	created, err := rt1.CreatePlanFromProgram(attached.SessionID, "", execution.Program{
		ProgramID: "prog_interactive_reinit",
		Nodes: []execution.ProgramNode{
			{
				NodeID: "node_start",
				Action: action.Spec{
					ToolName: hruntime.ProgramInteractiveStartToolName,
					Args: map[string]any{
						"handle_id": "hdl_program_reinit",
						"kind":      "stub",
					},
				},
			},
			{
				NodeID:    "node_view",
				Action:    action.Spec{ToolName: hruntime.ProgramInteractiveViewToolName, Args: map[string]any{"offset": int64(0), "max_bytes": 32}},
				DependsOn: []string{"node_start"},
				InputBinds: []execution.ProgramInputBinding{{
					Name: "handle",
					Kind: execution.ProgramInputBindingRuntimeHandleRef,
					RuntimeHandle: &execution.RuntimeHandleRef{
						StepID: "node_start",
					},
				}},
			},
			{
				NodeID:    "node_write",
				Action:    action.Spec{ToolName: hruntime.ProgramInteractiveWriteToolName, Args: map[string]any{"input": "status\n"}},
				DependsOn: []string{"node_view"},
				InputBinds: []execution.ProgramInputBinding{{
					Name: "handle",
					Kind: execution.ProgramInputBindingRuntimeHandleRef,
					RuntimeHandle: &execution.RuntimeHandleRef{
						StepID: "node_start",
					},
				}},
			},
			{
				NodeID:    "node_verify",
				Action:    action.Spec{ToolName: hruntime.ProgramInteractiveVerifyToolName},
				DependsOn: []string{"node_write"},
				InputBinds: []execution.ProgramInputBinding{{
					Name: "handle",
					Kind: execution.ProgramInputBindingRuntimeHandleRef,
					RuntimeHandle: &execution.RuntimeHandleRef{
						StepID: "node_start",
					},
				}},
			},
			{
				NodeID:    "node_close",
				Action:    action.Spec{ToolName: hruntime.ProgramInteractiveCloseToolName, Args: map[string]any{"reason": "done"}},
				DependsOn: []string{"node_verify"},
				InputBinds: []execution.ProgramInputBinding{{
					Name: "handle",
					Kind: execution.ProgramInputBindingRuntimeHandleRef,
					RuntimeHandle: &execution.RuntimeHandleRef{
						StepID: "node_start",
					},
				}},
			},
		},
	})
	if err != nil {
		t.Fatalf("create plan from program: %v", err)
	}
	if len(created.Steps) != 5 {
		t.Fatalf("expected five compiled steps, got %#v", created.Steps)
	}

	startOut, err := rt1.RunStep(context.Background(), attached.SessionID, created.Steps[0])
	if err != nil {
		t.Fatalf("run start step: %v", err)
	}
	startResultHandle, ok := startOut.Execution.Action.Data["runtime_handle"].(execution.RuntimeHandle)
	if !ok {
		t.Fatalf("expected typed runtime_handle on start result, got %#v", startOut.Execution.Action.Data)
	}
	if startResultHandle.HandleID != "hdl_program_reinit" || startResultHandle.Version != 1 {
		t.Fatalf("expected start result to expose handle_id/version 1, got %#v", startResultHandle)
	}
	startHandle, err := rt1.GetRuntimeHandle("hdl_program_reinit")
	if err != nil {
		t.Fatalf("get runtime handle after start: %v", err)
	}
	if startHandle.HandleID != "hdl_program_reinit" || startHandle.Version != 1 || startHandle.CycleID == "" {
		t.Fatalf("expected started handle identity/version/cycle, got %#v", startHandle)
	}
	startCycleID := startHandle.CycleID

	rt2 := hruntime.New(opts)
	latestPlans := mustListPlans(t, rt2, attached.SessionID)
	latest := latestPlans[len(latestPlans)-1]
	if len(latest.Steps) != 5 {
		t.Fatalf("expected latest compiled plan to preserve five steps, got %#v", latest.Steps)
	}

	viewOut, err := rt2.RunStep(context.Background(), attached.SessionID, latest.Steps[1])
	if err != nil {
		t.Fatalf("run view step after reinit: %v", err)
	}
	viewHandle, err := rt2.GetRuntimeHandle("hdl_program_reinit")
	if err != nil {
		t.Fatalf("get runtime handle after view: %v", err)
	}
	if viewHandle.HandleID != "hdl_program_reinit" || viewHandle.CycleID != startCycleID || viewHandle.Version != 2 || viewHandle.Status != execution.RuntimeHandleActive {
		t.Fatalf("expected view to preserve handle identity/cycle and advance version to 2, got %#v", viewHandle)
	}
	if resultHandle, ok := viewOut.Execution.Action.Data["runtime_handle"].(execution.RuntimeHandle); !ok || resultHandle.Version != 2 {
		t.Fatalf("expected view result to expose version 2 runtime_handle, got %#v", viewOut.Execution.Action.Data)
	}

	writeOut, err := rt2.RunStep(context.Background(), attached.SessionID, latest.Steps[2])
	if err != nil {
		t.Fatalf("run write step after reinit: %v", err)
	}
	writeHandle, err := rt2.GetRuntimeHandle("hdl_program_reinit")
	if err != nil {
		t.Fatalf("get runtime handle after write: %v", err)
	}
	if writeHandle.HandleID != "hdl_program_reinit" || writeHandle.CycleID != startCycleID || writeHandle.Version != 3 || writeHandle.Status != execution.RuntimeHandleActive {
		t.Fatalf("expected write to preserve handle identity/cycle and advance version to 3, got %#v", writeHandle)
	}
	if resultHandle, ok := writeOut.Execution.Action.Data["runtime_handle"].(execution.RuntimeHandle); !ok || resultHandle.Version != 3 {
		t.Fatalf("expected write result to expose version 3 runtime_handle, got %#v", writeOut.Execution.Action.Data)
	}

	verifyOut, err := rt2.RunStep(context.Background(), attached.SessionID, latest.Steps[3])
	if err != nil {
		t.Fatalf("run verify step after reinit: %v", err)
	}
	verifyHandle, err := rt2.GetRuntimeHandle("hdl_program_reinit")
	if err != nil {
		t.Fatalf("get runtime handle after verify: %v", err)
	}
	if verifyHandle.HandleID != "hdl_program_reinit" || verifyHandle.CycleID != startCycleID || verifyHandle.Version != 3 || verifyHandle.Status != execution.RuntimeHandleActive {
		t.Fatalf("expected verify to preserve handle identity/cycle and keep version at 3, got %#v", verifyHandle)
	}
	if resultHandle, ok := verifyOut.Execution.Action.Data["runtime_handle"].(execution.RuntimeHandle); !ok || resultHandle.Version != 3 {
		t.Fatalf("expected verify result to expose stable version 3 runtime_handle, got %#v", verifyOut.Execution.Action.Data)
	}

	closeOut, err := rt2.RunStep(context.Background(), attached.SessionID, latest.Steps[4])
	if err != nil {
		t.Fatalf("run close step after reinit: %v", err)
	}
	closeHandle, err := rt2.GetRuntimeHandle("hdl_program_reinit")
	if err != nil {
		t.Fatalf("get runtime handle after close: %v", err)
	}
	if closeHandle.HandleID != "hdl_program_reinit" || closeHandle.CycleID != startCycleID || closeHandle.Version != 4 || closeHandle.Status != execution.RuntimeHandleClosed {
		t.Fatalf("expected close to preserve handle identity/cycle and advance version to 4 closed, got %#v", closeHandle)
	}
	if resultHandle, ok := closeOut.Execution.Action.Data["runtime_handle"].(execution.RuntimeHandle); !ok || resultHandle.Version != 4 || resultHandle.Status != execution.RuntimeHandleClosed {
		t.Fatalf("expected close result to expose closed version 4 runtime_handle, got %#v", closeOut.Execution.Action.Data)
	}

	rt3 := hruntime.New(opts)
	reloaded, err := rt3.GetRuntimeHandle("hdl_program_reinit")
	if err != nil {
		t.Fatalf("get runtime handle after second reinit: %v", err)
	}
	if reloaded.HandleID != "hdl_program_reinit" || reloaded.CycleID != startCycleID || reloaded.Version != 4 || reloaded.Status != execution.RuntimeHandleClosed {
		t.Fatalf("expected reloaded handle to preserve identity/cycle/version/closed status, got %#v", reloaded)
	}

	cycle, err := rt3.GetExecutionCycle(attached.SessionID, startCycleID)
	if err != nil {
		t.Fatalf("get execution cycle after reinit: %v", err)
	}
	if cycle.CycleID != startCycleID || len(cycle.RuntimeHandles) != 1 {
		t.Fatalf("expected original execution cycle to retain one runtime handle, got %#v", cycle)
	}
	if cycle.RuntimeHandles[0].HandleID != "hdl_program_reinit" || cycle.RuntimeHandles[0].Version != 4 || cycle.RuntimeHandles[0].Status != execution.RuntimeHandleClosed {
		t.Fatalf("expected execution cycle replay to expose final runtime handle state, got %#v", cycle.RuntimeHandles)
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

func TestRunProgramAggregateVerifyProjectionDoesNotKeepPendingReasonOnSuccessfulTargets(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.target", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, targetAwareHandler{})
	verifiers := verify.NewRegistry()
	verify.RegisterBuiltins(verifiers)

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verifiers,
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "program aggregate verify projection reasons", "do not keep aggregate pending reason on successful targets")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "do not keep aggregate pending reason on successful targets"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	out, err := rt.RunProgram(context.Background(), attached.SessionID, execution.Program{
		ProgramID: "prog_aggregate_verify_projection_reason",
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
	if len(out.Aggregates) != 1 {
		t.Fatalf("expected one aggregate result, got %#v", out.Aggregates)
	}
	for _, target := range out.Aggregates[0].Targets {
		if target.Status != plan.StepCompleted {
			t.Fatalf("expected successful aggregate targets to remain completed, got %#v", out.Aggregates[0].Targets)
		}
		if target.Reason != "" {
			t.Fatalf("expected successful aggregate targets to drop stale pending reasons, got %#v", out.Aggregates[0].Targets)
		}
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

func TestRunProgramMaterializesInlineBytesAttachmentToTempFile(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.read_file", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, fileReaderHandler{})

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "program materialize inline bytes attachment", "materialize inline bytes attachment")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "materialize inline bytes attachment"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	out, err := rt.RunProgram(context.Background(), attached.SessionID, execution.Program{
		ProgramID: "prog_inline_bytes_attachment",
		Nodes: []execution.ProgramNode{{
			NodeID: "node_read",
			Action: action.Spec{ToolName: "demo.read_file"},
			InputBinds: []execution.ProgramInputBinding{{
				Name: "path",
				Kind: execution.ProgramInputBindingAttachment,
				Attachment: &execution.AttachmentInput{
					Kind:        execution.AttachmentInputBytes,
					Bytes:       []byte("hello-inline-bytes-attachment"),
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
	if got, _ := actions[0].Result.Data["stdout"].(string); got != "hello-inline-bytes-attachment" {
		t.Fatalf("expected materialized inline bytes payload, got %#v", actions[0].Result.Data)
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

func TestRunProgramSupportsCustomAttachmentMaterializationMode(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.echo-payload", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, echoPayloadHandler{})
	materializer := &recordingAttachmentMaterializer{
		value: map[string]any{
			"kind":  "opaque_handle",
			"value": "mem://payload/demo",
		},
	}

	rt := hruntime.New(hruntime.Options{
		Tools:                  tools,
		Verifiers:              verify.NewRegistry(),
		Policy:                 permission.DefaultEvaluator{},
		AttachmentMaterializer: materializer,
	})

	sess := mustCreateSession(t, rt, "program custom attachment materialization", "support custom attachment materialization")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "support custom attachment materialization"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	out, err := rt.RunProgram(context.Background(), attached.SessionID, execution.Program{
		ProgramID: "prog_custom_attachment_materialization",
		Nodes: []execution.ProgramNode{{
			NodeID: "node_echo",
			Action: action.Spec{ToolName: "demo.echo-payload"},
			InputBinds: []execution.ProgramInputBinding{{
				Name: "payload",
				Kind: execution.ProgramInputBindingAttachment,
				Attachment: &execution.AttachmentInput{
					Kind:        execution.AttachmentInputBytes,
					Bytes:       []byte("abc"),
					Materialize: execution.AttachmentMaterialization("opaque_handle"),
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
	if got, ok := actions[0].Result.Data["payload"].(map[string]any); !ok || got["kind"] != "opaque_handle" || got["value"] != "mem://payload/demo" {
		t.Fatalf("expected custom materializer passthrough value, got %#v", actions[0].Result.Data["payload"])
	}
	if len(materializer.requests) != 1 {
		t.Fatalf("expected one materializer request, got %#v", materializer.requests)
	}
	if materializer.requests[0].Input.Materialize != execution.AttachmentMaterialization("opaque_handle") {
		t.Fatalf("expected custom materialization mode to reach materializer, got %#v", materializer.requests[0].Input)
	}
}

func TestCreatePlanFromProgramPreservesRuntimeHandleBindingsAsTypedUnresolvedInputs(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.message", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, messageHandler{})

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "program runtime handle binding", "preserve runtime handle binding")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "preserve runtime handle binding"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	created, err := rt.CreatePlanFromProgram(attached.SessionID, "", execution.Program{
		ProgramID: "prog_handle_ref",
		Nodes: []execution.ProgramNode{
			{
				NodeID: "node_start",
				Action: action.Spec{ToolName: "demo.message"},
				InputBinds: []execution.ProgramInputBinding{
					{Name: "message", Kind: execution.ProgramInputBindingLiteral, Value: "start"},
				},
			},
			{
				NodeID:    "node_use",
				Action:    action.Spec{ToolName: "demo.message"},
				DependsOn: []string{"node_start"},
				InputBinds: []execution.ProgramInputBinding{
					{
						Name: "handle",
						Kind: execution.ProgramInputBindingRuntimeHandleRef,
						RuntimeHandle: &execution.RuntimeHandleRef{
							StepID: "node_start",
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("create plan from program: %v", err)
	}
	if len(created.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %#v", created.Steps)
	}

	bindings, ok := execution.ProgramInputBindingsFromStep(created.Steps[1])
	if !ok || len(bindings) != 1 {
		t.Fatalf("expected unresolved runtime handle binding on compiled step, got %#v", created.Steps[1].Metadata)
	}
	if bindings[0].Kind != execution.ProgramInputBindingRuntimeHandleRef || bindings[0].RuntimeHandle == nil {
		t.Fatalf("expected typed runtime handle binding, got %#v", bindings[0])
	}
	if bindings[0].RuntimeHandle.StepID != "node_start" {
		t.Fatalf("expected runtime handle ref to preserve source node, got %#v", bindings[0].RuntimeHandle)
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

type targetLongOutputHandler struct {
	outputs map[string]string
}

func (h targetLongOutputHandler) Invoke(_ context.Context, args map[string]any) (action.Result, error) {
	target, _ := args[execution.TargetArgKey].(map[string]any)
	targetID, _ := target[execution.TargetMetadataKeyID].(string)
	return action.Result{
		OK: true,
		Data: map[string]any{
			"target_id": targetID,
			"stdout":    h.outputs[targetID],
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

type targetErrorHandler struct {
	failTargets map[string]bool
}

func (h targetErrorHandler) Invoke(_ context.Context, args map[string]any) (action.Result, error) {
	target, _ := args[execution.TargetArgKey].(map[string]any)
	targetID, _ := target[execution.TargetMetadataKeyID].(string)
	if h.failTargets != nil && h.failTargets[targetID] {
		return action.Result{
			OK: false,
			Error: &action.Error{
				Code:    "TARGET_FAILED",
				Message: "target failed",
			},
			Data: map[string]any{
				"target_id": targetID,
				"stdout":    "",
			},
		}, nil
	}
	return action.Result{
		OK: true,
		Data: map[string]any{
			"target_id": targetID,
			"stdout":    targetID,
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

type inspectArgHandler struct{}

func (inspectArgHandler) Invoke(_ context.Context, args map[string]any) (action.Result, error) {
	message := fmt.Sprint(args["message"])
	suffix := message
	if len(suffix) > 6 {
		suffix = suffix[len(suffix)-6:]
	}
	return action.Result{
		OK: true,
		Data: map[string]any{
			"message_length": len(message),
			"message_suffix": suffix,
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

type mixedInteropHandler struct{}

func (mixedInteropHandler) Invoke(_ context.Context, args map[string]any) (action.Result, error) {
	message, _ := args["message"].(string)
	handle, ok := args["handle"].(execution.RuntimeHandleRef)
	if !ok {
		return action.Result{
			OK: false,
			Error: &action.Error{
				Code:    "HANDLE_TYPE_INVALID",
				Message: fmt.Sprintf("handle arg type %T", args["handle"]),
			},
		}, nil
	}
	return action.Result{
		OK: true,
		Data: map[string]any{
			"message":   message,
			"handle_id": handle.HandleID,
			"kind":      handle.Kind,
			"status":    string(handle.Status),
		},
	}, nil
}

type runtimeHandleProducerHandler struct {
	kind        string
	status      execution.RuntimeHandleStatus
	skipTargets map[string]bool
}

func (h runtimeHandleProducerHandler) Invoke(_ context.Context, args map[string]any) (action.Result, error) {
	target, _ := args[execution.TargetArgKey].(map[string]any)
	targetID, _ := target[execution.TargetMetadataKeyID].(string)
	if targetID == "" {
		targetID = "node_start"
	}
	if h.skipTargets != nil && h.skipTargets[targetID] {
		return action.Result{OK: true, Data: map[string]any{"stdout": "no runtime handle emitted"}}, nil
	}
	kind := h.kind
	if kind == "" {
		kind = "pty"
	}
	handle := map[string]any{
		"handle_id": "hdl_" + targetID,
		"kind":      kind,
		"value":     "pty-session-" + targetID,
	}
	if h.status != "" {
		handle["status"] = string(h.status)
	}
	return action.Result{
		OK: true,
		Data: map[string]any{
			"runtime_handle": handle,
		},
	}, nil
}

type runtimeHandleRefHandler struct{}

func (runtimeHandleRefHandler) Invoke(_ context.Context, args map[string]any) (action.Result, error) {
	ref, ok := args["handle"].(execution.RuntimeHandleRef)
	if !ok {
		return action.Result{
			OK: true,
			Data: map[string]any{
				"arg_type": fmt.Sprintf("%T", args["handle"]),
			},
		}, nil
	}
	return action.Result{
		OK: true,
		Data: map[string]any{
			"arg_type":  "typed",
			"handle_id": ref.HandleID,
			"kind":      ref.Kind,
			"status":    string(ref.Status),
			"version":   ref.Version,
		},
	}, nil
}

type scriptedMessageHandler struct{}

func (scriptedMessageHandler) Invoke(_ context.Context, args map[string]any) (action.Result, error) {
	message, _ := args["message"].(string)
	if message == "fail" {
		return action.Result{
			OK: false,
			Error: &action.Error{
				Code:    "SCRIPTED_FAIL",
				Message: "scripted failure",
			},
		}, nil
	}
	return action.Result{
		OK: true,
		Data: map[string]any{
			"stdout": message,
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

type echoPayloadHandler struct{}

func (echoPayloadHandler) Invoke(_ context.Context, args map[string]any) (action.Result, error) {
	return action.Result{
		OK: true,
		Data: map[string]any{
			"payload": args["payload"],
		},
	}, nil
}

type recordingAttachmentMaterializer struct {
	mu       sync.Mutex
	value    any
	requests []hruntime.AttachmentMaterializeRequest
}

type programTestPTYInspector struct {
	inspect map[string]shellmodule.PTYInspectResult
	read    map[string]shellmodule.PTYReadResult
}

func (i programTestPTYInspector) Inspect(_ context.Context, handleID string) (shellmodule.PTYInspectResult, error) {
	result, ok := i.inspect[handleID]
	if !ok {
		return shellmodule.PTYInspectResult{}, shellmodule.ErrPTYSessionNotFound
	}
	result.HandleID = handleID
	return result, nil
}

func (i programTestPTYInspector) Read(_ context.Context, handleID string, _ shellmodule.PTYReadRequest) (shellmodule.PTYReadResult, error) {
	result, ok := i.read[handleID]
	if !ok {
		return shellmodule.PTYReadResult{}, shellmodule.ErrPTYSessionNotFound
	}
	result.HandleID = handleID
	return result, nil
}

func (m *recordingAttachmentMaterializer) Materialize(_ context.Context, request hruntime.AttachmentMaterializeRequest) (any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = append(m.requests, request)
	return m.value, nil
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

type concurrencyProbeHandler struct {
	mu         sync.Mutex
	active     int
	maxActive  int
	threshold  int
	concurrent chan struct{}
	release    chan struct{}
}

func newConcurrencyProbeHandler(threshold int) *concurrencyProbeHandler {
	return &concurrencyProbeHandler{
		threshold:  threshold,
		concurrent: make(chan struct{}),
		release:    make(chan struct{}),
	}
}

func (h *concurrencyProbeHandler) Invoke(ctx context.Context, args map[string]any) (action.Result, error) {
	h.mu.Lock()
	h.active++
	if h.active > h.maxActive {
		h.maxActive = h.active
		if h.maxActive >= h.threshold {
			select {
			case <-h.concurrent:
			default:
				close(h.concurrent)
			}
		}
	}
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		h.active--
		h.mu.Unlock()
	}()

	select {
	case <-h.release:
	case <-ctx.Done():
		return action.Result{
			OK: false,
			Error: &action.Error{
				Code:    "CONCURRENCY_TIMEOUT",
				Message: ctx.Err().Error(),
			},
		}, ctx.Err()
	}

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

func (h *concurrencyProbeHandler) maxObserved() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.maxActive
}

type labeledBlockingHandler struct {
	started chan string
	release chan struct{}
	once    sync.Once
}

func newLabeledBlockingHandler() *labeledBlockingHandler {
	return &labeledBlockingHandler{
		started: make(chan string, 8),
		release: make(chan struct{}),
	}
}

func (h *labeledBlockingHandler) Invoke(ctx context.Context, args map[string]any) (action.Result, error) {
	label, _ := args["label"].(string)
	if label == "" {
		target, _ := args[execution.TargetArgKey].(map[string]any)
		label, _ = target[execution.TargetMetadataKeyID].(string)
	}
	h.started <- label
	select {
	case <-h.release:
	case <-ctx.Done():
		return action.Result{
			OK: false,
			Error: &action.Error{
				Code:    "CONCURRENCY_TIMEOUT",
				Message: ctx.Err().Error(),
			},
		}, ctx.Err()
	}
	return action.Result{OK: true, Data: map[string]any{"label": label}}, nil
}

func (h *labeledBlockingHandler) waitForStart(t *testing.T, timeout time.Duration) string {
	t.Helper()
	select {
	case label := <-h.started:
		return label
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for handler start after %s", timeout)
		return ""
	}
}

func (h *labeledBlockingHandler) releaseAll() {
	h.once.Do(func() {
		close(h.release)
	})
}

type nthFailingPlanUpdateStore struct {
	plan.Store
	updateErr        error
	failOnUpdateCall int
	updateCalls      int
}

func (s *nthFailingPlanUpdateStore) Update(next plan.Spec) error {
	s.updateCalls++
	if s.failOnUpdateCall > 0 && s.updateCalls == s.failOnUpdateCall {
		return s.updateErr
	}
	return s.Store.Update(next)
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

func TestAttachedProgramSessionProjectionExposesProgramLineageForHydration(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.message", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, messageHandler{})

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verify.NewRegistry(),
		Policy:    permission.DefaultEvaluator{},
	})

	sess := mustCreateSession(t, rt, "attached program projection lineage", "project attached program lineage for hydration")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "project attached program lineage for hydration"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	program := execution.Program{
		ProgramID: "prog_project_lineage",
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
	}

	created, err := rt.CreatePlan(attached.SessionID, "attached projection lineage", []plan.StepSpec{
		execution.AttachProgram(plan.StepSpec{StepID: "parent_root", Title: "parent"}, program),
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if len(created.Steps) != 2 {
		t.Fatalf("expected two compiled attached steps, got %#v", created.Steps)
	}
	for _, step := range created.Steps {
		if _, err := rt.RunStep(context.Background(), attached.SessionID, step); err != nil {
			t.Fatalf("run compiled attached step %q: %v", step.StepID, err)
		}
	}

	projection, err := replay.NewReader(rt).SessionProjection(attached.SessionID)
	if err != nil {
		t.Fatalf("session projection: %v", err)
	}
	if len(projection.Cycles) != 2 {
		t.Fatalf("expected two cycle projections, got %#v", projection.Cycles)
	}

	lineageByNode := map[string]execution.ProgramLineage{}
	for _, cycle := range projection.Cycles {
		if cycle.Program == nil {
			t.Fatalf("expected structured program lineage on cycle projection, got %#v", cycle)
		}
		lineageByNode[cycle.Program.NodeID] = *cycle.Program
	}
	if lineageByNode["node_prepare"].ProgramID != "prog_project_lineage" || lineageByNode["node_prepare"].GroupID != "parent_root__prog_project_lineage" || lineageByNode["node_prepare"].ParentStepID != "parent_root" {
		t.Fatalf("unexpected prepare lineage projection: %#v", lineageByNode["node_prepare"])
	}
	if lineageByNode["node_apply"].ProgramID != "prog_project_lineage" || lineageByNode["node_apply"].GroupID != "parent_root__prog_project_lineage" || lineageByNode["node_apply"].ParentStepID != "parent_root" {
		t.Fatalf("unexpected apply lineage projection: %#v", lineageByNode["node_apply"])
	}
	if !reflect.DeepEqual(lineageByNode["node_apply"].DependsOn, []string{"node_prepare"}) {
		t.Fatalf("expected apply lineage dependencies, got %#v", lineageByNode["node_apply"])
	}
}

func TestBlockedProgramProjectionExposesStructuredLinkageForHydration(t *testing.T) {
	rt := newBlockedRuntimeTestService()

	sess := mustCreateSession(t, rt, "blocked program projection", "project blocked program linkage")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "project blocked program linkage"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	created, err := rt.CreatePlanFromProgram(attached.SessionID, "blocked program projection", execution.Program{
		ProgramID: "prog_blocked_projection",
		Nodes: []execution.ProgramNode{{
			NodeID: "node_apply",
			Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{
				"mode":       "pipe",
				"command":    "echo blocked projection",
				"timeout_ms": 5000,
			}},
		}},
	})
	if err != nil {
		t.Fatalf("create plan from program: %v", err)
	}
	if len(created.Steps) != 1 {
		t.Fatalf("expected one compiled step, got %#v", created.Steps)
	}

	out, err := rt.RunStep(context.Background(), attached.SessionID, created.Steps[0])
	if err != nil {
		t.Fatalf("run blocked program step: %v", err)
	}
	if out.Execution.PendingApproval == nil {
		t.Fatalf("expected blocked approval, got %#v", out)
	}

	attempts := mustListAttempts(t, rt, attached.SessionID)
	if len(attempts) != 1 {
		t.Fatalf("expected one blocked attempt, got %#v", attempts)
	}

	handleMetadata := execution.ApplyInteractiveRuntimeMetadata(created.Steps[0].Metadata, &execution.InteractiveCapabilities{
		View: true,
	}, &execution.InteractiveObservation{
		Status: "active",
	}, nil)
	handleMetadata = execution.ApplyTargetMetadata(handleMetadata, execution.Target{TargetID: "target-1", Kind: "host"}, 1, 1)
	if _, err := rt.RuntimeHandles.Create(execution.RuntimeHandle{
		HandleID:  "hdl_blocked_program_projection",
		SessionID: attached.SessionID,
		TaskID:    attached.TaskID,
		AttemptID: attempts[0].AttemptID,
		CycleID:   attempts[0].CycleID,
		Status:    execution.RuntimeHandleActive,
		Metadata:  handleMetadata,
	}); err != nil {
		t.Fatalf("seed blocked runtime handle: %v", err)
	}

	view, err := rt.GetBlockedRuntimeProjection(attached.SessionID)
	if err != nil {
		t.Fatalf("get blocked runtime projection: %v", err)
	}
	if view.Program == nil || view.Program.ProgramID != "prog_blocked_projection" || view.Program.NodeID != "node_apply" {
		t.Fatalf("expected blocked projection program lineage, got %#v", view.Program)
	}
	if view.ApprovalLinkage == nil || view.ApprovalLinkage.ApprovalID != out.Execution.PendingApproval.ApprovalID || view.ApprovalLinkage.StepID != created.Steps[0].StepID {
		t.Fatalf("expected blocked projection approval linkage, got %#v", view)
	}
	if view.BlockedRuntimeLinkage == nil || view.BlockedRuntimeLinkage.BlockedRuntimeID != out.Execution.PendingApproval.ApprovalID || view.BlockedRuntimeLinkage.AttemptID != attempts[0].AttemptID || view.BlockedRuntimeLinkage.CycleID != attempts[0].CycleID {
		t.Fatalf("expected blocked projection linkage, got %#v", view)
	}
	if len(view.InteractiveRuntimes) != 1 || view.InteractiveRuntimes[0].Lineage == nil {
		t.Fatalf("expected blocked projection interactive lineage, got %#v", view.InteractiveRuntimes)
	}
	if view.InteractiveRuntimes[0].Lineage.HandleID != "hdl_blocked_program_projection" || view.InteractiveRuntimes[0].Lineage.AttemptID != attempts[0].AttemptID {
		t.Fatalf("unexpected blocked projection interactive handle lineage, got %#v", view.InteractiveRuntimes[0].Lineage)
	}
	if view.InteractiveRuntimes[0].Lineage.Program == nil || view.InteractiveRuntimes[0].Lineage.Program.ProgramID != "prog_blocked_projection" || view.InteractiveRuntimes[0].Lineage.Program.NodeID != "node_apply" {
		t.Fatalf("expected blocked projection interactive program lineage, got %#v", view.InteractiveRuntimes[0].Lineage)
	}
}
