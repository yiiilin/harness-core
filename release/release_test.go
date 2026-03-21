package release_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	internalpostgres "github.com/yiiilin/harness-core/internal/postgres"
	"github.com/yiiilin/harness-core/internal/postgrestest"
	"github.com/yiiilin/harness-core/pkg/harness"
	"github.com/yiiilin/harness-core/pkg/harness/builtins"
	hpostgres "github.com/yiiilin/harness-core/pkg/harness/postgres"
	"github.com/yiiilin/harness-core/pkg/harness/replay"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/worker"
)

func TestTier1StablePackagesExposeExpectedEntryPoints(t *testing.T) {
	var _ = harness.New
	var _ = harness.NewDefault
	var _ = harness.NewWorkerHelper
	var _ = harness.NewReplayReader
	var _ = (*harness.Service).CreatePlanFromPlanner
	var _ = (*harness.Service).RunSession
	var _ = (*harness.Service).RunClaimedSession
	var _ = (*harness.Service).RecoverClaimedSession
	var _ = (*harness.Service).RespondApproval
	var _ = (*harness.Service).ResumePendingApproval
	var _ = (*harness.Service).ClaimRunnableSession
	var _ = (*harness.Service).ClaimRecoverableSession
	var _ = (*harness.Service).ReleaseSessionLease
	var _ = (*harness.Service).ListAttempts
	var _ = (*harness.Service).ListActions
	var _ = (*harness.Service).ListVerifications
	var _ = (*harness.Service).ListExecutionCycles
	var _ = worker.New
	var _ = replay.NewReader
	var _ = hpostgres.OpenDB
	var _ = hpostgres.ApplyMigrations
	var _ = hpostgres.OpenService
	var _ = builtins.Register

	var _ harness.WorkerOptions
	var _ harness.WorkerLoopOptions
	var _ harness.WorkerLoopIteration
	var _ harness.WorkerResult
	var _ harness.ReplaySessionProjection
	var _ harness.StepRunOutput
	var _ harness.SessionRunOutput
	var _ harness.RuntimeHandleUpdate
	var _ harness.RuntimeHandleCloseRequest
	var _ harness.RuntimeHandleInvalidateRequest
}

