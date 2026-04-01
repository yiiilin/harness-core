package runtime_test

import (
	"context"
	"errors"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/capability"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/persistence"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/planning"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type singleStepPlanner struct {
	step plan.StepSpec
}

func (p singleStepPlanner) PlanNext(_ context.Context, state session.State, _ task.Spec, _ hruntime.ContextPackage) (plan.StepSpec, error) {
	if state.CurrentStepID != "" {
		return plan.StepSpec{}, errors.New("planner exhausted")
	}
	return p.step, nil
}

type splitServiceStores struct {
	sessions            *session.MemoryStore
	tasks               *task.MemoryStore
	plans               *plan.MemoryStore
	approvals           *approval.MemoryStore
	attempts            *execution.MemoryAttemptStore
	actions             *execution.MemoryActionStore
	verifications       *execution.MemoryVerificationStore
	artifacts           *execution.MemoryArtifactStore
	runtimeHandles      *execution.MemoryRuntimeHandleStore
	capabilitySnapshots *capability.MemorySnapshotStore
	planningRecords     *planning.MemoryStore
	audits              *audit.MemoryStore
}

func newSplitStoreRuntime(policy permission.Evaluator, planner hruntime.Planner, tools *tool.Registry, verifiers *verify.Registry) (*hruntime.Service, splitServiceStores) {
	serviceStores := splitServiceStores{
		sessions:            session.NewMemoryStore(),
		tasks:               task.NewMemoryStore(),
		plans:               plan.NewMemoryStore(),
		approvals:           approval.NewMemoryStore(),
		attempts:            execution.NewMemoryAttemptStore(),
		actions:             execution.NewMemoryActionStore(),
		verifications:       execution.NewMemoryVerificationStore(),
		artifacts:           execution.NewMemoryArtifactStore(),
		runtimeHandles:      execution.NewMemoryRuntimeHandleStore(),
		capabilitySnapshots: capability.NewMemorySnapshotStore(),
		planningRecords:     planning.NewMemoryStore(),
		audits:              audit.NewMemoryStore(),
	}
	runnerRepos := persistence.RepositorySet{
		Sessions:            session.NewMemoryStore(),
		Tasks:               task.NewMemoryStore(),
		Plans:               plan.NewMemoryStore(),
		Approvals:           approval.NewMemoryStore(),
		Attempts:            execution.NewMemoryAttemptStore(),
		Actions:             execution.NewMemoryActionStore(),
		Verifications:       execution.NewMemoryVerificationStore(),
		Artifacts:           execution.NewMemoryArtifactStore(),
		RuntimeHandles:      execution.NewMemoryRuntimeHandleStore(),
		CapabilitySnapshots: capability.NewMemorySnapshotStore(),
		PlanningRecords:     planning.NewMemoryStore(),
		ContextSummaries:    hruntime.NewMemoryContextSummaryStore(),
		Audits:              audit.NewMemoryStore(),
	}
	return hruntime.New(withExplicitPlannerProjection(hruntime.Options{
		Sessions:            serviceStores.sessions,
		Tasks:               serviceStores.tasks,
		Plans:               serviceStores.plans,
		Approvals:           serviceStores.approvals,
		Attempts:            serviceStores.attempts,
		Actions:             serviceStores.actions,
		Verifications:       serviceStores.verifications,
		Artifacts:           serviceStores.artifacts,
		RuntimeHandles:      serviceStores.runtimeHandles,
		CapabilitySnapshots: serviceStores.capabilitySnapshots,
		PlanningRecords:     serviceStores.planningRecords,
		Audit:               serviceStores.audits,
		Tools:               tools,
		Verifiers:           verifiers,
		Planner:             planner,
		Policy:              policy,
		Runner:              sinkRunner{repos: runnerRepos},
	})), serviceStores
}

