package executionrepo_test

import (
	"context"
	"testing"

	"github.com/yiiilin/harness-core/internal/postgres/executionrepo"
	"github.com/yiiilin/harness-core/internal/postgrestest"
	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	hpostgres "github.com/yiiilin/harness-core/pkg/harness/postgres"
)

func TestRuntimeHandleRepoPersistsLifecycleStateAgainstPostgres(t *testing.T) {
	pg := postgrestest.Start(t)
	db, err := hpostgres.OpenDB(context.Background(), pg.DSN)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	repo := executionrepo.NewRuntimeHandleStore(db)
	created, err := repo.Create(execution.RuntimeHandle{
		HandleID:     "hdl_pg_lifecycle",
		SessionID:    "sess_pg_runtime",
		TaskID:       "task_pg_runtime",
		AttemptID:    "att_pg_runtime",
		TraceID:      "trace_pg_runtime",
		Kind:         "pty",
		Value:        "pty-pg-runtime",
		Status:       execution.RuntimeHandleActive,
		StatusReason: "tool reported active",
		Metadata:     map[string]any{"origin": "integration-test"},
		CreatedAt:    10,
		UpdatedAt:    10,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	created.Status = execution.RuntimeHandleClosed
	created.StatusReason = "client closed"
	created.ClosedAt = 20
	created.UpdatedAt = 20
	created.Metadata["closed_by"] = "operator"
	if err := repo.Update(created); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := repo.Get(created.HandleID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != execution.RuntimeHandleClosed || got.ClosedAt != 20 || got.StatusReason != "client closed" {
		t.Fatalf("unexpected persisted runtime handle: %#v", got)
	}
	if got.Metadata["origin"] != "integration-test" || got.Metadata["closed_by"] != "operator" {
		t.Fatalf("expected lifecycle metadata to round-trip, got %#v", got.Metadata)
	}

	items, err := repo.List(created.SessionID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one runtime handle, got %#v", items)
	}
	if items[0].Status != execution.RuntimeHandleClosed || items[0].ClosedAt != 20 {
		t.Fatalf("unexpected listed runtime handle: %#v", items[0])
	}
}

func TestExecutionReposRoundTripCycleIDAgainstPostgres(t *testing.T) {
	pg := postgrestest.Start(t)
	db, err := hpostgres.OpenDB(context.Background(), pg.DSN)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	attempts := executionrepo.NewAttemptStore(db)
	actions := executionrepo.NewActionStore(db)
	verifications := executionrepo.NewVerificationStore(db)
	artifacts := executionrepo.NewArtifactStore(db)
	handles := executionrepo.NewRuntimeHandleStore(db)

	const cycleID = "cyc_pg_roundtrip"

	if _, err := attempts.Create(execution.Attempt{
		AttemptID:  "att_pg_cycle",
		SessionID:  "sess_pg_cycle",
		TaskID:     "task_pg_cycle",
		StepID:     "step_pg_cycle",
		CycleID:    cycleID,
		TraceID:    "trc_pg_cycle",
		Status:     execution.AttemptCompleted,
		StartedAt:  10,
		FinishedAt: 20,
	}); err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	if _, err := actions.Create(execution.ActionRecord{
		ActionID:  "act_pg_cycle",
		AttemptID: "att_pg_cycle",
		SessionID: "sess_pg_cycle",
		TaskID:    "task_pg_cycle",
		StepID:    "step_pg_cycle",
		CycleID:   cycleID,
		ToolName:  "shell.exec",
		TraceID:   "trc_pg_cycle",
		Status:    execution.ActionCompleted,
		Result: action.Result{
			OK: true,
			Data: map[string]any{
				"stdout":    "cycle",
				"exit_code": 0,
			},
		},
		StartedAt:  11,
		FinishedAt: 12,
	}); err != nil {
		t.Fatalf("create action: %v", err)
	}
	if _, err := verifications.Create(execution.VerificationRecord{
		VerificationID: "ver_pg_cycle",
		AttemptID:      "att_pg_cycle",
		SessionID:      "sess_pg_cycle",
		TaskID:         "task_pg_cycle",
		StepID:         "step_pg_cycle",
		ActionID:       "act_pg_cycle",
		CycleID:        cycleID,
		TraceID:        "trc_pg_cycle",
		Status:         execution.VerificationCompleted,
		StartedAt:      13,
		FinishedAt:     14,
	}); err != nil {
		t.Fatalf("create verification: %v", err)
	}
	if _, err := artifacts.Create(execution.Artifact{
		ArtifactID: "art_pg_cycle",
		SessionID:  "sess_pg_cycle",
		TaskID:     "task_pg_cycle",
		StepID:     "step_pg_cycle",
		AttemptID:  "att_pg_cycle",
		ActionID:   "act_pg_cycle",
		CycleID:    cycleID,
		TraceID:    "trc_pg_cycle",
		Name:       "action.result",
		Kind:       "action_result",
		Payload:    map[string]any{"stdout": "cycle"},
		CreatedAt:  15,
	}); err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	if _, err := handles.Create(execution.RuntimeHandle{
		HandleID:  "hdl_pg_cycle",
		SessionID: "sess_pg_cycle",
		TaskID:    "task_pg_cycle",
		AttemptID: "att_pg_cycle",
		CycleID:   cycleID,
		TraceID:   "trc_pg_cycle",
		Kind:      "pty",
		Value:     "pty-cycle",
		Status:    execution.RuntimeHandleActive,
		CreatedAt: 16,
		UpdatedAt: 16,
	}); err != nil {
		t.Fatalf("create runtime handle: %v", err)
	}

	gotAttempt, err := attempts.Get("att_pg_cycle")
	if err != nil {
		t.Fatalf("get attempt: %v", err)
	}
	if gotAttempt.CycleID != cycleID {
		t.Fatalf("expected attempt cycle_id %q, got %#v", cycleID, gotAttempt)
	}

	gotAction, err := actions.Get("act_pg_cycle")
	if err != nil {
		t.Fatalf("get action: %v", err)
	}
	if gotAction.CycleID != cycleID {
		t.Fatalf("expected action cycle_id %q, got %#v", cycleID, gotAction)
	}

	gotVerification, err := verifications.Get("ver_pg_cycle")
	if err != nil {
		t.Fatalf("get verification: %v", err)
	}
	if gotVerification.CycleID != cycleID {
		t.Fatalf("expected verification cycle_id %q, got %#v", cycleID, gotVerification)
	}

	gotArtifact, err := artifacts.Get("art_pg_cycle")
	if err != nil {
		t.Fatalf("get artifact: %v", err)
	}
	if gotArtifact.CycleID != cycleID {
		t.Fatalf("expected artifact cycle_id %q, got %#v", cycleID, gotArtifact)
	}

	gotHandle, err := handles.Get("hdl_pg_cycle")
	if err != nil {
		t.Fatalf("get runtime handle: %v", err)
	}
	if gotHandle.CycleID != cycleID {
		t.Fatalf("expected runtime handle cycle_id %q, got %#v", cycleID, gotHandle)
	}
}
