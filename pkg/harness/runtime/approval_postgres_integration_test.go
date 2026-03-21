package runtime_test

import (
	"context"
	"testing"
	"time"

	"github.com/yiiilin/harness-core/internal/postgrestest"
	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/builtins"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

func TestApprovalFlowPersistsAcrossPostgresRuntimeReinit(t *testing.T) {
	pg := postgrestest.Start(t)

	opts := hruntime.Options{}
	builtins.Register(&opts)
	opts.Policy = askPolicy{}

	rt1, db1 := pg.OpenService(t, opts)

	sess := mustCreateSession(t, rt1, "postgres approval", "persist approval across runtime restart")
	tsk := mustCreateTask(t, rt1, task.Spec{TaskType: "demo", Goal: "approval persists durably"})
	sess, err := rt1.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt1.CreatePlan(sess.SessionID, "approval durable", []plan.StepSpec{{
		StepID: "step_pg_approval",
		Title:  "durable approval step",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo durable", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
		}},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	initial, err := rt1.RunStep(context.Background(), sess.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	if initial.Execution.PendingApproval == nil {
		t.Fatalf("expected pending approval from postgres-backed ask path")
	}

	approvalRec, _, err := rt1.RespondApproval(initial.Execution.PendingApproval.ApprovalID, approval.Response{Reply: approval.ReplyOnce})
	if err != nil {
		t.Fatalf("respond approval: %v", err)
	}

	if err := db1.Close(); err != nil {
		t.Fatalf("close first db: %v", err)
	}

	rt2, db2 := pg.OpenService(t, opts)
	defer db2.Close()

	resumed, err := rt2.ResumePendingApproval(context.Background(), sess.SessionID)
	if err != nil {
		t.Fatalf("resume pending approval after reinit: %v", err)
	}
	if resumed.Session.PendingApprovalID != "" {
		t.Fatalf("expected pending approval cleared after resumed durable execution, got %#v", resumed.Session)
	}

	storedApproval, err := rt2.GetApproval(approvalRec.ApprovalID)
	if err != nil {
		t.Fatalf("get stored approval: %v", err)
	}
	if storedApproval.Status != approval.StatusConsumed {
		t.Fatalf("expected consumed approval after durable resume, got %#v", storedApproval)
	}
}

func TestPostgresRunClaimedSessionResumesApprovedPendingApproval(t *testing.T) {
	pg := postgrestest.Start(t)

	opts := hruntime.Options{}
	builtins.Register(&opts)
	opts.Policy = askPolicy{}

	rt, db := pg.OpenService(t, opts)
	defer db.Close()

	sess := mustCreateSession(t, rt, "postgres claimed approval", "claimed session driver should resume approved work")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "claimed postgres approval resume"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt.CreatePlan(sess.SessionID, "approval durable claimed", []plan.StepSpec{{
		StepID: "step_pg_claimed_approval",
		Title:  "durable claimed approval step",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo durable claimed", "timeout_ms": 5000}},
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
		t.Fatalf("expected pending approval from postgres-backed ask path")
	}
	if _, _, err := rt.RespondApproval(initial.Execution.PendingApproval.ApprovalID, approval.Response{Reply: approval.ReplyOnce}); err != nil {
		t.Fatalf("respond approval: %v", err)
	}

	claimed, ok, err := rt.ClaimRunnableSession(context.Background(), time.Minute)
	if err != nil {
		t.Fatalf("claim runnable session: %v", err)
	}
	if !ok || claimed.SessionID != sess.SessionID {
		t.Fatalf("expected claimed session %s, got %#v ok=%v", sess.SessionID, claimed, ok)
	}

	out, err := rt.RunClaimedSession(context.Background(), sess.SessionID, claimed.LeaseID)
	if err != nil {
		t.Fatalf("run claimed session: %v", err)
	}
	if out.Session.PendingApprovalID != "" || out.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected claimed session run to consume approval and complete, got %#v", out.Session)
	}
}

func TestExecutionFactsPersistAcrossPostgresRuntimeReinit(t *testing.T) {
	pg := postgrestest.Start(t)

	opts := hruntime.Options{}
	builtins.Register(&opts)

	rt1, db1 := pg.OpenService(t, opts)

	sess := mustCreateSession(t, rt1, "postgres execution facts", "persist attempts and events")
	tsk := mustCreateTask(t, rt1, task.Spec{TaskType: "demo", Goal: "execution facts should survive runtime restart"})
	sess, err := rt1.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	step := plan.StepSpec{
		StepID: "step_pg_execution",
		Title:  "durable execution facts",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo durable facts", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
		}},
	}

	out, err := rt1.RunStep(context.Background(), sess.SessionID, step)
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	if attempts := mustListAttempts(t, rt1, sess.SessionID); len(attempts) != 1 {
		t.Fatalf("expected attempts in first runtime, got %#v", attempts)
	}
	if actions := mustListActions(t, rt1, sess.SessionID); len(actions) != 1 {
		t.Fatalf("expected actions in first runtime, got %#v", actions)
	}
	if verifications := mustListVerifications(t, rt1, sess.SessionID); len(verifications) != 1 {
		t.Fatalf("expected verifications in first runtime, got %#v", verifications)
	}
	if artifacts := mustListArtifacts(t, rt1, sess.SessionID); len(artifacts) == 0 {
		t.Fatalf("expected artifacts in first runtime, got %#v", artifacts)
	}
	if len(out.Events) == 0 || out.Events[0].AttemptID == "" || out.Events[0].TaskID == "" || out.Events[0].TraceID == "" {
		t.Fatalf("expected rich event envelope in first runtime, got %#v", out.Events)
	}

	if err := db1.Close(); err != nil {
		t.Fatalf("close first db: %v", err)
	}

	rt2, db2 := pg.OpenService(t, opts)
	defer db2.Close()

	if attempts := mustListAttempts(t, rt2, sess.SessionID); len(attempts) != 1 {
		t.Fatalf("expected durable attempts after reinit, got %#v", attempts)
	}
	if actions := mustListActions(t, rt2, sess.SessionID); len(actions) != 1 {
		t.Fatalf("expected durable actions after reinit, got %#v", actions)
	}
	if verifications := mustListVerifications(t, rt2, sess.SessionID); len(verifications) != 1 {
		t.Fatalf("expected durable verifications after reinit, got %#v", verifications)
	}
	if artifacts := mustListArtifacts(t, rt2, sess.SessionID); len(artifacts) == 0 {
		t.Fatalf("expected durable artifacts after reinit, got %#v", artifacts)
	}

	events := mustListAuditEvents(t, rt2, sess.SessionID)
	foundRichEnvelope := false
	for _, event := range events {
		if event.AttemptID != "" && event.TaskID != "" && event.TraceID != "" {
			foundRichEnvelope = true
			break
		}
	}
	if !foundRichEnvelope {
		t.Fatalf("expected durable rich audit envelope after reinit, got %#v", events)
	}
}

func TestApprovedPendingApprovalRecoveryPreservesExecutionCycleAcrossPostgresReinit(t *testing.T) {
	pg := postgrestest.Start(t)

	opts := hruntime.Options{}
	builtins.Register(&opts)
	opts.Policy = askPolicy{}

	rt1, db1 := pg.OpenService(t, opts)

	sess := mustCreateSession(t, rt1, "postgres cycle", "preserve logical execution cycle across reinit")
	tsk := mustCreateTask(t, rt1, task.Spec{TaskType: "demo", Goal: "recover one logical execution cycle"})
	sess, err := rt1.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, err := rt1.CreatePlan(sess.SessionID, "approval durable cycle", []plan.StepSpec{{
		StepID: "step_pg_cycle",
		Title:  "durable approval cycle",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo durable cycle", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{
			{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
		}},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	initial, err := rt1.RunStep(context.Background(), sess.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	if initial.Execution.PendingApproval == nil {
		t.Fatalf("expected pending approval")
	}

	attemptsBefore := mustListAttempts(t, rt1, sess.SessionID)
	if len(attemptsBefore) != 1 || attemptsBefore[0].CycleID == "" {
		t.Fatalf("expected one blocked attempt with cycle_id, got %#v", attemptsBefore)
	}
	cycleID := attemptsBefore[0].CycleID

	if _, _, err := rt1.RespondApproval(initial.Execution.PendingApproval.ApprovalID, approval.Response{Reply: approval.ReplyOnce}); err != nil {
		t.Fatalf("respond approval: %v", err)
	}

	if err := db1.Close(); err != nil {
		t.Fatalf("close first db: %v", err)
	}

	rt2, db2 := pg.OpenService(t, opts)
	defer db2.Close()

	resumed, err := rt2.ResumePendingApproval(context.Background(), sess.SessionID)
	if err != nil {
		t.Fatalf("resume pending approval after reinit: %v", err)
	}
	if resumed.Session.PendingApprovalID != "" {
		t.Fatalf("expected pending approval cleared after resume, got %#v", resumed.Session)
	}

	attempts := mustListAttempts(t, rt2, sess.SessionID)
	if len(attempts) != 1 || attempts[0].CycleID != cycleID {
		t.Fatalf("expected durable attempt to retain cycle_id %q, got %#v", cycleID, attempts)
	}
	actions := mustListActions(t, rt2, sess.SessionID)
	if len(actions) != 1 || actions[0].CycleID != cycleID {
		t.Fatalf("expected durable action to retain cycle_id %q, got %#v", cycleID, actions)
	}
	verifications := mustListVerifications(t, rt2, sess.SessionID)
	if len(verifications) != 1 || verifications[0].CycleID != cycleID {
		t.Fatalf("expected durable verification to retain cycle_id %q, got %#v", cycleID, verifications)
	}
	artifacts := mustListArtifacts(t, rt2, sess.SessionID)
	if len(artifacts) == 0 {
		t.Fatalf("expected durable artifacts")
	}
	for _, artifact := range artifacts {
		if artifact.CycleID != cycleID {
			t.Fatalf("expected durable artifact to retain cycle_id %q, got %#v", cycleID, artifact)
		}
	}
}