func TestPublicReadAPIsUseRunnerCommittedRepositories(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(tool.Definition{ToolName: "demo.handle", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, runtimeHandleHandler{})

	rt, serviceStores := newSplitStoreRuntime(permission.DefaultEvaluator{}, hruntime.NoopPlanner{}, tools, verify.NewRegistry())

	sess := mustCreateSession(t, rt, "split reads", "read through runner repositories")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "public reads should see runner state"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(attached.SessionID, "single step", []plan.StepSpec{{
		StepID: "step_split_reads",
		Title:  "use runner-backed reads",
		Action: action.Spec{ToolName: "demo.handle"},
		Verify: verify.Spec{},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	out, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	if out.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected completed session, got %#v", out.Session)
	}

	if items, _ := serviceStores.sessions.List(); len(items) != 0 {
		t.Fatalf("expected service session store to stay empty, got %#v", items)
	}
	if items, _ := serviceStores.tasks.List(); len(items) != 0 {
		t.Fatalf("expected service task store to stay empty, got %#v", items)
	}
	if items, _ := serviceStores.plans.ListBySession(attached.SessionID); len(items) != 0 {
		t.Fatalf("expected service plan store to stay empty, got %#v", items)
	}

	if got, err := rt.GetSession(attached.SessionID); err != nil || got.SessionID != attached.SessionID {
		t.Fatalf("expected runner-backed GetSession to succeed, got session=%#v err=%v", got, err)
	}
	if items, err := rt.ListSessions(); err != nil || len(items) != 1 {
		t.Fatalf("expected runner-backed ListSessions to return one session, got %#v err=%v", items, err)
	}
	if got, err := rt.GetTask(tsk.TaskID); err != nil || got.TaskID != tsk.TaskID {
		t.Fatalf("expected runner-backed GetTask to succeed, got task=%#v err=%v", got, err)
	}
	if items, err := rt.ListTasks(); err != nil || len(items) != 1 {
		t.Fatalf("expected runner-backed ListTasks to return one task, got %#v err=%v", items, err)
	}
	if got, err := rt.GetPlan(pl.PlanID); err != nil || got.PlanID != pl.PlanID {
		t.Fatalf("expected runner-backed GetPlan to succeed, got plan=%#v err=%v", got, err)
	}
	if items := mustListPlans(t, rt, attached.SessionID); len(items) != 1 {
		t.Fatalf("expected runner-backed ListPlans to return one plan, got %#v", items)
	}
	if attempts := mustListAttempts(t, rt, attached.SessionID); len(attempts) != 1 {
		t.Fatalf("expected runner-backed attempt reads, got %#v", attempts)
	}
	if actions := mustListActions(t, rt, attached.SessionID); len(actions) != 1 {
		t.Fatalf("expected runner-backed action reads, got %#v", actions)
	}
	if verifications := mustListVerifications(t, rt, attached.SessionID); len(verifications) != 1 {
		t.Fatalf("expected runner-backed verification reads, got %#v", verifications)
	}
	if artifacts := mustListArtifacts(t, rt, attached.SessionID); len(artifacts) == 0 {
		t.Fatalf("expected runner-backed artifact reads, got %#v", artifacts)
	}
	if snapshots := mustListCapabilitySnapshots(t, rt, attached.SessionID); len(snapshots) != 1 {
		t.Fatalf("expected runner-backed capability snapshot reads, got %#v", snapshots)
	}
	handles, err := rt.ListRuntimeHandles(attached.SessionID)
	if err != nil || len(handles) != 1 {
		t.Fatalf("expected runner-backed runtime handle reads, got %#v err=%v", handles, err)
	}
	if got, err := rt.GetRuntimeHandle(handles[0].HandleID); err != nil || got.HandleID != handles[0].HandleID {
		t.Fatalf("expected runner-backed GetRuntimeHandle to succeed, got handle=%#v err=%v", got, err)
	}
	if cycles, err := rt.ListExecutionCycles(attached.SessionID); err != nil || len(cycles) != 1 {
		t.Fatalf("expected runner-backed execution cycle reads, got %#v err=%v", cycles, err)
	}
	if events := mustListAuditEvents(t, rt, attached.SessionID); len(events) == 0 {
		t.Fatalf("expected runner-backed audit reads, got %#v", events)
	}
}

func TestPlannerAndCapabilityFreezeUseRunnerRepositories(t *testing.T) {
	tools := tool.NewRegistry()
	handler := &countingHandler{}
	tools.Register(tool.Definition{ToolName: "demo.echo", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, handler)

	planner := singleStepPlanner{step: plan.StepSpec{
		StepID: "step_split_plan",
		Title:  "planned through runner repos",
		Action: action.Spec{ToolName: "demo.echo"},
		Verify: verify.Spec{},
	}}
	rt, serviceStores := newSplitStoreRuntime(permission.DefaultEvaluator{}, planner, tools, verify.NewRegistry())

	sess := mustCreateSession(t, rt, "split planner", "planner and freeze should read runner state")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "planner reads should use runner repos"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	created, _, err := rt.CreatePlanFromPlanner(context.Background(), attached.SessionID, "planner split stores", 1)
	if err != nil {
		t.Fatalf("create plan from planner: %v", err)
	}
	if created.PlanID == "" {
		t.Fatalf("expected created plan id")
	}

	if items, _ := serviceStores.plans.ListBySession(attached.SessionID); len(items) != 0 {
		t.Fatalf("expected service plan store to stay empty, got %#v", items)
	}
	if items := mustListPlans(t, rt, attached.SessionID); len(items) != 1 {
		t.Fatalf("expected runner-backed plans after planner create, got %#v", items)
	}
	records := mustListPlanningRecords(t, rt, attached.SessionID)
	if len(records) != 1 || records[0].PlanID != created.PlanID {
		t.Fatalf("expected runner-backed planning records after planner create, got %#v", records)
	}
	if got, err := rt.GetPlanningRecord(records[0].PlanningID); err != nil || got.PlanningID != records[0].PlanningID {
		t.Fatalf("expected runner-backed GetPlanningRecord to succeed, got %#v err=%v", got, err)
	}
	if snapshots := mustListCapabilitySnapshots(t, rt, attached.SessionID); len(snapshots) != 1 {
		t.Fatalf("expected runner-backed frozen capability view snapshots, got %#v", snapshots)
	}

	out, err := rt.RunSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("run session against existing plan: %v", err)
	}
	if out.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected existing planner-created plan to execute to completion, got %#v", out.Session)
	}
	if handler.calls != 1 {
		t.Fatalf("expected exactly one execution against the existing plan, got %d", handler.calls)
	}
	if items := mustListPlans(t, rt, attached.SessionID); len(items) != 1 {
		t.Fatalf("expected run session not to create a duplicate plan revision, got %#v", items)
	}
	if records := mustListPlanningRecords(t, rt, attached.SessionID); len(records) != 1 {
		t.Fatalf("expected run session not to create duplicate planning records, got %#v", records)
	}
}

func TestApprovalFlowsUseRunnerRepositoriesForReads(t *testing.T) {
	tools := tool.NewRegistry()
	handler := &countingHandler{}
	tools.Register(tool.Definition{ToolName: "demo.echo", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, handler)

	rt, serviceStores := newSplitStoreRuntime(askPolicy{}, hruntime.NoopPlanner{}, tools, verify.NewRegistry())

	sess := mustCreateSession(t, rt, "split approval", "approval reads should use runner state")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "approval reads should not use stale service store"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(attached.SessionID, "approval", []plan.StepSpec{{
		StepID: "step_split_approval",
		Title:  "ask through runner repos",
		Action: action.Spec{ToolName: "demo.echo"},
		Verify: verify.Spec{},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	initial, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	if initial.Execution.PendingApproval == nil {
		t.Fatalf("expected pending approval")
	}
	if items, _ := serviceStores.approvals.List(attached.SessionID); len(items) != 0 {
		t.Fatalf("expected service approval store to stay empty, got %#v", items)
	}

	approvalID := initial.Execution.PendingApproval.ApprovalID
	if got, err := rt.GetApproval(approvalID); err != nil || got.ApprovalID != approvalID {
		t.Fatalf("expected runner-backed GetApproval to succeed, got approval=%#v err=%v", got, err)
	}
	if items, err := rt.ListApprovals(attached.SessionID); err != nil || len(items) != 1 {
		t.Fatalf("expected runner-backed ListApprovals to return one item, got %#v err=%v", items, err)
	}

	if _, _, err := rt.RespondApproval(approvalID, approval.Response{Reply: approval.ReplyOnce}); err != nil {
		t.Fatalf("respond approval: %v", err)
	}
	resumed, err := rt.ResumePendingApproval(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("resume approval: %v", err)
	}
	if resumed.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected approval-resumed session to complete, got %#v", resumed.Session)
	}
	if handler.calls != 1 {
		t.Fatalf("expected exactly one post-approval execution, got %d", handler.calls)
	}
}

func TestRecoveryReadPathsUseRunnerRepositories(t *testing.T) {
	tools := tool.NewRegistry()
	handler := &countingHandler{}
	tools.Register(tool.Definition{ToolName: "demo.echo", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true}, handler)

	rt, serviceStores := newSplitStoreRuntime(permission.DefaultEvaluator{}, hruntime.NoopPlanner{}, tools, verify.NewRegistry())

	sess := mustCreateSession(t, rt, "split recovery", "recovery reads should use runner state")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "recover through runner-backed reads"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	_, err = rt.CreatePlan(attached.SessionID, "recovery", []plan.StepSpec{{
		StepID: "step_split_recovery",
		Title:  "recover through runner repos",
		Action: action.Spec{ToolName: "demo.echo"},
		Verify: verify.Spec{},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	if _, err := rt.MarkSessionInterrupted(context.Background(), attached.SessionID); err != nil {
		t.Fatalf("mark interrupted: %v", err)
	}
	if items, _ := serviceStores.sessions.List(); len(items) != 0 {
		t.Fatalf("expected service session store to stay empty, got %#v", items)
	}
	if recoverable := mustListRecoverableSessions(t, rt); len(recoverable) != 1 || recoverable[0].SessionID != attached.SessionID {
		t.Fatalf("expected runner-backed ListRecoverableSessions to expose interrupted session, got %#v", recoverable)
	}

	out, err := rt.RecoverSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("recover session: %v", err)
	}
	if out.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected recovered session to complete, got %#v", out.Session)
	}
	if handler.calls != 1 {
		t.Fatalf("expected one recovered execution, got %d", handler.calls)
	}
}
