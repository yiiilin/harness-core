package runtime_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yiiilin/harness-core/pkg/harness/execution"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

func TestCreateBlockedRuntimePersistsGenericBlockedSessionState(t *testing.T) {
	rt := hruntime.New(hruntime.Options{})

	sess := mustCreateSession(t, rt, "generic blocked runtime", "create external blocked runtime")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "create external blocked runtime"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	record, updated, err := rt.CreateBlockedRuntime(context.Background(), attached.SessionID, hruntime.BlockedRuntimeRequest{
		Kind: execution.BlockedRuntimeExternal,
		Subject: execution.BlockedRuntimeSubject{
			StepID:    "step_wait_external",
			ActionID:  "act_wait_external",
			AttemptID: "att_wait_external",
			CycleID:   "cyc_wait_external",
		},
		Condition: execution.BlockedRuntimeCondition{
			Kind:       execution.BlockedRuntimeConditionExternal,
			WaitingFor: "external_signal",
			Metadata:   map[string]any{"source": "operator"},
		},
		Metadata: map[string]any{"ticket": "ops-42"},
	})
	if err != nil {
		t.Fatalf("create blocked runtime: %v", err)
	}
	if record.Kind != execution.BlockedRuntimeExternal || record.Status != execution.BlockedRuntimePending {
		t.Fatalf("unexpected blocked runtime record: %#v", record)
	}
	if updated.ExecutionState != session.ExecutionBlocked {
		t.Fatalf("expected session execution blocked, got %#v", updated)
	}

	stored, err := rt.GetBlockedRuntime(attached.SessionID)
	if err != nil {
		t.Fatalf("get blocked runtime: %v", err)
	}
	if stored.BlockedRuntimeID != record.BlockedRuntimeID || stored.Kind != execution.BlockedRuntimeExternal {
		t.Fatalf("unexpected stored blocked runtime: %#v", stored)
	}
	if stored.WaitingFor != "external_signal" || stored.StepID != "step_wait_external" || stored.AttemptID != "att_wait_external" {
		t.Fatalf("unexpected blocked runtime linkage fields: %#v", stored)
	}

	byID, err := rt.GetBlockedRuntimeByID(record.BlockedRuntimeID)
	if err != nil {
		t.Fatalf("get blocked runtime by id: %v", err)
	}
	if byID.BlockedRuntimeID != stored.BlockedRuntimeID || byID.SessionID != stored.SessionID {
		t.Fatalf("expected id lookup to match session lookup, got session=%#v byID=%#v", stored, byID)
	}

	records, err := rt.ListBlockedRuntimeRecords(attached.SessionID)
	if err != nil {
		t.Fatalf("list blocked runtime records: %v", err)
	}
	if len(records) != 1 || records[0].BlockedRuntimeID != record.BlockedRuntimeID {
		t.Fatalf("expected one generic blocked runtime record, got %#v", records)
	}

	claimed, ok, err := rt.ClaimRunnableSession(context.Background(), time.Minute)
	if err != nil {
		t.Fatalf("claim runnable session: %v", err)
	}
	if ok {
		t.Fatalf("expected blocked session to be skipped by runnable claims, got %#v", claimed)
	}
}

func TestRespondAndResumeBlockedRuntimeClearsGenericBlockedState(t *testing.T) {
	rt := hruntime.New(hruntime.Options{})

	sess := mustCreateSession(t, rt, "resume generic blocked runtime", "respond and resume blocked runtime")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "respond and resume blocked runtime"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	record, _, err := rt.CreateBlockedRuntime(context.Background(), attached.SessionID, hruntime.BlockedRuntimeRequest{
		Kind: execution.BlockedRuntimeConfirmation,
		Subject: execution.BlockedRuntimeSubject{
			StepID:  "step_confirm",
			CycleID: "cyc_confirm",
			Target:  execution.TargetRef{TargetID: "host-a", Kind: "host"},
		},
		Condition: execution.BlockedRuntimeCondition{
			Kind:       execution.BlockedRuntimeConditionConfirmation,
			WaitingFor: "human_confirmation",
		},
	})
	if err != nil {
		t.Fatalf("create blocked runtime: %v", err)
	}

	responded, blockedState, err := rt.RespondBlockedRuntime(context.Background(), record.BlockedRuntimeID, hruntime.BlockedRuntimeResponse{
		Status:   execution.BlockedRuntimeConfirmed,
		Metadata: map[string]any{"actor": "reviewer"},
	})
	if err != nil {
		t.Fatalf("respond blocked runtime: %v", err)
	}
	if responded.Status != execution.BlockedRuntimeConfirmed {
		t.Fatalf("expected confirmed status, got %#v", responded)
	}
	if blockedState.ExecutionState != session.ExecutionBlocked {
		t.Fatalf("expected session to remain blocked until explicit resume, got %#v", blockedState)
	}

	projection, err := rt.GetBlockedRuntimeProjection(attached.SessionID)
	if err != nil {
		t.Fatalf("get blocked runtime projection: %v", err)
	}
	if projection.Runtime.BlockedRuntimeID != record.BlockedRuntimeID || !projection.Wait.ReferencesTarget() {
		t.Fatalf("expected generic blocked runtime projection with target wait scope, got %#v", projection)
	}

	resumed, resumedState, err := rt.ResumeBlockedRuntime(context.Background(), record.BlockedRuntimeID)
	if err != nil {
		t.Fatalf("resume blocked runtime: %v", err)
	}
	if resumed.Status != execution.BlockedRuntimeResumed {
		t.Fatalf("expected resumed blocked runtime status, got %#v", resumed)
	}
	if resumedState.ExecutionState != session.ExecutionIdle {
		t.Fatalf("expected session to return to idle after resume, got %#v", resumedState)
	}

	_, err = rt.GetBlockedRuntime(attached.SessionID)
	if !errors.Is(err, execution.ErrBlockedRuntimeNotFound) {
		t.Fatalf("expected no current blocked runtime after resume, got %v", err)
	}

	claimed, ok, err := rt.ClaimRunnableSession(context.Background(), time.Minute)
	if err != nil {
		t.Fatalf("claim runnable session after resume: %v", err)
	}
	if !ok || claimed.SessionID != attached.SessionID {
		t.Fatalf("expected resumed session to become runnable again, got ok=%v claimed=%#v", ok, claimed)
	}
}

