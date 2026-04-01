package runtime_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

func TestRunStepPersistsExecutionFactsAndRichEventEnvelope(t *testing.T) {
	rt, sess, step := newHappyRuntime(t)

	out, err := rt.RunStep(context.Background(), sess.SessionID, step)
	if err != nil {
		t.Fatalf("run step: %v", err)
	}

	attempts := mustListAttempts(t, rt, sess.SessionID)
	if len(attempts) != 1 {
		t.Fatalf("expected one attempt record, got %#v", attempts)
	}
	attempt := attempts[0]
	if attempt.AttemptID == "" || attempt.TaskID == "" || attempt.TraceID == "" {
		t.Fatalf("expected attempt identifiers to be populated, got %#v", attempt)
	}

	actions := mustListActions(t, rt, sess.SessionID)
	if len(actions) != 1 {
		t.Fatalf("expected one action record, got %#v", actions)
	}
	actionRec := actions[0]
	if actionRec.ActionID == "" || actionRec.AttemptID != attempt.AttemptID || actionRec.TraceID != attempt.TraceID {
		t.Fatalf("expected action record to link to attempt, got %#v", actionRec)
	}

	verifications := mustListVerifications(t, rt, sess.SessionID)
	if len(verifications) != 1 {
		t.Fatalf("expected one verification record, got %#v", verifications)
	}
	verifyRec := verifications[0]
	if verifyRec.VerificationID == "" || verifyRec.AttemptID != attempt.AttemptID || verifyRec.TraceID != attempt.TraceID {
		t.Fatalf("expected verification record to link to attempt, got %#v", verifyRec)
	}

	artifacts := mustListArtifacts(t, rt, sess.SessionID)
	if len(artifacts) == 0 {
		t.Fatalf("expected at least one artifact record for execution output")
	}

	for _, event := range out.Events {
		if event.TaskID == "" || event.AttemptID == "" || event.TraceID == "" {
			t.Fatalf("expected event envelope ids on every event, got %#v", event)
		}
		switch event.Type {
		case audit.EventToolCalled, audit.EventToolCompleted, audit.EventToolFailed:
			if event.ActionID == "" {
				t.Fatalf("expected action_id on tool event, got %#v", event)
			}
			if event.CausationID == "" {
				t.Fatalf("expected causation_id on tool event, got %#v", event)
			}
		case audit.EventVerifyCompleted:
			if event.VerificationID == "" {
				t.Fatalf("expected verification_id on verify event, got %#v", event)
			}
			if event.CausationID == "" {
				t.Fatalf("expected causation_id on verify event, got %#v", event)
			}
		}
	}

	storedEvents := mustListAuditEvents(t, rt, sess.SessionID)
	var storedVerify *audit.Event
	for i := range storedEvents {
		if storedEvents[i].Type == audit.EventVerifyCompleted {
			storedVerify = &storedEvents[i]
			break
		}
	}
	if storedVerify == nil {
		t.Fatalf("expected persisted verify event, got %#v", storedEvents)
	}
	if storedVerify.VerificationID != verifyRec.VerificationID {
		t.Fatalf("expected persisted verify event to retain verification_id %q, got %#v", verifyRec.VerificationID, storedVerify)
	}
	if cycleID, ok := auditEventStringField(*storedVerify, "CycleID"); !ok || cycleID != attempt.CycleID {
		t.Fatalf("expected persisted verify event to expose cycle correlation %q, got %#v", attempt.CycleID, storedVerify)
	}
}

