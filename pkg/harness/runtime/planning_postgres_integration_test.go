package runtime_test

import (
	"context"
	"testing"

	"github.com/yiiilin/harness-core/internal/postgrestest"
	"github.com/yiiilin/harness-core/pkg/harness/builtins"
	hplanning "github.com/yiiilin/harness-core/pkg/harness/planning"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

func TestPlanningRecordsPersistAcrossPostgresRuntimeReinitAndReplan(t *testing.T) {
	pg := postgrestest.Start(t)

	opts := hruntime.Options{Compactor: planningSummaryCompactor{}}
	builtins.Register(&opts)

	rt1, db1 := pg.OpenService(t, opts)
	rt1 = rt1.WithPlanner(sequencePlanner{})

	sess := mustCreateSession(t, rt1, "postgres planning", "persist planning records across runtime restart")
	tsk := mustCreateTask(t, rt1, task.Spec{TaskType: "demo", Goal: "planning records survive restart"})
	sess, err := rt1.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	first, _, err := rt1.CreatePlanFromPlanner(context.Background(), sess.SessionID, "initial postgres planning", 1)
	if err != nil {
		t.Fatalf("create first plan from planner: %v", err)
	}

	beforeRestart := mustListPlanningRecords(t, rt1, sess.SessionID)
	if len(beforeRestart) != 1 || beforeRestart[0].Status != hplanning.StatusCompleted {
		t.Fatalf("expected first runtime to persist one completed planning record, got %#v", beforeRestart)
	}

	if err := db1.Close(); err != nil {
		t.Fatalf("close first db: %v", err)
	}

	rt2, db2 := pg.OpenService(t, opts)
	rt2 = rt2.WithPlanner(sequencePlanner{})
	defer db2.Close()

	persisted := mustListPlanningRecords(t, rt2, sess.SessionID)
	if len(persisted) != 1 {
		t.Fatalf("expected planning record after runtime reinit, got %#v", persisted)
	}
	if persisted[0].PlanID != first.PlanID || persisted[0].PlanRevision != first.Revision {
		t.Fatalf("expected persisted planning record to keep plan correlation, got %#v", persisted[0])
	}

	second, _, err := rt2.CreatePlanFromPlanner(context.Background(), sess.SessionID, "postgres replan", 1)
	if err != nil {
		t.Fatalf("create second plan from planner after restart: %v", err)
	}

	records := mustListPlanningRecords(t, rt2, sess.SessionID)
	if len(records) != 2 {
		t.Fatalf("expected two durable planning records after restart and replan, got %#v", records)
	}
	if records[0].PlanID != first.PlanID || records[1].PlanID != second.PlanID {
		t.Fatalf("expected durable planning records for both revisions, got %#v", records)
	}
}

func TestContextSummariesPersistTriggerAndSupersedesAcrossPostgresRuntimeReinit(t *testing.T) {
	pg := postgrestest.Start(t)

	opts := hruntime.Options{Compactor: planningSummaryCompactor{}}
	builtins.Register(&opts)

	rt1, db1 := pg.OpenService(t, opts)

	sess := mustCreateSession(t, rt1, "postgres summaries", "persist context summary metadata across restart")
	tsk := mustCreateTask(t, rt1, task.Spec{TaskType: "demo", Goal: "summary metadata survives restart"})
	sess, err := rt1.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	firstPkg, firstSummary, err := rt1.CompactSessionContext(context.Background(), sess.SessionID, hruntime.CompactionTriggerPlan)
	if err != nil {
		t.Fatalf("compact plan summary: %v", err)
	}
	if firstSummary == nil || firstPkg.Compaction == nil {
		t.Fatalf("expected first persisted summary, got summary=%#v pkg=%#v", firstSummary, firstPkg)
	}

	secondPkg, secondSummary, err := rt1.CompactSessionContext(context.Background(), sess.SessionID, hruntime.CompactionTriggerExecute)
	if err != nil {
		t.Fatalf("compact execute summary: %v", err)
	}
	if secondSummary == nil || secondPkg.Compaction == nil {
		t.Fatalf("expected second persisted summary, got summary=%#v pkg=%#v", secondSummary, secondPkg)
	}
	if secondSummary.SupersedesSummaryID != firstSummary.SummaryID {
		t.Fatalf("expected in-process supersedes chain before restart, got %#v", secondSummary)
	}

	if err := db1.Close(); err != nil {
		t.Fatalf("close first db: %v", err)
	}

	rt2, db2 := pg.OpenService(t, opts)
	defer db2.Close()

	summaries, err := rt2.ListContextSummaries(sess.SessionID)
	if err != nil {
		t.Fatalf("list summaries after reinit: %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("expected two durable summaries, got %#v", summaries)
	}
	if summaries[0].SummaryID != firstSummary.SummaryID || summaries[0].Trigger != hruntime.CompactionTriggerPlan {
		t.Fatalf("expected first durable summary to keep plan trigger, got %#v", summaries[0])
	}
	if summaries[1].SummaryID != secondSummary.SummaryID || summaries[1].Trigger != hruntime.CompactionTriggerExecute {
		t.Fatalf("expected second durable summary to keep execute trigger, got %#v", summaries[1])
	}
	if summaries[1].SupersedesSummaryID != firstSummary.SummaryID {
		t.Fatalf("expected durable supersedes chain %q, got %#v", firstSummary.SummaryID, summaries[1])
	}
}