func TestAbortBlockedRuntimeClearsGenericBlockedState(t *testing.T) {
	rt := hruntime.New(hruntime.Options{})

	sess := mustCreateSession(t, rt, "abort generic blocked runtime", "abort blocked runtime")
	record, _, err := rt.CreateBlockedRuntime(context.Background(), sess.SessionID, hruntime.BlockedRuntimeRequest{
		Kind: execution.BlockedRuntimeInteractive,
		Subject: execution.BlockedRuntimeSubject{
			StepID:  "step_interactive_wait",
			CycleID: "cyc_interactive_wait",
		},
		Condition: execution.BlockedRuntimeCondition{
			Kind:       execution.BlockedRuntimeConditionInteractive,
			WaitingFor: "interactive_attach",
		},
	})
	if err != nil {
		t.Fatalf("create blocked runtime: %v", err)
	}

	aborted, updated, err := rt.AbortBlockedRuntime(context.Background(), record.BlockedRuntimeID, hruntime.BlockedRuntimeAbortRequest{
		Reason:   "operator_abort",
		Metadata: map[string]any{"actor": "operator"},
	})
	if err != nil {
		t.Fatalf("abort blocked runtime: %v", err)
	}
	if aborted.Status != execution.BlockedRuntimeAborted {
		t.Fatalf("expected aborted status, got %#v", aborted)
	}
	if updated.ExecutionState != session.ExecutionIdle {
		t.Fatalf("expected session to return to idle after abort, got %#v", updated)
	}

	stored, err := rt.GetBlockedRuntimeRecord(record.BlockedRuntimeID)
	if err != nil {
		t.Fatalf("get blocked runtime record: %v", err)
	}
	if stored.Status != execution.BlockedRuntimeAborted {
		t.Fatalf("expected persisted aborted record, got %#v", stored)
	}
}

func TestRunStepRejectsGenericBlockedSession(t *testing.T) {
	rt := hruntime.New(hruntime.Options{})

	sess := mustCreateSession(t, rt, "blocked session run step", "run step should reject blocked session")
	if _, _, err := rt.CreateBlockedRuntime(context.Background(), sess.SessionID, hruntime.BlockedRuntimeRequest{
		Kind: execution.BlockedRuntimeExternal,
		Condition: execution.BlockedRuntimeCondition{
			Kind:       execution.BlockedRuntimeConditionExternal,
			WaitingFor: "external_signal",
		},
	}); err != nil {
		t.Fatalf("create blocked runtime: %v", err)
	}

	_, err := rt.RunStep(context.Background(), sess.SessionID, planStepSpecForBlockedRuntime()[0])
	if !errors.Is(err, hruntime.ErrSessionBlocked) {
		t.Fatalf("expected ErrSessionBlocked, got %v", err)
	}
}

func TestAbortSessionAbortsGenericBlockedRuntime(t *testing.T) {
	rt := hruntime.New(hruntime.Options{})

	sess := mustCreateSession(t, rt, "abort session with blocked runtime", "aborting session should abort blocked runtime")
	record, _, err := rt.CreateBlockedRuntime(context.Background(), sess.SessionID, hruntime.BlockedRuntimeRequest{
		Kind: execution.BlockedRuntimeInteractive,
		Condition: execution.BlockedRuntimeCondition{
			Kind:       execution.BlockedRuntimeConditionInteractive,
			WaitingFor: "interactive_attach",
		},
	})
	if err != nil {
		t.Fatalf("create blocked runtime: %v", err)
	}

	out, err := rt.AbortSession(context.Background(), sess.SessionID, hruntime.AbortRequest{Reason: "user aborted"})
	if err != nil {
		t.Fatalf("abort session: %v", err)
	}
	if out.Session.Phase != session.PhaseAborted {
		t.Fatalf("expected aborted session phase, got %#v", out.Session)
	}

	stored, err := rt.GetBlockedRuntimeRecord(record.BlockedRuntimeID)
	if err != nil {
		t.Fatalf("get blocked runtime record: %v", err)
	}
	if stored.Status != execution.BlockedRuntimeAborted {
		t.Fatalf("expected blocked runtime to be aborted with session, got %#v", stored)
	}

	_, err = rt.GetBlockedRuntime(sess.SessionID)
	if !errors.Is(err, execution.ErrBlockedRuntimeNotFound) {
		t.Fatalf("expected no current blocked runtime after session abort, got %v", err)
	}
}
