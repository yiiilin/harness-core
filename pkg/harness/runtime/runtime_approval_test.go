package runtime_test

import (
	"context"
	"errors"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/persistence"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type askPolicy struct{}

func (askPolicy) Evaluate(_ context.Context, _ session.State, _ plan.StepSpec) (permission.Decision, error) {
	return permission.Decision{Action: permission.Ask, Reason: "approval required", MatchedRule: "test/ask"}, nil
}

type scopedAskPolicy struct{}

func (scopedAskPolicy) Evaluate(_ context.Context, _ session.State, step plan.StepSpec) (permission.Decision, error) {
	path, _ := step.Action.Args["path"].(string)
	return permission.Decision{
		Action:      permission.Ask,
		Reason:      "approval required",
		MatchedRule: "test/ask:" + path,
	}, nil
}

type toolScopedAskPolicy struct {
	askTools map[string]bool
}

func (p toolScopedAskPolicy) Evaluate(_ context.Context, _ session.State, step plan.StepSpec) (permission.Decision, error) {
	if p.askTools != nil && p.askTools[step.Action.ToolName] {
		return permission.Decision{
			Action:      permission.Ask,
			Reason:      "approval required",
			MatchedRule: "test/ask:" + step.Action.ToolName,
		}, nil
	}
	return permission.Decision{
		Action:      permission.Allow,
		Reason:      "allowed",
		MatchedRule: "test/allow",
	}, nil
}

type countingHandler struct {
	calls int
}

func (h *countingHandler) Invoke(_ context.Context, _ map[string]any) (action.Result, error) {
	h.calls++
	return action.Result{
		OK: true,
		Data: map[string]any{
			"status":    "completed",
			"exit_code": 0,
			"stdout":    "executed",
		},
	}, nil
}

func TestRequestSessionApprovalBlocksRunSessionBeforeFirstStep(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	audits := audit.NewMemoryStore()
	handler := &countingHandler{}

	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		handler,
	)
	verifiers.Register(
		verify.Definition{Kind: "exit_code", Description: "Verify that an execution result exit code is in the allowed set."},
		verify.ExitCodeChecker{},
	)

	rt := hruntime.New(hruntime.Options{
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Tools:     tools,
		Verifiers: verifiers,
		Audit:     audits,
	})

	sess := mustCreateSession(t, rt, "session gate", "block before first step")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "require whole-session approval"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	if _, err := rt.CreatePlan(attached.SessionID, "session gate", []plan.StepSpec{{
		StepID: "step_session_gate",
		Title:  "run after session approval",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo gated", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
		}},
	}}); err != nil {
		t.Fatalf("create plan: %v", err)
	}
	pinnedBeforeGate, err := rt.GetSession(attached.SessionID)
	if err != nil {
		t.Fatalf("get session before gate: %v", err)
	}
	pinnedBeforeGate.CurrentStepID = "step_session_gate"
	pinnedBeforeGate.Version++
	if err := rt.Sessions.Update(pinnedBeforeGate); err != nil {
		t.Fatalf("pin current step before gate: %v", err)
	}

	approvalRec, blockedState, err := rt.RequestSessionApproval(context.Background(), attached.SessionID, hruntime.SessionApprovalRequest{
		Reason:      "approve the whole request before execution",
		MatchedRule: "test/session-gate",
		Metadata:    map[string]any{"scope": "session_entry"},
	})
	if err != nil {
		t.Fatalf("request session approval: %v", err)
	}
	if approvalRec.Status != approval.StatusPending {
		t.Fatalf("expected pending approval record, got %#v", approvalRec)
	}
	if blockedState.PendingApprovalID != approvalRec.ApprovalID || blockedState.ExecutionState != session.ExecutionAwaitingApproval {
		t.Fatalf("expected session awaiting approval, got %#v", blockedState)
	}
	if approvalRec.Step.StepID == "" || approvalRec.Step.Status != plan.StepBlocked {
		t.Fatalf("expected synthetic blocked gate step on approval record, got %#v", approvalRec.Step)
	}
	attempts := mustListAttempts(t, rt, attached.SessionID)
	if len(attempts) != 0 {
		t.Fatalf("expected no execution attempts before approval, got %#v", attempts)
	}

	out, err := rt.RunSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("run session: %v", err)
	}
	if out.Session.PendingApprovalID != approvalRec.ApprovalID || out.Session.ExecutionState != session.ExecutionAwaitingApproval {
		t.Fatalf("expected run session to stay blocked on entry approval, got %#v", out.Session)
	}
	if len(out.Executions) != 0 {
		t.Fatalf("expected no step execution while session approval is pending, got %#v", out.Executions)
	}
	if handler.calls != 0 {
		t.Fatalf("expected tool execution to stay blocked before approval, got %d calls", handler.calls)
	}
}

func TestRunSessionContinuesAfterApprovingSessionApprovalGate(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	audits := audit.NewMemoryStore()
	handler := &countingHandler{}

	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		handler,
	)
	verifiers.Register(
		verify.Definition{Kind: "exit_code", Description: "Verify that an execution result exit code is in the allowed set."},
		verify.ExitCodeChecker{},
	)

	rt := hruntime.New(hruntime.Options{
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Tools:     tools,
		Verifiers: verifiers,
		Audit:     audits,
	})

	sess := mustCreateSession(t, rt, "session gate resume", "resume original session after approval")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "session approval then execute"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	if _, err := rt.CreatePlan(attached.SessionID, "session gate", []plan.StepSpec{{
		StepID: "step_after_session_gate",
		Title:  "run after gate approval",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo resumed", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
		}},
	}}); err != nil {
		t.Fatalf("create plan: %v", err)
	}

	approvalRec, _, err := rt.RequestSessionApproval(context.Background(), attached.SessionID, hruntime.SessionApprovalRequest{
		Reason:      "approve request before the first step",
		MatchedRule: "test/session-gate",
	})
	if err != nil {
		t.Fatalf("request session approval: %v", err)
	}
	if _, _, err := rt.RespondApproval(approvalRec.ApprovalID, approval.Response{Reply: approval.ReplyOnce}); err != nil {
		t.Fatalf("respond approval: %v", err)
	}

	out, err := rt.RunSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("run session after approval: %v", err)
	}
	if handler.calls != 1 {
		t.Fatalf("expected first real step to execute exactly once after approval, got %d", handler.calls)
	}
	if out.Session.PendingApprovalID != "" || out.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected session to resume through original run path and complete, got %#v", out.Session)
	}
	if len(out.Executions) != 1 || out.Executions[0].Execution.Step.StepID != "step_after_session_gate" {
		t.Fatalf("expected only the real plan step to appear in execution output, got %#v", out.Executions)
	}
	attempts := mustListAttempts(t, rt, attached.SessionID)
	if len(attempts) != 1 || attempts[0].StepID != "step_after_session_gate" {
		t.Fatalf("expected exactly one real-step attempt after approval, got %#v", attempts)
	}
}

