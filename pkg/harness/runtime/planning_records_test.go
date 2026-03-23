package runtime_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/capability"
	"github.com/yiiilin/harness-core/pkg/harness/builtins"
	hplanning "github.com/yiiilin/harness-core/pkg/harness/planning"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

type planningSummaryCompactor struct{}

type nthFailingPlanningStore struct {
	hplanning.Store
	createErr        error
	failOnCreateCall int
	createCalls      int
}

func (s *nthFailingPlanningStore) Create(spec hplanning.Record) (hplanning.Record, error) {
	s.createCalls++
	if s.failOnCreateCall > 0 && s.createCalls == s.failOnCreateCall {
		return hplanning.Record{}, s.createErr
	}
	return s.Store.Create(spec)
}

type nthFailingSnapshotStore struct {
	capability.SnapshotStore
	createErr        error
	failOnCreateCall int
	createCalls      int
}

func (s *nthFailingSnapshotStore) Create(spec capability.Snapshot) (capability.Snapshot, error) {
	s.createCalls++
	if s.failOnCreateCall > 0 && s.createCalls == s.failOnCreateCall {
		return capability.Snapshot{}, s.createErr
	}
	return s.SnapshotStore.Create(spec)
}

func (planningSummaryCompactor) Compact(_ context.Context, pkg hruntime.ContextPackage, state session.State, spec task.Spec, _ hruntime.LoopBudgets) (hruntime.ContextPackage, *hruntime.ContextSummary, error) {
	return pkg, &hruntime.ContextSummary{
		Strategy:       "summary/test",
		Summary:        map[string]any{"goal": spec.Goal, "current_step_id": state.CurrentStepID},
		Metadata:       map[string]any{"origin": "planning-records-test"},
		OriginalBytes:  256,
		CompactedBytes: 64,
	}, nil
}

func TestCreatePlanFromPlannerPersistsCompletedPlanningRecord(t *testing.T) {
	opts := hruntime.Options{Compactor: planningSummaryCompactor{}}
	builtins.Register(&opts)
	rt := hruntime.New(opts).WithPlanner(sequencePlanner{})

	sess := mustCreateSession(t, rt, "planning records", "persist successful planning")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "record successful planning"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, _, err := rt.CreatePlanFromPlanner(context.Background(), sess.SessionID, "planner derived", 1)
	if err != nil {
		t.Fatalf("create plan from planner: %v", err)
	}

	records := mustListPlanningRecords(t, rt, sess.SessionID)
	if len(records) != 1 {
		t.Fatalf("expected one planning record, got %#v", records)
	}
	rec := records[0]
	if rec.Status != hplanning.StatusCompleted {
		t.Fatalf("expected completed planning record, got %#v", rec)
	}
	if rec.Reason != "planner derived" {
		t.Fatalf("expected planning reason to be persisted, got %#v", rec)
	}
	if rec.TaskID != tsk.TaskID || rec.PlanID != pl.PlanID || rec.PlanRevision != pl.Revision {
		t.Fatalf("expected plan/task correlation links, got %#v", rec)
	}
	if rec.CapabilityViewID == "" {
		t.Fatalf("expected capability view link, got %#v", rec)
	}
	if rec.ContextSummaryID == "" {
		t.Fatalf("expected context summary link, got %#v", rec)
	}
	if rec.StartedAt == 0 || rec.FinishedAt == 0 || rec.FinishedAt < rec.StartedAt {
		t.Fatalf("expected planning timestamps, got %#v", rec)
	}

	got, err := rt.GetPlanningRecord(rec.PlanningID)
	if err != nil {
		t.Fatalf("get planning record: %v", err)
	}
	if got.PlanningID != rec.PlanningID {
		t.Fatalf("expected get to return persisted planning record, got %#v", got)
	}

	snapshots := mustListCapabilitySnapshots(t, rt, sess.SessionID)
	foundView := false
	for _, snapshot := range snapshots {
		if snapshot.ViewID == rec.CapabilityViewID && snapshot.PlanID == pl.PlanID {
			foundView = true
			break
		}
	}
	if !foundView {
		t.Fatalf("expected capability snapshots linked to planning record view %q, got %#v", rec.CapabilityViewID, snapshots)
	}

	summaries, err := rt.ListContextSummaries(sess.SessionID)
	if err != nil {
		t.Fatalf("list context summaries: %v", err)
	}
	foundSummary := false
	for _, summary := range summaries {
		if summary.SummaryID == rec.ContextSummaryID {
			foundSummary = true
			break
		}
	}
	if !foundSummary {
		t.Fatalf("expected context summary %q, got %#v", rec.ContextSummaryID, summaries)
	}

	events := mustListAuditEvents(t, rt, sess.SessionID)
	foundPlanEvent := false
	for _, event := range events {
		if event.Type == "plan.generated" && event.PlanningID == rec.PlanningID {
			foundPlanEvent = true
			break
		}
	}
	if !foundPlanEvent {
		t.Fatalf("expected plan.generated event correlated to planning record %q, got %#v", rec.PlanningID, events)
	}
}