func TestApprovalResumePersistsOneLogicalExecutionCycleAcrossExecutionFacts(t *testing.T) {
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

	sess := mustCreateSession(t, rt, "cycle coherence", "approval and execution facts should stay in one logical cycle")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "preserve one logical cycle"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(attached.SessionID, "cycle coherence", []plan.StepSpec{{
		StepID: "step_cycle",
		Title:  "approval gated step",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo cycle", "timeout_ms": 5000}},
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
	if initial.Execution.PendingApproval == nil {
		t.Fatalf("expected pending approval")
	}

	attempts := mustListAttempts(t, rt, attached.SessionID)
	if len(attempts) != 1 {
		t.Fatalf("expected one blocked attempt, got %#v", attempts)
	}
	blockedAttempt := attempts[0]
	if blockedAttempt.CycleID == "" {
		t.Fatalf("expected blocked attempt cycle_id, got %#v", blockedAttempt)
	}

	storedApproval, err := rt.GetApproval(initial.Execution.PendingApproval.ApprovalID)
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	if storedApproval.Step.Metadata["execution_cycle_id"] != blockedAttempt.CycleID {
		t.Fatalf("expected approval step metadata to carry attempt cycle_id, got %#v and %#v", storedApproval.Step.Metadata, blockedAttempt)
	}

	if _, _, err := rt.RespondApproval(storedApproval.ApprovalID, approval.Response{Reply: approval.ReplyOnce}); err != nil {
		t.Fatalf("respond approval: %v", err)
	}
	if _, err := rt.ResumePendingApproval(context.Background(), attached.SessionID); err != nil {
		t.Fatalf("resume approval: %v", err)
	}

	attempts = mustListAttempts(t, rt, attached.SessionID)
	if len(attempts) != 1 {
		t.Fatalf("expected one reused attempt, got %#v", attempts)
	}
	if attempts[0].CycleID != blockedAttempt.CycleID {
		t.Fatalf("expected attempt cycle_id to stay stable across approval resume, got before=%q after=%q", blockedAttempt.CycleID, attempts[0].CycleID)
	}

	actions := mustListActions(t, rt, attached.SessionID)
	if len(actions) != 1 || actions[0].CycleID != blockedAttempt.CycleID {
		t.Fatalf("expected action cycle_id to match blocked attempt, got %#v", actions)
	}

	verifications := mustListVerifications(t, rt, attached.SessionID)
	if len(verifications) != 1 || verifications[0].CycleID != blockedAttempt.CycleID {
		t.Fatalf("expected verification cycle_id to match blocked attempt, got %#v", verifications)
	}

	artifacts := mustListArtifacts(t, rt, attached.SessionID)
	if len(artifacts) == 0 {
		t.Fatalf("expected artifacts")
	}
	for _, artifact := range artifacts {
		if artifact.CycleID != blockedAttempt.CycleID {
			t.Fatalf("expected artifact cycle_id to match blocked attempt, got %#v", artifact)
		}
	}
}

func TestApprovalAuditEventsExposeApprovalCorrelation(t *testing.T) {
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

	sess := mustCreateSession(t, rt, "approval audit", "approval events should stay correlated")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "keep approval audit correlated"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(attached.SessionID, "approval audit", []plan.StepSpec{{
		StepID: "step_approval_audit",
		Title:  "approval audit step",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo approval", "timeout_ms": 5000}},
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
	if initial.Execution.PendingApproval == nil {
		t.Fatalf("expected pending approval")
	}

	attempts := mustListAttempts(t, rt, attached.SessionID)
	if len(attempts) != 1 {
		t.Fatalf("expected one blocked attempt, got %#v", attempts)
	}

	requestEvents := mustListAuditEvents(t, rt, attached.SessionID)
	var requestEvent *audit.Event
	for i := range requestEvents {
		if requestEvents[i].Type == audit.EventApprovalRequested {
			requestEvent = &requestEvents[i]
			break
		}
	}
	if requestEvent == nil {
		t.Fatalf("expected approval.requested event, got %#v", requestEvents)
	}
	if approvalID, ok := auditEventStringField(*requestEvent, "ApprovalID"); !ok || approvalID != initial.Execution.PendingApproval.ApprovalID {
		t.Fatalf("expected approval.requested event to expose approval_id %q, got %#v", initial.Execution.PendingApproval.ApprovalID, requestEvent)
	}
	if cycleID, ok := auditEventStringField(*requestEvent, "CycleID"); !ok || cycleID != attempts[0].CycleID {
		t.Fatalf("expected approval.requested event to expose cycle_id %q, got %#v", attempts[0].CycleID, requestEvent)
	}

	if _, _, err := rt.RespondApproval(initial.Execution.PendingApproval.ApprovalID, approval.Response{Reply: approval.ReplyOnce}); err != nil {
		t.Fatalf("respond approval: %v", err)
	}

	responseEvents := mustListAuditEvents(t, rt, attached.SessionID)
	var responseEvent *audit.Event
	for i := range responseEvents {
		if responseEvents[i].Type == audit.EventApprovalApproved {
			responseEvent = &responseEvents[i]
		}
	}
	if responseEvent == nil {
		t.Fatalf("expected approval.approved event, got %#v", responseEvents)
	}
	if responseEvent.TaskID != attached.TaskID || responseEvent.TraceID == "" || responseEvent.CausationID == "" {
		t.Fatalf("expected approval.approved event to retain task/trace/causation correlation, got %#v", responseEvent)
	}
	if approvalID, ok := auditEventStringField(*responseEvent, "ApprovalID"); !ok || approvalID != initial.Execution.PendingApproval.ApprovalID {
		t.Fatalf("expected approval.approved event to expose approval_id %q, got %#v", initial.Execution.PendingApproval.ApprovalID, responseEvent)
	}
}

func TestRunStepPreservesRawActionResultAlongsideTrimmedInlineResult(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(
		tool.Definition{ToolName: "demo.long-output", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true},
		longOutputHandler{stdout: "abcdefghijklmnopqrstuvwxyz"},
	)

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

	sess := mustCreateSession(t, rt, "raw result", "preserve raw action output")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "preserve raw action output"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(attached.SessionID, "raw result", []plan.StepSpec{{
		StepID: "step_raw_result",
		Title:  "raw result",
		Action: action.Spec{ToolName: "demo.long-output"},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	out, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}

	inlineStdout, _ := out.Execution.Action.Data["stdout"].(string)
	if inlineStdout != "abcdefgh" {
		t.Fatalf("expected inline stdout to be trimmed, got %#v", out.Execution.Action)
	}
	if !out.Execution.Action.WasTrimmed {
		t.Fatalf("expected action result to report trimming, got %#v", out.Execution.Action)
	}
	if out.Execution.Action.Raw == nil {
		t.Fatalf("expected raw action result channel, got %#v", out.Execution.Action)
	}
	rawStdout, _ := out.Execution.Action.Raw.Data["stdout"].(string)
	if rawStdout != "abcdefghijklmnopqrstuvwxyz" {
		t.Fatalf("expected raw stdout to stay intact, got %#v", out.Execution.Action.Raw)
	}

	actions := mustListActions(t, rt, attached.SessionID)
	if len(actions) != 1 {
		t.Fatalf("expected one action record, got %#v", actions)
	}
	storedInline, _ := actions[0].Result.Data["stdout"].(string)
	if storedInline != "abcdefgh" {
		t.Fatalf("expected stored inline stdout to stay trimmed, got %#v", actions[0].Result)
	}
	if actions[0].Result.Raw == nil {
		t.Fatalf("expected stored raw action result, got %#v", actions[0].Result)
	}
	storedRaw, _ := actions[0].Result.Raw.Data["stdout"].(string)
	if storedRaw != "abcdefghijklmnopqrstuvwxyz" {
		t.Fatalf("expected stored raw stdout to stay intact, got %#v", actions[0].Result.Raw)
	}

	artifacts := mustListArtifacts(t, rt, attached.SessionID)
	if len(artifacts) == 0 {
		t.Fatalf("expected action result artifact")
	}
	payload, _ := artifacts[0].Payload["data"].(map[string]any)
	artifactStdout, _ := payload["stdout"].(string)
	if artifactStdout != "abcdefghijklmnopqrstuvwxyz" {
		t.Fatalf("expected artifact payload to keep raw stdout, got %#v", artifacts[0])
	}
}

func TestRunStepVerificationUsesRawActionResultWhenInlineTrimmed(t *testing.T) {
	tools := tool.NewRegistry()
	tools.Register(
		tool.Definition{ToolName: "demo.long-output", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskLow, Enabled: true},
		longOutputHandler{stdout: "prefix-" + strings.Repeat("x", 20) + "-needle"},
	)
	verifiers := verify.NewRegistry()
	verify.RegisterBuiltins(verifiers)

	rt := hruntime.New(hruntime.Options{
		Tools:     tools,
		Verifiers: verifiers,
		Policy:    permission.DefaultEvaluator{},
		LoopBudgets: hruntime.LoopBudgets{
			MaxSteps:           8,
			MaxRetriesPerStep:  3,
			MaxPlanRevisions:   8,
			MaxTotalRuntimeMS:  60000,
			MaxToolOutputChars: 8,
		},
	})

	sess := mustCreateSession(t, rt, "raw verify", "verify against raw action output")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "verify against raw action output"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(attached.SessionID, "raw verify", []plan.StepSpec{{
		StepID: "step_raw_verify",
		Title:  "raw verify",
		Action: action.Spec{ToolName: "demo.long-output"},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "output_contains", Args: map[string]any{"text": "-needle"}},
		}},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	out, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	if !out.Execution.Verify.Success {
		t.Fatalf("expected verification to read raw output beyond inline trim, got %#v", out.Execution.Verify)
	}

	verifications := mustListVerifications(t, rt, attached.SessionID)
	if len(verifications) != 1 || !verifications[0].Result.Success {
		t.Fatalf("expected persisted verification success, got %#v", verifications)
	}
}

func auditEventStringField(event audit.Event, field string) (string, bool) {
	value := reflect.ValueOf(event).FieldByName(field)
	if !value.IsValid() || value.Kind() != reflect.String {
		return "", false
	}
	return value.String(), true
}

type longOutputHandler struct {
	stdout string
}

func (h longOutputHandler) Invoke(_ context.Context, _ map[string]any) (action.Result, error) {
	return action.Result{
		OK: true,
		Data: map[string]any{
			"stdout": h.stdout,
		},
	}, nil
}