func TestSessionApprovalGateAuditEventsPreserveOrderAndCausation(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	audits := audit.NewMemoryStore()
	handler := &countingHandler{}

	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		handler,
	)
	verifiers.Register(
		verify.Definition{Kind: "exit_code", Description: "Verify that an execution result exit code is in the allowed set."},
		verify.ExitCodeChecker{},
	)

	rt := hruntime.New(hruntime.Options{
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Tools:     tools,
		Verifiers: verifiers,
		Audit:     audits,
	})

	sess := mustCreateSession(t, rt, "session gate audit order", "request-level approval events should stay ordered and causally linked")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "request-level approval audit order"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	if _, err := rt.CreatePlan(attached.SessionID, "session gate", []plan.StepSpec{{
		StepID: "step_after_session_gate_audit",
		Title:  "run after gate approval audit",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo session-gate-audit", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
		}},
	}}); err != nil {
		t.Fatalf("create plan: %v", err)
	}

	approvalRec, _, err := rt.RequestSessionApproval(context.Background(), attached.SessionID, hruntime.SessionApprovalRequest{
		Reason:      "approve request before the first audited step",
		MatchedRule: "test/session-gate-audit",
	})
	if err != nil {
		t.Fatalf("request session approval: %v", err)
	}
	if _, _, err := rt.RespondApproval(approvalRec.ApprovalID, approval.Response{Reply: approval.ReplyOnce}); err != nil {
		t.Fatalf("respond approval: %v", err)
	}
	if _, err := rt.RunSession(context.Background(), attached.SessionID); err != nil {
		t.Fatalf("run session after approval: %v", err)
	}

	events := mustListAuditEvents(t, rt, attached.SessionID)
	var requestEvent *audit.Event
	var approvedEvent *audit.Event
	var toolCalledEvent *audit.Event
	for i := range events {
		switch events[i].Type {
		case audit.EventApprovalRequested:
			if requestEvent == nil {
				requestEvent = &events[i]
			}
		case audit.EventApprovalApproved:
			if approvedEvent == nil {
				approvedEvent = &events[i]
			}
		case audit.EventToolCalled:
			if toolCalledEvent == nil {
				toolCalledEvent = &events[i]
			}
		}
	}
	if requestEvent == nil || approvedEvent == nil || toolCalledEvent == nil {
		t.Fatalf("expected approval request/approve and first tool call events, got %#v", events)
	}
	if requestEvent.Sequence == 0 || approvedEvent.Sequence == 0 || toolCalledEvent.Sequence == 0 {
		t.Fatalf("expected sequenced audit events, got request=%#v approved=%#v tool=%#v", requestEvent, approvedEvent, toolCalledEvent)
	}
	if !(requestEvent.Sequence < approvedEvent.Sequence && approvedEvent.Sequence < toolCalledEvent.Sequence) {
		t.Fatalf("expected request-level approval events to precede the first tool call, got request=%#v approved=%#v tool=%#v", requestEvent, approvedEvent, toolCalledEvent)
	}
	if requestEvent.ApprovalID != approvalRec.ApprovalID || requestEvent.TraceID != approvalRec.ApprovalID || requestEvent.CausationID != approvalRec.ApprovalID {
		t.Fatalf("expected approval.requested event to correlate to approval id %q, got %#v", approvalRec.ApprovalID, requestEvent)
	}
	if approvedEvent.ApprovalID != approvalRec.ApprovalID || approvedEvent.TraceID != approvalRec.ApprovalID || approvedEvent.CausationID != approvalRec.ApprovalID {
		t.Fatalf("expected approval.approved event to correlate to approval id %q, got %#v", approvalRec.ApprovalID, approvedEvent)
	}
	if requestEvent.StepID != approvalRec.Step.StepID || approvedEvent.StepID != approvalRec.Step.StepID {
		t.Fatalf("expected request-level approval events to stay pinned to the synthetic gate step, got request=%#v approved=%#v", requestEvent, approvedEvent)
	}
	if scope, _ := requestEvent.Payload["scope"].(string); scope != "session_entry" {
		t.Fatalf("expected approval.requested event to expose session_entry scope, got %#v", requestEvent)
	}
	if toolCalledEvent.StepID != "step_after_session_gate_audit" {
		t.Fatalf("expected first tool call after gate approval to belong to the real first step, got %#v", toolCalledEvent)
	}
	if handler.calls != 1 {
		t.Fatalf("expected one real tool call after the session gate approval, got %d", handler.calls)
	}
}

func TestRequestSessionApprovalRejectsSessionsThatAlreadyStartedExecution(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	handler := &countingHandler{}

	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		handler,
	)
	verifiers.Register(
		verify.Definition{Kind: "exit_code", Description: "Verify that an execution result exit code is in the allowed set."},
		verify.ExitCodeChecker{},
	)

	rt := hruntime.New(hruntime.Options{
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Tools:     tools,
		Verifiers: verifiers,
	})

	sess := mustCreateSession(t, rt, "late session gate", "reject gate after execution starts")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "start execution first"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(attached.SessionID, "late gate", []plan.StepSpec{{
		StepID: "step_started",
		Title:  "already started step",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo started", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
		}},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := rt.MarkSessionInFlight(context.Background(), attached.SessionID, pl.Steps[0].StepID); err != nil {
		t.Fatalf("mark session in flight: %v", err)
	}

	if _, _, err := rt.RequestSessionApproval(context.Background(), attached.SessionID, hruntime.SessionApprovalRequest{
		Reason: "too late",
	}); !errors.Is(err, hruntime.ErrSessionApprovalGateTooLate) {
		t.Fatalf("expected ErrSessionApprovalGateTooLate, got %v", err)
	}
}

func TestResumePendingApprovalForSessionGateClearsGateBeforeRunningPlan(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	handler := &countingHandler{}

	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		handler,
	)
	verifiers.Register(
		verify.Definition{Kind: "exit_code", Description: "Verify that an execution result exit code is in the allowed set."},
		verify.ExitCodeChecker{},
	)

	rt := hruntime.New(hruntime.Options{
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Tools:     tools,
		Verifiers: verifiers,
	})

	sess := mustCreateSession(t, rt, "session gate direct resume", "resume the gate context before running plan work")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "session gate resume should not execute the plan step yet"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	if _, err := rt.CreatePlan(attached.SessionID, "session gate", []plan.StepSpec{{
		StepID: "step_after_direct_gate_resume",
		Title:  "run after gate resume",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo after gate resume", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
		}},
	}}); err != nil {
		t.Fatalf("create plan: %v", err)
	}

	approvalRec, _, err := rt.RequestSessionApproval(context.Background(), attached.SessionID, hruntime.SessionApprovalRequest{
		Reason: "gate the whole session before execution",
	})
	if err != nil {
		t.Fatalf("request session approval: %v", err)
	}
	if _, _, err := rt.RespondApproval(approvalRec.ApprovalID, approval.Response{Reply: approval.ReplyOnce}); err != nil {
		t.Fatalf("respond approval: %v", err)
	}

	resumed, err := rt.ResumePendingApproval(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("resume pending approval: %v", err)
	}
	if resumed.Execution.Step.StepID != "__session_approval_gate__" || resumed.Session.PendingApprovalID != "" {
		t.Fatalf("expected resume to clear the gate context itself, got %#v", resumed)
	}
	if attempts := mustListAttempts(t, rt, attached.SessionID); len(attempts) != 0 {
		t.Fatalf("expected gate resume itself not to create execution attempts, got %#v", attempts)
	}
	if handler.calls != 0 {
		t.Fatalf("expected real plan work to remain unexecuted until RunSession, got %d calls", handler.calls)
	}

	out, err := rt.RunSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("run session after gate resume: %v", err)
	}
	if len(out.Executions) != 1 || out.Executions[0].Execution.Step.StepID != "step_after_direct_gate_resume" {
		t.Fatalf("expected the first real plan step to execute only after RunSession, got %#v", out.Executions)
	}
	if handler.calls != 1 {
		t.Fatalf("expected exactly one real step execution after RunSession, got %d", handler.calls)
	}
}