func TestCreatePlanFromPlannerPersistsFailedPlanningRecord(t *testing.T) {
	opts := hruntime.Options{Compactor: planningSummaryCompactor{}}
	builtins.Register(&opts)
	rt := hruntime.New(opts).WithPlanner(failingPlanner{})

	sess := mustCreateSession(t, rt, "planning failure records", "persist failed planning")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "record failed planning"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	if _, _, err := rt.CreatePlanFromPlanner(context.Background(), sess.SessionID, "planner failure", 2); err == nil {
		t.Fatalf("expected planner failure")
	}

	records := mustListPlanningRecords(t, rt, sess.SessionID)
	if len(records) != 1 {
		t.Fatalf("expected one failed planning record, got %#v", records)
	}
	rec := records[0]
	if rec.Status != hplanning.StatusFailed {
		t.Fatalf("expected failed planning record, got %#v", rec)
	}
	if rec.Reason != "planner failure" {
		t.Fatalf("expected persisted planning reason, got %#v", rec)
	}
	if !strings.Contains(rec.Error, "planner failed") {
		t.Fatalf("expected planner failure message, got %#v", rec)
	}
	if rec.PlanID != "" || rec.PlanRevision != 0 {
		t.Fatalf("expected failed planning record to have no persisted plan, got %#v", rec)
	}
	if rec.CapabilityViewID == "" || rec.ContextSummaryID == "" {
		t.Fatalf("expected failed planning record to keep context/capability links, got %#v", rec)
	}

	plans := mustListPlans(t, rt, sess.SessionID)
	if len(plans) != 0 {
		t.Fatalf("expected planner failure to leave no plan revision, got %#v", plans)
	}
}

func TestCreatePlanFromPlannerPersistsDistinctRecordsAcrossReplans(t *testing.T) {
	opts := hruntime.Options{Compactor: planningSummaryCompactor{}}
	builtins.Register(&opts)
	rt := hruntime.New(opts).WithPlanner(sequencePlanner{})

	sess := mustCreateSession(t, rt, "planning replans", "persist replanning records")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "record replans durably"})
	sess, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	first, _, err := rt.CreatePlanFromPlanner(context.Background(), sess.SessionID, "initial planning", 1)
	if err != nil {
		t.Fatalf("create first plan: %v", err)
	}
	second, _, err := rt.CreatePlanFromPlanner(context.Background(), sess.SessionID, "runtime/replan", 1)
	if err != nil {
		t.Fatalf("create second plan: %v", err)
	}

	records := mustListPlanningRecords(t, rt, sess.SessionID)
	if len(records) != 2 {
		t.Fatalf("expected two planning records, got %#v", records)
	}
	if records[0].PlanID != first.PlanID || records[0].PlanRevision != first.Revision || records[0].Reason != "initial planning" {
		t.Fatalf("unexpected initial planning record: %#v", records[0])
	}
	if records[1].PlanID != second.PlanID || records[1].PlanRevision != second.Revision || records[1].Reason != "runtime/replan" {
		t.Fatalf("unexpected replan record: %#v", records[1])
	}
	if records[0].PlanningID == records[1].PlanningID {
		t.Fatalf("expected distinct planning ids across replans, got %#v", records)
	}
	if records[0].CapabilityViewID == "" || records[1].CapabilityViewID == "" || records[0].CapabilityViewID == records[1].CapabilityViewID {
		t.Fatalf("expected replanning to freeze a distinct capability view per cycle, got %#v", records)
	}
}

func TestCreatePlanFromPlannerStaysSuccessfulWhenNoRunnerPlanningRecordPersistenceFails(t *testing.T) {
	boom := errors.New("boom:planning.create")
	opts := hruntime.Options{
		Compactor:       planningSummaryCompactor{},
		PlanningRecords: &nthFailingPlanningStore{Store: hplanning.NewMemoryStore(), createErr: boom, failOnCreateCall: 1},
	}
	builtins.Register(&opts)
	rt := hruntime.New(opts).WithPlanner(sequencePlanner{})
	rt.Runner = nil

	sess := mustCreateSession(t, rt, "planning record failure", "no-runner planning record failures should be best effort")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "planner result should still be returned"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, _, err := rt.CreatePlanFromPlanner(context.Background(), attached.SessionID, "planner derived", 1)
	if err != nil {
		t.Fatalf("expected no-runner planner to stay successful when planning record persistence fails, got %v", err)
	}
	if pl.PlanID == "" || len(pl.Steps) != 1 {
		t.Fatalf("expected persisted plan despite planning record failure, got %#v", pl)
	}

	plans := mustListPlans(t, rt, attached.SessionID)
	if len(plans) != 1 || plans[0].PlanID != pl.PlanID {
		t.Fatalf("expected plan to remain visible despite planning record failure, got %#v", plans)
	}
}

func TestCreatePlanFromPlannerDegradesGracefullyWhenNoRunnerCapabilitySnapshotPersistenceFails(t *testing.T) {
	boom := errors.New("boom:snapshot.create")
	opts := hruntime.Options{
		Compactor:           planningSummaryCompactor{},
		CapabilitySnapshots: &nthFailingSnapshotStore{SnapshotStore: capability.NewMemorySnapshotStore(), createErr: boom, failOnCreateCall: 1},
	}
	builtins.Register(&opts)
	rt := hruntime.New(opts).WithPlanner(sequencePlanner{})
	rt.Runner = nil

	sess := mustCreateSession(t, rt, "snapshot failure", "no-runner snapshot failures should not strand a broken plan")
	tsk := mustCreateTask(t, rt, task.Spec{TaskType: "demo", Goal: "planner result should still execute after snapshot failure"})
	attached, err := rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	pl, _, err := rt.CreatePlanFromPlanner(context.Background(), attached.SessionID, "planner derived", 1)
	if err != nil {
		t.Fatalf("expected no-runner planner to stay successful when snapshot persistence fails, got %v", err)
	}
	if len(pl.Steps) != 1 {
		t.Fatalf("expected one planned step, got %#v", pl)
	}

	out, err := rt.RunStep(context.Background(), attached.SessionID, pl.Steps[0])
	if err != nil {
		t.Fatalf("expected degraded plan to remain executable after snapshot persistence failure, got %v", err)
	}
	if out.Session.Phase != session.PhaseComplete {
		t.Fatalf("expected degraded plan to complete successfully, got %#v", out.Session)
	}
}
