package runtime_test

import (
	"context"
	"testing"

	"github.com/yiiilin/harness-core/internal/postgrestest"
	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

func TestApprovalFlowPersistsAcrossPostgresRuntimeReinit(t *testing.T) {
	pg := postgrestest.Start(t)

	opts := hruntime.Options{}
	hruntime.RegisterBuiltins(&opts)
	opts.Policy = askPolicy{}

	rt1, db1 := pg.OpenService(t, opts)

	sess := rt1.CreateSession("postgres approval", "persist approval across runtime restart")
	tsk := rt1.CreateTask(task.Spec{TaskType: "demo", Goal: "approval persists durably"})
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

func TestExecutionFactsPersistAcrossPostgresRuntimeReinit(t *testing.T) {
	pg := postgrestest.Start(t)

	opts := hruntime.Options{}
	hruntime.RegisterBuiltins(&opts)

	rt1, db1 := pg.OpenService(t, opts)

	sess := rt1.CreateSession("postgres execution facts", "persist attempts and events")
	tsk := rt1.CreateTask(task.Spec{TaskType: "demo", Goal: "execution facts should survive runtime restart"})
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
	if len(rt1.ListAttempts(sess.SessionID)) != 1 {
		t.Fatalf("expected attempts in first runtime, got %#v", rt1.ListAttempts(sess.SessionID))
	}
	if len(rt1.ListActions(sess.SessionID)) != 1 {
		t.Fatalf("expected actions in first runtime, got %#v", rt1.ListActions(sess.SessionID))
	}
	if len(rt1.ListVerifications(sess.SessionID)) != 1 {
		t.Fatalf("expected verifications in first runtime, got %#v", rt1.ListVerifications(sess.SessionID))
	}
	if len(rt1.ListArtifacts(sess.SessionID)) == 0 {
		t.Fatalf("expected artifacts in first runtime, got %#v", rt1.ListArtifacts(sess.SessionID))
	}
	if len(out.Events) == 0 || out.Events[0].AttemptID == "" || out.Events[0].TaskID == "" || out.Events[0].TraceID == "" {
		t.Fatalf("expected rich event envelope in first runtime, got %#v", out.Events)
	}

	if err := db1.Close(); err != nil {
		t.Fatalf("close first db: %v", err)
	}

	rt2, db2 := pg.OpenService(t, opts)
	defer db2.Close()

	if len(rt2.ListAttempts(sess.SessionID)) != 1 {
		t.Fatalf("expected durable attempts after reinit, got %#v", rt2.ListAttempts(sess.SessionID))
	}
	if len(rt2.ListActions(sess.SessionID)) != 1 {
		t.Fatalf("expected durable actions after reinit, got %#v", rt2.ListActions(sess.SessionID))
	}
	if len(rt2.ListVerifications(sess.SessionID)) != 1 {
		t.Fatalf("expected durable verifications after reinit, got %#v", rt2.ListVerifications(sess.SessionID))
	}
	if len(rt2.ListArtifacts(sess.SessionID)) == 0 {
		t.Fatalf("expected durable artifacts after reinit, got %#v", rt2.ListArtifacts(sess.SessionID))
	}

	events := rt2.ListAuditEvents(sess.SessionID)
	if len(events) == 0 || events[0].AttemptID == "" || events[0].TaskID == "" || events[0].TraceID == "" {
		t.Fatalf("expected durable rich audit envelope after reinit, got %#v", events)
	}
}