func TestRecoverSessionResumesApprovedSessionGateIntoNativeInteractiveProgramWithoutHandleIdentityLoss(t *testing.T) {
	controller := &stubInteractiveController{}
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	approvals := approval.NewMemoryStore()
	attempts := execution.NewMemoryAttemptStore()
	actions := execution.NewMemoryActionStore()
	verifications := execution.NewMemoryVerificationStore()
	artifacts := execution.NewMemoryArtifactStore()
	runtimeHandles := execution.NewMemoryRuntimeHandleStore()

	opts := hruntime.Options{
		Sessions:              sessions,
		Tasks:                 tasks,
		Plans:                 plans,
		Approvals:             approvals,
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

	sess := mustCreateSession(t, rt1, "interactive session gate", "recover an approved session gate into a native interactive program")
	tsk := mustCreateTask(t, rt1, task.Spec{TaskType: "demo", Goal: "session gate should preserve native interactive handle identity"})
	attached, err := rt1.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	if _, err := rt1.CreatePlanFromProgram(attached.SessionID, "", execution.Program{
		ProgramID: "prog_session_gate_interactive",
		Nodes: []execution.ProgramNode{
			{
				NodeID: "node_start",
				Action: action.Spec{
					ToolName: hruntime.ProgramInteractiveStartToolName,
					Args: map[string]any{
						"handle_id": "hdl_session_gate_interactive",
						"kind":      "stub",
					},
				},
			},
			{
				NodeID:    "node_close",
				Action:    action.Spec{ToolName: hruntime.ProgramInteractiveCloseToolName, Args: map[string]any{"reason": "approved"}},
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
	}); err != nil {
		t.Fatalf("create plan from program: %v", err)
	}

	approvalRec, blockedState, err := rt1.RequestSessionApproval(context.Background(), attached.SessionID, hruntime.SessionApprovalRequest{
		Reason:      "approve the interactive workflow before it starts",
		MatchedRule: "test/session-gate-interactive",
	})
	if err != nil {
		t.Fatalf("request session approval: %v", err)
	}
	if blockedState.PendingApprovalID != approvalRec.ApprovalID {
		t.Fatalf("expected pending approval on the session gate, got %#v", blockedState)
	}

	blocked, err := rt1.RunSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("run session while gate is pending: %v", err)
	}
	if blocked.Session.PendingApprovalID != approvalRec.ApprovalID || len(blocked.Executions) != 0 {
		t.Fatalf("expected session gate to block before native interactive execution, got %#v", blocked)
	}
	if controller.startCalls != 0 || controller.closeCalls != 0 {
		t.Fatalf("expected native interactive controller to stay idle before approval, got start=%d close=%d", controller.startCalls, controller.closeCalls)
	}

	if _, _, err := rt1.RespondApproval(approvalRec.ApprovalID, approval.Response{Reply: approval.ReplyOnce}); err != nil {
		t.Fatalf("respond approval: %v", err)
	}

	rt2 := hruntime.New(opts)
	recovered, err := rt2.RecoverSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("recover session: %v", err)
	}
	if recovered.Session.Phase != session.PhaseComplete || recovered.Session.PendingApprovalID != "" {
		t.Fatalf("expected recovery to clear the gate and complete the native interactive program, got %#v", recovered.Session)
	}
	if len(recovered.Executions) != 2 {
		t.Fatalf("expected recovery to execute native interactive start and close, got %#v", recovered.Executions)
	}
	if recovered.Executions[0].Execution.Step.StepID != "prog_session_gate_interactive__node_start" || recovered.Executions[1].Execution.Step.StepID != "prog_session_gate_interactive__node_close" {
		t.Fatalf("unexpected recovered execution order: %#v", recovered.Executions)
	}
	if controller.startCalls != 1 || controller.closeCalls != 1 {
		t.Fatalf("expected one native interactive start and close after approval, got start=%d close=%d", controller.startCalls, controller.closeCalls)
	}

	handle, err := rt2.GetRuntimeHandle("hdl_session_gate_interactive")
	if err != nil {
		t.Fatalf("get runtime handle after recovery: %v", err)
	}
	if handle.HandleID != "hdl_session_gate_interactive" || handle.Status != execution.RuntimeHandleClosed || handle.Version != 2 || handle.CycleID == "" {
		t.Fatalf("expected one stable closed handle after session-gated interactive recovery, got %#v", handle)
	}
	handles, err := rt2.ListRuntimeHandles(attached.SessionID)
	if err != nil {
		t.Fatalf("list runtime handles: %v", err)
	}
	if len(handles) != 1 {
		t.Fatalf("expected exactly one persisted native interactive handle, got %#v", handles)
	}
	if resultHandle, ok := recovered.Executions[1].Execution.Action.Data["runtime_handle"].(execution.RuntimeHandle); !ok || resultHandle.HandleID != handle.HandleID || resultHandle.CycleID != handle.CycleID || resultHandle.Version != 2 {
		t.Fatalf("expected close execution to expose the stable native interactive handle, got %#v", recovered.Executions[1].Execution.Action.Data)
	}
	attemptsAfter := mustListAttempts(t, rt2, attached.SessionID)
	if len(attemptsAfter) != 2 {
		t.Fatalf("expected two real attempts after session-gated recovery, got %#v", attemptsAfter)
	}
	for _, attempt := range attemptsAfter {
		if attempt.Status != execution.AttemptCompleted {
			t.Fatalf("expected session-gated native interactive attempts to complete, got %#v", attemptsAfter)
		}
	}
	storedApproval, err := rt2.GetApproval(approvalRec.ApprovalID)
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	if storedApproval.Status != approval.StatusConsumed {
		t.Fatalf("expected session-gate approval to be consumed after recovery, got %#v", storedApproval)
	}
}

func TestRecoverSessionResumesApprovedNativeInteractiveProgramStepWithoutHandleIdentityLoss(t *testing.T) {
	controller := &stubInteractiveController{}
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	approvals := approval.NewMemoryStore()
	attempts := execution.NewMemoryAttemptStore()
	actions := execution.NewMemoryActionStore()
	verifications := execution.NewMemoryVerificationStore()
	artifacts := execution.NewMemoryArtifactStore()
	runtimeHandles := execution.NewMemoryRuntimeHandleStore()

	opts := hruntime.Options{
		Sessions:              sessions,
		Tasks:                 tasks,
		Plans:                 plans,
		Approvals:             approvals,
		Attempts:              attempts,
		Actions:               actions,
		Verifications:         verifications,
		Artifacts:             artifacts,
		RuntimeHandles:        runtimeHandles,
		InteractiveController: controller,
		Verifiers:             verify.NewRegistry(),
		Policy:                permission.DefaultEvaluator{},
	}
	policy := toolScopedAskPolicy{askTools: map[string]bool{
		hruntime.ProgramInteractiveWriteToolName: true,
	}}
	rt1 := hruntime.New(opts).WithPolicyEvaluator(policy)

	sess := mustCreateSession(t, rt1, "interactive mid-program approval", "resume a blocked native interactive step after restart")
	tsk := mustCreateTask(t, rt1, task.Spec{TaskType: "demo", Goal: "approval should resume native interactive writes without handle drift"})
	attached, err := rt1.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	initial, err := rt1.RunProgram(context.Background(), attached.SessionID, execution.Program{
		ProgramID: "prog_mid_approval_interactive",
		Nodes: []execution.ProgramNode{
			{
				NodeID: "node_start",
				Action: action.Spec{
					ToolName: hruntime.ProgramInteractiveStartToolName,
					Args: map[string]any{
						"handle_id": "hdl_mid_approval_interactive",
						"kind":      "stub",
					},
				},
			},
			{
				NodeID:    "node_write",
				Action:    action.Spec{ToolName: hruntime.ProgramInteractiveWriteToolName, Args: map[string]any{"input": "status\n"}},
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
				NodeID:    "node_close",
				Action:    action.Spec{ToolName: hruntime.ProgramInteractiveCloseToolName, Args: map[string]any{"reason": "approved"}},
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
	if initial.Session.PendingApprovalID == "" {
		t.Fatalf("expected native interactive write to block on approval, got %#v", initial.Session)
	}
	if len(initial.Executions) != 2 {
		t.Fatalf("expected start execution and blocked write attempt before approval, got %#v", initial.Executions)
	}
	if initial.Executions[0].Execution.Step.StepID != "prog_mid_approval_interactive__node_start" || initial.Executions[1].Execution.Step.StepID != "prog_mid_approval_interactive__node_write" {
		t.Fatalf("unexpected execution order before approval: %#v", initial.Executions)
	}
	if controller.startCalls != 1 || controller.writeCalls != 0 || controller.closeCalls != 0 {
		t.Fatalf("expected only native interactive start before approval, got start=%d write=%d close=%d", controller.startCalls, controller.writeCalls, controller.closeCalls)
	}

	startHandle, err := rt1.GetRuntimeHandle("hdl_mid_approval_interactive")
	if err != nil {
		t.Fatalf("get runtime handle after start: %v", err)
	}
	if startHandle.HandleID != "hdl_mid_approval_interactive" || startHandle.Version != 1 || startHandle.CycleID == "" || startHandle.Status != execution.RuntimeHandleActive {
		t.Fatalf("expected start handle to be active with stable identity before approval, got %#v", startHandle)
	}
	startCycleID := startHandle.CycleID

	attemptsBefore := mustListAttempts(t, rt1, attached.SessionID)
	if len(attemptsBefore) != 2 {
		t.Fatalf("expected completed start and blocked write attempts before approval, got %#v", attemptsBefore)
	}
	blockedWriteAttemptID := ""
	for _, attempt := range attemptsBefore {
		if attempt.StepID == "prog_mid_approval_interactive__node_write" && attempt.Status == execution.AttemptBlocked {
			blockedWriteAttemptID = attempt.AttemptID
		}
	}
	if blockedWriteAttemptID == "" {
		t.Fatalf("expected blocked write attempt before approval, got %#v", attemptsBefore)
	}
	storedApprovalBeforeResume, err := rt1.GetApproval(initial.Session.PendingApprovalID)
	if err != nil {
		t.Fatalf("get approval before resume: %v", err)
	}
	if got, _ := storedApprovalBeforeResume.Step.Action.Args["handle_id"].(string); got != "hdl_mid_approval_interactive" {
		t.Fatalf("expected approval record to persist the resolved native interactive handle_id, got %#v", storedApprovalBeforeResume.Step.Action.Args)
	}
	rtPending := hruntime.New(opts).WithPolicyEvaluator(policy)
	pending, err := rtPending.RecoverSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("recover session while approval is still pending: %v", err)
	}
	if pending.Session.PendingApprovalID != initial.Session.PendingApprovalID || len(pending.Executions) != 0 {
		t.Fatalf("expected restart while approval is pending to preserve the blocked native interactive context, got %#v", pending)
	}
	pendingHandle, err := rtPending.GetRuntimeHandle("hdl_mid_approval_interactive")
	if err != nil {
		t.Fatalf("get runtime handle while approval is pending: %v", err)
	}
	if pendingHandle.HandleID != startHandle.HandleID || pendingHandle.CycleID != startCycleID || pendingHandle.Version != 1 || pendingHandle.Status != execution.RuntimeHandleActive {
		t.Fatalf("expected native interactive handle to stay active and unchanged while approval is pending, got %#v", pendingHandle)
	}
	if controller.writeCalls != 0 || controller.closeCalls != 0 {
		t.Fatalf("expected pending approval restart not to execute native interactive write/close, got write=%d close=%d", controller.writeCalls, controller.closeCalls)
	}

	if _, _, err := rt1.RespondApproval(initial.Session.PendingApprovalID, approval.Response{Reply: approval.ReplyOnce}); err != nil {
		t.Fatalf("respond approval: %v", err)
	}

	rt2 := hruntime.New(opts).WithPolicyEvaluator(policy)
	recovered, err := rt2.RecoverSession(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("recover session after approval: %v", err)
	}
	if recovered.Session.Phase != session.PhaseComplete || recovered.Session.PendingApprovalID != "" {
		t.Fatalf("expected approved native interactive program to recover to completion, got %#v", recovered.Session)
	}
	if len(recovered.Executions) != 2 {
		t.Fatalf("expected recovery to resume write and then close, got %#v", recovered.Executions)
	}
	if recovered.Executions[0].Execution.Step.StepID != "prog_mid_approval_interactive__node_write" || recovered.Executions[1].Execution.Step.StepID != "prog_mid_approval_interactive__node_close" {
		t.Fatalf("unexpected resumed execution order after approval: %#v", recovered.Executions)
	}
	if controller.startCalls != 1 || controller.writeCalls != 1 || controller.closeCalls != 1 {
		t.Fatalf("expected exactly one native interactive start/write/close across approval recovery, got start=%d write=%d close=%d", controller.startCalls, controller.writeCalls, controller.closeCalls)
	}

	finalHandle, err := rt2.GetRuntimeHandle("hdl_mid_approval_interactive")
	if err != nil {
		t.Fatalf("get runtime handle after approval recovery: %v", err)
	}
	if finalHandle.HandleID != "hdl_mid_approval_interactive" || finalHandle.CycleID != startCycleID || finalHandle.Version != 3 || finalHandle.Status != execution.RuntimeHandleClosed {
		t.Fatalf("expected approval-resumed native interactive handle to preserve identity/cycle and finish closed at version 3, got %#v", finalHandle)
	}
	if resultHandle, ok := recovered.Executions[0].Execution.Action.Data["runtime_handle"].(execution.RuntimeHandle); !ok || resultHandle.HandleID != finalHandle.HandleID || resultHandle.CycleID != startCycleID || resultHandle.Version != 2 {
		t.Fatalf("expected resumed write step to expose version 2 runtime_handle, got %#v", recovered.Executions[0].Execution.Action.Data)
	}
	if resultHandle, ok := recovered.Executions[1].Execution.Action.Data["runtime_handle"].(execution.RuntimeHandle); !ok || resultHandle.HandleID != finalHandle.HandleID || resultHandle.CycleID != startCycleID || resultHandle.Version != 3 || resultHandle.Status != execution.RuntimeHandleClosed {
		t.Fatalf("expected resumed close step to expose the final closed runtime_handle, got %#v", recovered.Executions[1].Execution.Action.Data)
	}

	handles, err := rt2.ListRuntimeHandles(attached.SessionID)
	if err != nil {
		t.Fatalf("list runtime handles: %v", err)
	}
	if len(handles) != 1 {
		t.Fatalf("expected approval-resumed program to keep exactly one persisted handle, got %#v", handles)
	}

	attemptsAfter := mustListAttempts(t, rt2, attached.SessionID)
	if len(attemptsAfter) != 3 {
		t.Fatalf("expected start, resumed write, and close attempts after recovery, got %#v", attemptsAfter)
	}
	foundCompletedWrite := false
	for _, attempt := range attemptsAfter {
		if attempt.AttemptID == blockedWriteAttemptID {
			if attempt.Status != execution.AttemptCompleted {
				t.Fatalf("expected blocked write attempt to be resumed in place, got %#v", attempt)
			}
			foundCompletedWrite = true
		}
	}
	if !foundCompletedWrite {
		t.Fatalf("expected recovered approval flow to reuse the blocked write attempt id %q, got %#v", blockedWriteAttemptID, attemptsAfter)
	}

	cycle, err := rt2.GetExecutionCycle(attached.SessionID, startCycleID)
	if err != nil {
		t.Fatalf("get execution cycle: %v", err)
	}
	if cycle.CycleID != startCycleID || len(cycle.RuntimeHandles) != 1 {
		t.Fatalf("expected original interactive cycle to retain one runtime handle after approval recovery, got %#v", cycle)
	}
	if cycle.RuntimeHandles[0].HandleID != finalHandle.HandleID || cycle.RuntimeHandles[0].Version != 3 || cycle.RuntimeHandles[0].Status != execution.RuntimeHandleClosed {
		t.Fatalf("expected cycle projection to expose the final recovered runtime handle state, got %#v", cycle.RuntimeHandles)
	}

	storedApproval, err := rt2.GetApproval(initial.Session.PendingApprovalID)
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	if storedApproval.Status != approval.StatusConsumed {
		t.Fatalf("expected recovered native interactive approval to be consumed, got %#v", storedApproval)
	}
}

func TestRunStepPolicyAskCreatesApprovalAndDoesNotExecute(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	audits := audit.NewMemoryStore()
	handler := &countingHandler{}

	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		handler,
	)
	verifiers.Register(
		verify.Definition{Kind: "exit_code", Description: "Verify that an execution result exit code is in the allowed set."},
		verify.ExitCodeChecker{},
	)

	rt := hruntime.New(hruntime.Options{
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Tools:     tools,
		Verifiers: verifiers,
		Audit:     audits,
	}).WithPolicyEvaluator(askPolicy{})

	sess := mustCreateSession(t, rt, "ask session", "approval required path")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "wait for approval before executing"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(attached.SessionID, "ask", []plan.StepSpec{{
		StepID: "step_ask",
		Title:  "approval required shell action",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo hello", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
		}},
		OnFail: plan.OnFailSpec{Strategy: "abort"},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	out, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}

	if out.Execution.Policy.Decision.Action != permission.Ask {
		t.Fatalf("expected ask decision, got %#v", out.Execution.Policy.Decision)
	}
	if handler.calls != 0 {
		t.Fatalf("expected action not to execute before approval, got %d calls", handler.calls)
	}
	if out.Session.Phase != session.PhaseExecute {
		t.Fatalf("expected session to stay blocked before execution, got %s", out.Session.Phase)
	}
	if out.UpdatedTask != nil && out.UpdatedTask.Status != task.StatusRunning {
		t.Fatalf("expected task to stay running while approval is pending, got %#v", out.UpdatedTask)
	}

	foundApprovalRequested := false
	foundToolCall := false
	for _, event := range mustListAuditEvents(t, rt, attached.SessionID) {
		if event.Type == "approval.requested" {
			foundApprovalRequested = true
		}
		if event.Type == audit.EventToolCalled {
			foundToolCall = true
		}
	}
	if !foundApprovalRequested {
		t.Fatalf("expected approval.requested event in audit trail")
	}
	if foundToolCall {
		t.Fatalf("did not expect tool.called event before approval")
	}
}

func TestResumePendingApprovalExecutesStepAfterReplyOnce(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	audits := audit.NewMemoryStore()
	handler := &countingHandler{}

	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		handler,
	)
	verifiers.Register(
		verify.Definition{Kind: "exit_code", Description: "Verify that an execution result exit code is in the allowed set."},
		verify.ExitCodeChecker{},
	)

	rt := hruntime.New(hruntime.Options{
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Tools:     tools,
		Verifiers: verifiers,
		Audit:     audits,
	}).WithPolicyEvaluator(askPolicy{})

	sess := mustCreateSession(t, rt, "resume once", "approval then resume")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "resume after one-time approval"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(sess.SessionID, "approval", []plan.StepSpec{{
		StepID: "step_resume_once",
		Title:  "resume once",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo approved", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
		}},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	initial, err := rt.RunStep(context.Background(), sess.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	if initial.Execution.PendingApproval == nil {
		t.Fatalf("expected pending approval in execution result")
	}
	attempts := mustListAttempts(t, rt, sess.SessionID)
	if len(attempts) != 1 || attempts[0].Status != "blocked" {
		t.Fatalf("expected one blocked attempt before approval resume, got %#v", attempts)
	}
	if attempts[0].FinishedAt != 0 {
		t.Fatalf("expected blocked attempt to stay open until resume, got %#v", attempts[0])
	}
	blockedAttemptID := attempts[0].AttemptID

	approvalRec, stateAfterReply, err := rt.RespondApproval(initial.Execution.PendingApproval.ApprovalID, approval.Response{Reply: approval.ReplyOnce})
	if err != nil {
		t.Fatalf("respond approval: %v", err)
	}
	if approvalRec.Status != approval.StatusApproved {
		t.Fatalf("expected approved approval record, got %#v", approvalRec)
	}
	if stateAfterReply.PendingApprovalID == "" {
		t.Fatalf("expected session to keep approval handle until resume")
	}
	attempts = mustListAttempts(t, rt, sess.SessionID)
	if len(attempts) != 1 || attempts[0].AttemptID != blockedAttemptID || attempts[0].Status != "blocked" {
		t.Fatalf("expected approval response to keep the original blocked attempt pending, got %#v", attempts)
	}

	resumed, err := rt.ResumePendingApproval(context.Background(), sess.SessionID)
	if err != nil {
		t.Fatalf("resume pending approval: %v", err)
	}
	if handler.calls != 1 {
		t.Fatalf("expected one tool execution after resume, got %d", handler.calls)
	}
	if resumed.Session.PendingApprovalID != "" {
		t.Fatalf("expected pending approval to clear after resume, got %#v", resumed.Session)
	}
	if resumed.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected session complete after resumed execution, got %s", resumed.Session.Phase)
	}
	attempts = mustListAttempts(t, rt, sess.SessionID)
	if len(attempts) != 1 {
		t.Fatalf("expected resume to reuse the original attempt, got %#v", attempts)
	}
	if attempts[0].AttemptID != blockedAttemptID || attempts[0].Status != "completed" || attempts[0].Step.Status != plan.StepCompleted {
		t.Fatalf("expected original blocked attempt to become the completed execution attempt, got %#v", attempts[0])
	}

	storedApproval, err := rt.GetApproval(approvalRec.ApprovalID)
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	if storedApproval.Status != approval.StatusConsumed {
		t.Fatalf("expected one-time approval to be consumed, got %#v", storedApproval)
	}
}

func TestResumePendingApprovalUsesRecordedBlockedAttemptContext(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	handler := &countingHandler{}

	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		handler,
	)
	verifiers.Register(
		verify.Definition{Kind: "exit_code", Description: "Verify that an execution result exit code is in the allowed set."},
		verify.ExitCodeChecker{},
	)

	rt := hruntime.New(hruntime.Options{
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Tools:     tools,
		Verifiers: verifiers,
	}).WithPolicyEvaluator(askPolicy{})

	sess := mustCreateSession(t, rt, "recorded blocked attempt context", "resume the originally blocked attempt")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "approval resume must target original blocked attempt"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(attached.SessionID, "approval", []plan.StepSpec{{
		StepID: "step_resume_exact_attempt",
		Title:  "resume original blocked attempt",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo exact attempt", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
		}},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	initial, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	originalAttempts := mustListAttempts(t, rt, attached.SessionID)
	if len(originalAttempts) != 1 || originalAttempts[0].Status != execution.AttemptBlocked {
		t.Fatalf("expected one original blocked attempt, got %#v", originalAttempts)
	}
	originalAttempt := originalAttempts[0]

	if _, _, err := rt.RespondApproval(initial.Execution.PendingApproval.ApprovalID, approval.Response{Reply: approval.ReplyOnce}); err != nil {
		t.Fatalf("respond approval: %v", err)
	}

	bogusAttempt := originalAttempt
	bogusAttempt.AttemptID = "att_shadow_blocked_attempt"
	bogusAttempt.TraceID = "trc_shadow_blocked_attempt"
	bogusAttempt.StartedAt = bogusAttempt.StartedAt + 1
	bogusAttempt.FinishedAt = 0
	bogusAttempt.Status = execution.AttemptBlocked
	if bogusAttempt.Metadata == nil {
		bogusAttempt.Metadata = map[string]any{}
	}
	bogusAttempt.Metadata["injected"] = true
	if _, err := rt.Attempts.Create(bogusAttempt); err != nil {
		t.Fatalf("inject newer blocked attempt: %v", err)
	}

	resumed, err := rt.ResumePendingApproval(context.Background(), attached.SessionID)
	if err != nil {
		t.Fatalf("resume pending approval: %v", err)
	}
	if resumed.Session.PendingApprovalID != "" || resumed.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected approval resume to complete original execution path, got %#v", resumed.Session)
	}

	attempts := mustListAttempts(t, rt, attached.SessionID)
	if len(attempts) != 2 {
		t.Fatalf("expected original and shadow attempts to remain inspectable, got %#v", attempts)
	}
	var completedOriginal execution.Attempt
	var shadow execution.Attempt
	for _, attempt := range attempts {
		switch attempt.AttemptID {
		case originalAttempt.AttemptID:
			completedOriginal = attempt
		case bogusAttempt.AttemptID:
			shadow = attempt
		}
	}
	if completedOriginal.AttemptID == "" || completedOriginal.Status != execution.AttemptCompleted {
		t.Fatalf("expected the originally blocked attempt to be resumed and completed, got %#v", attempts)
	}
	if shadow.AttemptID == "" || shadow.Status != execution.AttemptBlocked {
		t.Fatalf("expected the later shadow blocked attempt to stay untouched, got %#v", attempts)
	}
}

func TestResumePendingApprovalFailsClosedWhenRecordedBlockedAttemptIsMissing(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	handler := &countingHandler{}

	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		handler,
	)
	verifiers.Register(
		verify.Definition{Kind: "exit_code", Description: "Verify that an execution result exit code is in the allowed set."},
		verify.ExitCodeChecker{},
	)

	rt := hruntime.New(hruntime.Options{
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Tools:     tools,
		Verifiers: verifiers,
	}).WithPolicyEvaluator(askPolicy{})

	sess := mustCreateSession(t, rt, "missing blocked attempt context", "resume must fail closed when original blocked attempt is gone")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "approval resume must not fall back to another blocked attempt"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	pl, err := rt.CreatePlan(attached.SessionID, "approval", []plan.StepSpec{{
		StepID: "step_missing_exact_attempt",
		Title:  "resume original blocked attempt or fail",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo fail closed", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
		}},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	initial, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	originalAttempts := mustListAttempts(t, rt, attached.SessionID)
	if len(originalAttempts) != 1 || originalAttempts[0].Status != execution.AttemptBlocked {
		t.Fatalf("expected one original blocked attempt, got %#v", originalAttempts)
	}
	originalAttempt := originalAttempts[0]

	if _, _, err := rt.RespondApproval(initial.Execution.PendingApproval.ApprovalID, approval.Response{Reply: approval.ReplyOnce}); err != nil {
		t.Fatalf("respond approval: %v", err)
	}

	lostOriginal := originalAttempt
	lostOriginal.Status = execution.AttemptFailed
	lostOriginal.FinishedAt = lostOriginal.StartedAt + 1
	if err := rt.Attempts.Update(lostOriginal); err != nil {
		t.Fatalf("mark original attempt missing from blocked context: %v", err)
	}

	shadow := originalAttempt
	shadow.AttemptID = "att_shadow_missing_exact_attempt"
	shadow.TraceID = "trc_shadow_missing_exact_attempt"
	shadow.StartedAt = shadow.StartedAt + 2
	shadow.FinishedAt = 0
	shadow.Status = execution.AttemptBlocked
	if _, err := rt.Attempts.Create(shadow); err != nil {
		t.Fatalf("inject shadow blocked attempt: %v", err)
	}

	if _, err := rt.ResumePendingApproval(context.Background(), attached.SessionID); !errors.Is(err, hruntime.ErrApprovalResumeContextMissing) {
		t.Fatalf("expected ErrApprovalResumeContextMissing, got %v", err)
	}

	attempts := mustListAttempts(t, rt, attached.SessionID)
	for _, attempt := range attempts {
		if attempt.AttemptID == shadow.AttemptID && attempt.Status != execution.AttemptBlocked {
			t.Fatalf("expected shadow blocked attempt to remain untouched after fail-closed resume, got %#v", attempts)
		}
	}
	if handler.calls != 0 {
		t.Fatalf("expected no tool execution when resume context is missing, got %d calls", handler.calls)
	}
}

func TestResumePendingApprovalReusesBlockedAttemptFromRunnerRepositories(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	approvals := approval.NewMemoryStore()
	serviceAttempts := execution.NewMemoryAttemptStore()
	runnerAttempts := execution.NewMemoryAttemptStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	audits := audit.NewMemoryStore()
	handler := &countingHandler{}

	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		handler,
	)
	verifiers.Register(
		verify.Definition{Kind: "exit_code", Description: "Verify that an execution result exit code is in the allowed set."},
		verify.ExitCodeChecker{},
	)

	rt := hruntime.New(hruntime.Options{
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Approvals: approvals,
		Attempts:  serviceAttempts,
		Tools:     tools,
		Verifiers: verifiers,
		Audit:     audits,
		Runner: sinkRunner{repos: persistence.RepositorySet{
			Sessions:  sessions,
			Tasks:     tasks,
			Plans:     plans,
			Approvals: approvals,
			Attempts:  runnerAttempts,
			Audits:    audits,
		}},
	}).WithPolicyEvaluator(askPolicy{})

	sess := mustCreateSession(t, rt, "resume runner attempt", "approval then resume through runner attempts")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "reuse blocked attempt from runner repos"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(sess.SessionID, "approval", []plan.StepSpec{{
		StepID: "step_resume_runner_attempt",
		Title:  "resume through runner attempts",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo approved", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
		}},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	initial, err := rt.RunStep(context.Background(), sess.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	if initial.Execution.PendingApproval == nil {
		t.Fatalf("expected pending approval in execution result")
	}
	if attempts, err := serviceAttempts.List(sess.SessionID); err != nil {
		t.Fatalf("list service attempts: %v", err)
	} else if len(attempts) != 0 {
		t.Fatalf("expected service attempt store to stay empty in mixed setup, got %#v", attempts)
	}
	attempts, err := runnerAttempts.List(sess.SessionID)
	if err != nil {
		t.Fatalf("list runner attempts: %v", err)
	}
	if len(attempts) != 1 || attempts[0].Status != execution.AttemptBlocked {
		t.Fatalf("expected one blocked runner attempt before approval resume, got %#v", attempts)
	}
	blockedAttemptID := attempts[0].AttemptID

	if _, _, err := rt.RespondApproval(initial.Execution.PendingApproval.ApprovalID, approval.Response{Reply: approval.ReplyOnce}); err != nil {
		t.Fatalf("respond approval: %v", err)
	}

	resumed, err := rt.ResumePendingApproval(context.Background(), sess.SessionID)
	if err != nil {
		t.Fatalf("resume pending approval: %v", err)
	}
	if resumed.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected session complete after resumed execution, got %#v", resumed.Session)
	}

	attempts, err = runnerAttempts.List(sess.SessionID)
	if err != nil {
		t.Fatalf("list runner attempts after resume: %v", err)
	}
	if len(attempts) != 1 {
		t.Fatalf("expected resume to reuse the original runner attempt, got %#v", attempts)
	}
	if attempts[0].AttemptID != blockedAttemptID || attempts[0].Status != execution.AttemptCompleted {
		t.Fatalf("expected original runner attempt to complete in place, got %#v", attempts[0])
	}
}

func TestRespondApprovalBestEffortEventEmissionWithoutRunner(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	audits := audit.NewMemoryStore()
	handler := &countingHandler{}

	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		handler,
	)

	rt := hruntime.New(hruntime.Options{
		Sessions: sessions,
		Tasks:    tasks,
		Plans:    plans,
		Tools:    tools,
		Audit:    audits,
		EventSink: selectiveFailingEventSink{failures: map[string]error{
			audit.EventApprovalApproved: errors.New("boom:approval.approved"),
		}},
	}).WithPolicyEvaluator(askPolicy{})
	rt.Runner = nil

	sess := mustCreateSession(t, rt, "best effort approval", "approval responses should stay successful without runner")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "approve without transactional sink"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(sess.SessionID, "approval", []plan.StepSpec{{
		StepID: "step_best_effort_approval",
		Title:  "best effort approval",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo approved", "timeout_ms": 5000}},
		Verify: verify.Spec{},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	initial, err := rt.RunStep(context.Background(), sess.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	if initial.Execution.PendingApproval == nil {
		t.Fatalf("expected pending approval")
	}

	rec, stateAfterReply, err := rt.RespondApproval(initial.Execution.PendingApproval.ApprovalID, approval.Response{Reply: approval.ReplyOnce})
	if err != nil {
		t.Fatalf("expected approval response to stay successful without runner compensation, got %v", err)
	}
	if rec.Status != approval.StatusApproved {
		t.Fatalf("expected approved record, got %#v", rec)
	}
	if stateAfterReply.PendingApprovalID == "" {
		t.Fatalf("expected session to retain pending approval until resume, got %#v", stateAfterReply)
	}

	storedApproval, err := rt.GetApproval(rec.ApprovalID)
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	if storedApproval.Status != approval.StatusApproved {
		t.Fatalf("expected stored approval to remain approved, got %#v", storedApproval)
	}
}

func TestRespondApprovalRejectFailsPendingStepWithoutExecuting(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	audits := audit.NewMemoryStore()
	handler := &countingHandler{}

	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		handler,
	)

	rt := hruntime.New(hruntime.Options{
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Tools:     tools,
		Verifiers: verifiers,
		Audit:     audits,
	}).WithPolicyEvaluator(askPolicy{})

	sess := mustCreateSession(t, rt, "reject approval", "approval rejected path")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "reject approval and fail safely"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(sess.SessionID, "approval reject", []plan.StepSpec{{
		StepID: "step_reject",
		Title:  "reject pending action",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo should-not-run", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	initial, err := rt.RunStep(context.Background(), sess.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	if initial.Execution.PendingApproval == nil {
		t.Fatalf("expected pending approval in execution result")
	}

	approvalRec, stateAfterReply, err := rt.RespondApproval(initial.Execution.PendingApproval.ApprovalID, approval.Response{Reply: approval.ReplyReject})
	if err != nil {
		t.Fatalf("respond approval: %v", err)
	}
	if approvalRec.Status != approval.StatusRejected {
		t.Fatalf("expected rejected approval record, got %#v", approvalRec)
	}
	if handler.calls != 0 {
		t.Fatalf("expected reject path not to execute tool, got %d calls", handler.calls)
	}
	if stateAfterReply.PendingApprovalID != "" {
		t.Fatalf("expected pending approval to clear after reject, got %#v", stateAfterReply)
	}
	if stateAfterReply.Phase != session.PhaseFailed {
		t.Fatalf("expected session failed after reject, got %s", stateAfterReply.Phase)
	}
}

func TestReplyAlwaysAllowsFutureMatchingToolWithoutAnotherApproval(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	audits := audit.NewMemoryStore()
	handler := &countingHandler{}

	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		handler,
	)
	verifiers.Register(
		verify.Definition{Kind: "exit_code", Description: "Verify that an execution result exit code is in the allowed set."},
		verify.ExitCodeChecker{},
	)

	rt := hruntime.New(hruntime.Options{
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Tools:     tools,
		Verifiers: verifiers,
		Audit:     audits,
	}).WithPolicyEvaluator(askPolicy{})

	sess := mustCreateSession(t, rt, "always approval", "reuse approval for matching tool")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "reuse approval"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(sess.SessionID, "always approval", []plan.StepSpec{
		{
			StepID: "step_always_1",
			Title:  "first approval-required shell action",
			Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo first", "timeout_ms": 5000}},
			Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
				{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
			}},
		},
		{
			StepID: "step_always_2",
			Title:  "second identical shell action should reuse approval",
			Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo first", "timeout_ms": 5000}},
			Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
				{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
			}},
		},
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	first, err := rt.RunStep(context.Background(), sess.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run first step: %v", err)
	}
	if first.Execution.PendingApproval == nil {
		t.Fatalf("expected pending approval for first step")
	}

	approvalRec, _, err := rt.RespondApproval(first.Execution.PendingApproval.ApprovalID, approval.Response{Reply: approval.ReplyAlways})
	if err != nil {
		t.Fatalf("respond approval: %v", err)
	}
	if _, err := rt.ResumePendingApproval(context.Background(), sess.SessionID); err != nil {
		t.Fatalf("resume pending approval: %v", err)
	}
	if handler.calls != 1 {
		t.Fatalf("expected first resumed execution to call tool once, got %d", handler.calls)
	}

	second, err := rt.RunStep(context.Background(), sess.SessionID, pl.Steps[1])
	if err != nil {
		t.Fatalf("run second step: %v", err)
	}
	if second.Execution.Policy.Decision.Action != permission.Allow {
		t.Fatalf("expected second step to auto-allow after reply always, got %#v", second.Execution.Policy.Decision)
	}
	if second.Execution.PendingApproval != nil {
		t.Fatalf("did not expect a new pending approval after reply always")
	}
	if handler.calls != 2 {
		t.Fatalf("expected second step to execute immediately, got %d calls", handler.calls)
	}

	storedApproval, err := rt.GetApproval(approvalRec.ApprovalID)
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	if storedApproval.Status != approval.StatusApproved {
		t.Fatalf("expected always approval to remain approved for reuse, got %#v", storedApproval)
	}
}

func TestReplyAlwaysDoesNotReuseApprovalAcrossDifferentArgsOrVersion(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	audits := audit.NewMemoryStore()
	handlerV1 := &countingHandler{}
	handlerV2 := &countingHandler{}

	tools.Register(
		tool.Definition{ToolName: "fs.write", Version: "v1", CapabilityType: "filesystem", RiskLevel: tool.RiskMedium, Enabled: true},
		handlerV1,
	)
	tools.Register(
		tool.Definition{ToolName: "fs.write", Version: "v2", CapabilityType: "filesystem", RiskLevel: tool.RiskMedium, Enabled: true},
		handlerV2,
	)

	rt := hruntime.New(hruntime.Options{
		Sessions: sessions,
		Tasks:    tasks,
		Plans:    plans,
		Tools:    tools,
		Audit:    audits,
	}).WithPolicyEvaluator(scopedAskPolicy{})

	sess := mustCreateSession(t, rt, "always scoped", "approval scope must stay narrow")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "avoid overly broad always approvals"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(sess.SessionID, "scoped always", []plan.StepSpec{
		{
			StepID: "step_first",
			Title:  "write alpha with v1",
			Action: action.Spec{ToolName: "fs.write", ToolVersion: "v1", Args: map[string]any{"path": "/tmp/alpha.txt", "content": "alpha"}},
			Verify: verify.Spec{},
		},
		{
			StepID: "step_second",
			Title:  "write beta with v2",
			Action: action.Spec{ToolName: "fs.write", ToolVersion: "v2", Args: map[string]any{"path": "/tmp/beta.txt", "content": "beta"}},
			Verify: verify.Spec{},
		},
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	first, err := rt.RunStep(context.Background(), sess.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step 1: %v", err)
	}
	if first.Execution.PendingApproval == nil {
		t.Fatalf("expected pending approval on first step")
	}
	if _, _, err := rt.RespondApproval(first.Execution.PendingApproval.ApprovalID, approval.Response{Reply: approval.ReplyAlways}); err != nil {
		t.Fatalf("respond approval: %v", err)
	}
	if _, err := rt.ResumePendingApproval(context.Background(), sess.SessionID); err != nil {
		t.Fatalf("resume pending approval: %v", err)
	}
	if handlerV1.calls != 1 {
		t.Fatalf("expected v1 handler to run once after approved resume, got %d", handlerV1.calls)
	}

	second, err := rt.RunStep(context.Background(), sess.SessionID, pl.Steps[1])
	if err != nil {
		t.Fatalf("run step 2: %v", err)
	}
	if second.Execution.PendingApproval == nil {
		t.Fatalf("expected second step to require a fresh approval instead of reusing reply-always approval")
	}
	if handlerV2.calls != 0 {
		t.Fatalf("expected v2 handler not to execute before fresh approval, got %d", handlerV2.calls)
	}
}

func TestApprovalResolutionKeepsWritingToOriginPlanRevisionWhenNewerRevisionExists(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	audits := audit.NewMemoryStore()

	rt := hruntime.New(hruntime.Options{
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Tools:     tools,
		Verifiers: verifiers,
		Audit:     audits,
	}).WithPolicyEvaluator(askPolicy{})

	sess := mustCreateSession(t, rt, "approval revision binding", "approval responses must mutate the originating plan revision")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "keep revision-bound approval state"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	first, err := rt.CreatePlan(attached.SessionID, "revision 1", []plan.StepSpec{{
		StepID: "step_shared",
		Title:  "revision 1 gated step",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo rev1", "timeout_ms": 5000}},
		Verify: verify.Spec{},
	}})
	if err != nil {
		t.Fatalf("create first plan: %v", err)
	}

	initial, err := rt.RunStep(context.Background(), attached.SessionID, first.Steps[0])
	if err != nil {
		t.Fatalf("run first revision step: %v", err)
	}
	if initial.Execution.PendingApproval == nil {
		t.Fatalf("expected pending approval")
	}

	second, err := rt.CreatePlan(attached.SessionID, "revision 2", []plan.StepSpec{{
		StepID: "step_shared",
		Title:  "revision 2 replacement step",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo rev2", "timeout_ms": 5000}},
		Verify: verify.Spec{},
	}})
	if err != nil {
		t.Fatalf("create second plan: %v", err)
	}

	if _, _, err := rt.RespondApproval(initial.Execution.PendingApproval.ApprovalID, approval.Response{Reply: approval.ReplyReject}); err != nil {
		t.Fatalf("respond approval reject: %v", err)
	}

	storedFirst := mustPlanByRevision(t, rt, attached.SessionID, first.Revision)
	storedSecond := mustPlanByRevision(t, rt, attached.SessionID, second.Revision)
	if storedFirst.Steps[0].Status != plan.StepFailed {
		t.Fatalf("expected originating revision to be failed, got %#v", storedFirst)
	}
	if storedSecond.Steps[0].Status != plan.StepPending {
		t.Fatalf("expected newer revision to remain untouched, got %#v", storedSecond)
	}
}

func TestRejectApprovalFinalizesBlockedAttempt(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	audits := audit.NewMemoryStore()
	handler := &countingHandler{}

	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		handler,
	)

	rt := hruntime.New(hruntime.Options{
		Sessions: sessions,
		Tasks:    tasks,
		Plans:    plans,
		Tools:    tools,
		Audit:    audits,
	}).WithPolicyEvaluator(askPolicy{})

	sess := mustCreateSession(t, rt, "reject blocked attempt", "blocked attempt should be finalized on reject")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "reject approval and reconcile attempt"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(sess.SessionID, "reject blocked attempt", []plan.StepSpec{{
		StepID: "step_blocked",
		Title:  "blocked",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo blocked", "timeout_ms": 5000}},
		Verify: verify.Spec{},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	initial, err := rt.RunStep(context.Background(), sess.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	if initial.Execution.PendingApproval == nil {
		t.Fatalf("expected pending approval")
	}
	attempts := mustListAttempts(t, rt, sess.SessionID)
	if len(attempts) != 1 || attempts[0].Status != "blocked" {
		t.Fatalf("expected one blocked attempt after ask path, got %#v", attempts)
	}

	if _, _, err := rt.RespondApproval(initial.Execution.PendingApproval.ApprovalID, approval.Response{Reply: approval.ReplyReject}); err != nil {
		t.Fatalf("respond approval: %v", err)
	}
	attempts = mustListAttempts(t, rt, sess.SessionID)
	if len(attempts) != 1 {
		t.Fatalf("expected one attempt after reject, got %#v", attempts)
	}
	if attempts[0].Status == "blocked" || attempts[0].FinishedAt == 0 {
		t.Fatalf("expected blocked attempt to be finalized after reject, got %#v", attempts[0])
	}
}

func TestRespondApprovalRejectsInvalidReply(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	audits := audit.NewMemoryStore()
	handler := &countingHandler{}

	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		handler,
	)

	rt := hruntime.New(hruntime.Options{
		Sessions: sessions,
		Tasks:    tasks,
		Plans:    plans,
		Tools:    tools,
		Audit:    audits,
	}).WithPolicyEvaluator(askPolicy{})

	sess := mustCreateSession(t, rt, "invalid reply", "reject unknown approval replies")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "approval replies must be validated"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(sess.SessionID, "invalid reply", []plan.StepSpec{{
		StepID: "step_invalid_reply",
		Title:  "pending approval",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo invalid", "timeout_ms": 5000}},
		Verify: verify.Spec{},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	initial, err := rt.RunStep(context.Background(), sess.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	if initial.Execution.PendingApproval == nil {
		t.Fatalf("expected pending approval")
	}

	approvalID := initial.Execution.PendingApproval.ApprovalID
	if _, _, err := rt.RespondApproval(approvalID, approval.Response{Reply: approval.Reply("bogus")}); !errors.Is(err, approval.ErrInvalidReply) {
		t.Fatalf("expected ErrInvalidReply, got %v", err)
	}

	storedApproval, err := rt.GetApproval(approvalID)
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	if storedApproval.Status != approval.StatusPending || storedApproval.Reply != "" {
		t.Fatalf("expected pending approval to remain unchanged, got %#v", storedApproval)
	}
	if handler.calls != 0 {
		t.Fatalf("did not expect tool execution on invalid reply, got %d calls", handler.calls)
	}
}

func TestRespondApprovalRejectsNonPendingApprovalAndCannotBroadenConsumedApproval(t *testing.T) {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	audits := audit.NewMemoryStore()
	handler := &countingHandler{}

	tools.Register(
		tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true},
		handler,
	)

	rt := hruntime.New(hruntime.Options{
		Sessions: sessions,
		Tasks:    tasks,
		Plans:    plans,
		Tools:    tools,
		Audit:    audits,
	}).WithPolicyEvaluator(askPolicy{})

	sess := mustCreateSession(t, rt, "non-pending reply", "consumed approvals must stay immutable")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "old approvals must not be broadened"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(sess.SessionID, "consumed approval", []plan.StepSpec{
		{
			StepID: "step_first",
			Title:  "first gated step",
			Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo first", "timeout_ms": 5000}},
			Verify: verify.Spec{},
		},
		{
			StepID: "step_second",
			Title:  "second identical step",
			Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo first", "timeout_ms": 5000}},
			Verify: verify.Spec{},
		},
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	initial, err := rt.RunStep(context.Background(), sess.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run first step: %v", err)
	}
	if initial.Execution.PendingApproval == nil {
		t.Fatalf("expected pending approval")
	}

	approvalID := initial.Execution.PendingApproval.ApprovalID
	if _, _, err := rt.RespondApproval(approvalID, approval.Response{Reply: approval.ReplyOnce}); err != nil {
		t.Fatalf("respond approval once: %v", err)
	}
	if _, err := rt.ResumePendingApproval(context.Background(), sess.SessionID); err != nil {
		t.Fatalf("resume pending approval: %v", err)
	}

	storedApproval, err := rt.GetApproval(approvalID)
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	if storedApproval.Status != approval.StatusConsumed {
		t.Fatalf("expected consumed approval, got %#v", storedApproval)
	}

	if _, _, err := rt.RespondApproval(approvalID, approval.Response{Reply: approval.ReplyAlways}); !errors.Is(err, approval.ErrApprovalNotPending) {
		t.Fatalf("expected ErrApprovalNotPending when re-responding consumed approval, got %v", err)
	}

	second, err := rt.RunStep(context.Background(), sess.SessionID, pl.Steps[1])
	if err != nil {
		t.Fatalf("run second step: %v", err)
	}
	if second.Execution.PendingApproval == nil {
		t.Fatalf("expected second step to require fresh approval")
	}
	if second.Execution.Policy.Decision.Action != permission.Ask {
		t.Fatalf("expected second step to stay on ask path, got %#v", second.Execution.Policy.Decision)
	}
	if handler.calls != 1 {
		t.Fatalf("expected only the resumed first step to execute, got %d calls", handler.calls)
	}
}
