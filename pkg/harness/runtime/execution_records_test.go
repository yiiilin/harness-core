package runtime_test

import (
	"context"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
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