func TestTier1InMemoryWorkerApprovalReplayFlow(t *testing.T) {
	ctx := context.Background()
	opts := harness.Options{}
	builtins.Register(&opts)
	rt := harness.New(opts).WithPolicyEvaluator(releaseAskAllPolicy{})

	sessionID := seedShellSession(t, rt, "tier1-memory", "echo tier1 memory release")

	workerHelper, err := worker.New(worker.Options{
		Runtime:       rt,
		LeaseTTL:      time.Minute,
		RenewInterval: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("new worker helper: %v", err)
	}

	first, err := workerHelper.RunOnce(ctx)
	if err != nil {
		t.Fatalf("first run once: %v", err)
	}
	if !first.ApprovalPending || first.Run.Session.PendingApprovalID == "" {
		t.Fatalf("expected approval-pending first run, got %#v", first)
	}

	actionsBeforeApproval, err := rt.ListActions(sessionID)
	if err != nil {
		t.Fatalf("list actions before approval: %v", err)
	}
	if len(actionsBeforeApproval) != 0 {
		t.Fatalf("expected no actions before approval, got %#v", actionsBeforeApproval)
	}

	approvals, err := rt.ListApprovals(sessionID)
	if err != nil {
		t.Fatalf("list approvals: %v", err)
	}
	if len(approvals) != 1 {
		t.Fatalf("expected one approval, got %#v", approvals)
	}

	if _, _, err := rt.RespondApproval(approvals[0].ApprovalID, harness.ApprovalResponse{Reply: harness.ApprovalReply("once")}); err != nil {
		t.Fatalf("respond approval: %v", err)
	}

	second, err := workerHelper.RunOnce(ctx)
	if err != nil {
		t.Fatalf("second run once: %v", err)
	}
	if second.Run.Session.Phase != harness.SessionPhase("complete") || second.ApprovalPending {
		t.Fatalf("expected completed second run, got %#v", second)
	}

	actions, err := rt.ListActions(sessionID)
	if err != nil {
		t.Fatalf("list actions: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("expected one persisted action, got %#v", actions)
	}
	stdout, _ := actions[0].Result.Data["stdout"].(string)
	if !strings.Contains(stdout, "tier1 memory release") {
		t.Fatalf("expected action stdout in persisted result, got %#v", actions[0].Result)
	}

	projection, err := replay.NewReader(rt).SessionProjection(sessionID)
	if err != nil {
		t.Fatalf("session projection: %v", err)
	}
	if len(projection.Cycles) != 1 || len(projection.Events) == 0 {
		t.Fatalf("expected replay projection facts, got %#v", projection)
	}
}

func TestTier1PostgresRestartApprovalResumeFlow(t *testing.T) {
	ctx := context.Background()
	pg := postgrestest.Start(t)

	opts := harness.Options{}
	builtins.Register(&opts)
	opts.Policy = releaseAskAllPolicy{}

	rt1, db1, err := hpostgres.OpenService(ctx, pg.DSN, opts)
	if err != nil {
		t.Fatalf("open first service: %v", err)
	}

	sessionID := seedShellSession(t, rt1, "tier1-postgres", "echo tier1 postgres release")

	worker1, err := worker.New(worker.Options{
		Runtime:       rt1,
		LeaseTTL:      time.Minute,
		RenewInterval: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("new first worker: %v", err)
	}

	first, err := worker1.RunOnce(ctx)
	if err != nil {
		t.Fatalf("first run once: %v", err)
	}
	if !first.ApprovalPending {
		t.Fatalf("expected approval pause before restart, got %#v", first)
	}

	approvalsBeforeRestart, err := rt1.ListApprovals(sessionID)
	if err != nil {
		t.Fatalf("list approvals before restart: %v", err)
	}
	if len(approvalsBeforeRestart) != 1 {
		t.Fatalf("expected one persisted approval before restart, got %#v", approvalsBeforeRestart)
	}
	if err := db1.Close(); err != nil {
		t.Fatalf("close first db: %v", err)
	}

	rt2, db2, err := hpostgres.OpenService(ctx, pg.DSN, opts)
	if err != nil {
		t.Fatalf("open second service: %v", err)
	}
	defer db2.Close()

	approvalsAfterRestart, err := rt2.ListApprovals(sessionID)
	if err != nil {
		t.Fatalf("list approvals after restart: %v", err)
	}
	if len(approvalsAfterRestart) != 1 || approvalsAfterRestart[0].Status != harness.ApprovalStatus("pending") {
		t.Fatalf("expected one pending approval after restart, got %#v", approvalsAfterRestart)
	}

	if _, _, err := rt2.RespondApproval(approvalsAfterRestart[0].ApprovalID, harness.ApprovalResponse{Reply: harness.ApprovalReply("once")}); err != nil {
		t.Fatalf("respond approval after restart: %v", err)
	}

	worker2, err := worker.New(worker.Options{
		Runtime:       rt2,
		LeaseTTL:      time.Minute,
		RenewInterval: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("new second worker: %v", err)
	}

	second, err := worker2.RunOnce(ctx)
	if err != nil {
		t.Fatalf("second run once: %v", err)
	}
	if second.Run.Session.Phase != session.PhaseComplete || second.ApprovalPending {
		t.Fatalf("expected completion after restart and approval, got %#v", second)
	}

	projection, err := replay.NewReader(rt2).SessionProjection(sessionID)
	if err != nil {
		t.Fatalf("projection after restart: %v", err)
	}
	if len(projection.Cycles) != 1 || len(projection.Events) == 0 {
		t.Fatalf("expected durable replay facts after restart, got %#v", projection)
	}
}

func TestPostgresUpgradeFromPreviousSchemaVersionRemainsBootable(t *testing.T) {
	ctx := context.Background()
	pg := postgrestest.Start(t)

	db, err := hpostgres.OpenDB(ctx, pg.DSN)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	previousVersion, err := applyMigrationsExceptLatest(ctx, db)
	if err != nil {
		t.Fatalf("apply migrations except latest: %v", err)
	}
	if previousVersion == "" {
		t.Fatal("expected at least one previous schema version")
	}

	versionBefore, err := hpostgres.SchemaVersion(ctx, db)
	if err != nil {
		t.Fatalf("schema version before upgrade: %v", err)
	}
	if versionBefore != previousVersion {
		t.Fatalf("expected previous schema version %q before upgrade, got %q", previousVersion, versionBefore)
	}

	if err := hpostgres.ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("upgrade apply migrations: %v", err)
	}
	versionAfter, err := hpostgres.SchemaVersion(ctx, db)
	if err != nil {
		t.Fatalf("schema version after upgrade: %v", err)
	}
	if versionAfter != hpostgres.LatestSchemaVersion() {
		t.Fatalf("expected latest schema version %q after upgrade, got %q", hpostgres.LatestSchemaVersion(), versionAfter)
	}

	opts := harness.Options{}
	builtins.Register(&opts)
	rt, db2, err := hpostgres.OpenService(ctx, pg.DSN, opts)
	if err != nil {
		t.Fatalf("open service after upgrade: %v", err)
	}
	defer db2.Close()

	if _, err := rt.CreateSession("upgraded", "service boots after schema upgrade"); err != nil {
		t.Fatalf("create session after upgrade: %v", err)
	}
	sessions, err := rt.ListSessions()
	if err != nil {
		t.Fatalf("list sessions after upgrade: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected bootable upgraded runtime with one session, got %#v", sessions)
	}
}

type releaseAskAllPolicy struct{}

func (releaseAskAllPolicy) Evaluate(_ context.Context, _ harness.SessionState, step harness.StepSpec) (harness.PermissionDecision, error) {
	return harness.PermissionDecision{
		Action:      harness.Ask,
		Reason:      "release gate approval path",
		MatchedRule: fmt.Sprintf("release/%s", step.Action.ToolName),
	}, nil
}

func seedShellSession(t *testing.T, rt *harness.Service, title, command string) string {
	t.Helper()

	sess, err := rt.CreateSession(title, command)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	tsk, err := rt.CreateTask(harness.TaskSpec{TaskType: "release", Goal: command})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	sess, err = rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}
	_, err = rt.CreatePlan(sess.SessionID, title, []harness.StepSpec{{
		StepID: "step_release",
		Title:  title,
		Action: harness.ActionSpec{
			ToolName: "shell.exec",
			Args: map[string]any{
				"mode":       "pipe",
				"command":    command,
				"timeout_ms": 5000,
			},
		},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	return sess.SessionID
}

func applyMigrationsExceptLatest(ctx context.Context, db *sql.DB) (string, error) {
	migrations := internalpostgres.Migrations()
	if len(migrations) < 2 {
		return "", fmt.Errorf("need at least two migrations to test upgrade path")
	}

	if _, err := db.ExecContext(ctx, `
		DROP TABLE IF EXISTS harness_schema_migrations CASCADE
	`); err != nil {
		return "", err
	}
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE harness_schema_migrations (
			version TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at BIGINT NOT NULL
		)
	`); err != nil {
		return "", err
	}

	lastApplied := ""
	for _, migration := range migrations[:len(migrations)-1] {
		if _, err := db.ExecContext(ctx, migration.SQL); err != nil {
			return "", fmt.Errorf("apply migration %s: %w", migration.Version, err)
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO harness_schema_migrations (version, name, applied_at)
			VALUES ($1, $2, $3)
		`, migration.Version, migration.Name, time.Now().UnixMilli()); err != nil {
			return "", fmt.Errorf("record migration %s: %w", migration.Version, err)
		}
		lastApplied = migration.Version
	}
	return lastApplied, nil
}
