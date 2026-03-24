package release_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	var _ = (*harness.Service).CreatePlanFromProgram
	var _ = (*harness.Service).RunSession
	var _ = (*harness.Service).RunProgram
	var _ = (*harness.Service).RunClaimedSession
	var _ = (*harness.Service).RecoverClaimedSession
	var _ = (*harness.Service).RespondApproval
	var _ = (*harness.Service).ResumePendingApproval
	var _ = (*harness.Service).ClaimRunnableSession
	var _ = (*harness.Service).ClaimRecoverableSession
	var _ = (*harness.Service).ReleaseSessionLease
	var _ = (*harness.Service).MatchCapability
	var _ = (*harness.Service).GetBlockedRuntime
	var _ = (*harness.Service).GetBlockedRuntimeByApproval
	var _ = (*harness.Service).ListBlockedRuntimes
	var _ = (*harness.Service).GetBlockedRuntimeProjection
	var _ = (*harness.Service).GetBlockedRuntimeProjectionByApproval
	var _ = (*harness.Service).ListBlockedRuntimeProjections
	var _ = (*harness.Service).ListAttempts
	var _ = (*harness.Service).ListActions
	var _ = (*harness.Service).ListVerifications
	var _ = (*harness.Service).ListAggregateResults
	var _ = (*harness.Service).GetInteractiveRuntime
	var _ = (*harness.Service).ListInteractiveRuntimes
	var _ = (*harness.Service).StartInteractive
	var _ = (*harness.Service).ReopenInteractive
	var _ = (*harness.Service).ViewInteractive
	var _ = (*harness.Service).WriteInteractive
	var _ = (*harness.Service).CloseInteractive
	var _ = (*harness.Service).UpdateInteractiveRuntime
	var _ = (*harness.Service).UpdateClaimedInteractiveRuntime
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
	var _ harness.CapabilityMatchResult
	var _ harness.CapabilityUnsupportedReason
	var _ harness.CapabilityUnsupportedReasonCode
	var _ harness.CapabilitySupportRequirements
	var _ harness.ExecutionBlockedRuntime
	var _ harness.ExecutionBlockedRuntimeKind
	var _ harness.ExecutionBlockedRuntimeStatus
	var _ harness.ExecutionTarget
	var _ harness.ExecutionTargetRef
	var _ harness.ExecutionTargetSelection
	var _ harness.ExecutionTargetSelectionMode
	var _ harness.ExecutionTargetFailureStrategy
	var _ harness.ExecutionAggregateScope
	var _ harness.ExecutionAggregateStatus
	var _ harness.ExecutionAggregateTargetResult
	var _ harness.ExecutionAggregateResult
	var _ harness.ExecutionInteractiveCapabilities
	var _ harness.ExecutionInteractiveSnapshot
	var _ harness.ExecutionInteractiveObservation
	var _ harness.ExecutionInteractiveOperation
	var _ harness.ExecutionInteractiveOperationKind
	var _ harness.ExecutionInteractiveRuntime
	var _ = harness.ExecutionTargetArgKey
	var _ = harness.ExecutionTargetMetadataKeyID
	var _ = harness.ExecutionTargetMetadataKeyKind
	var _ = harness.ExecutionAggregateMetadataKeyID
	var _ = harness.ExecutionAggregateMetadataKeyScope
	var _ = harness.ExecutionAggregateMetadataKeyStrategy
	var _ = harness.ExecutionAggregateMetadataKeyMaxConcurrency
	var _ = harness.ExecutionInteractiveMetadataKeyEnabled
	var _ = harness.ExecutionInteractiveMetadataKeySupportsReopen
	var _ = harness.ExecutionInteractiveMetadataKeySupportsView
	var _ = harness.ExecutionInteractiveMetadataKeySupportsWrite
	var _ = harness.ExecutionInteractiveMetadataKeySupportsClose
	var _ = harness.ExecutionInteractiveMetadataKeyNextOffset
	var _ = harness.ExecutionInteractiveMetadataKeyClosed
	var _ = harness.ExecutionInteractiveMetadataKeyExitCode
	var _ = harness.ExecutionInteractiveMetadataKeyStatus
	var _ = harness.ExecutionInteractiveMetadataKeyStatusReason
	var _ = harness.ExecutionInteractiveMetadataKeySnapshotArtifactID
	var _ = harness.ExecutionInteractiveMetadataKeyLastOperationKind
	var _ = harness.ExecutionInteractiveMetadataKeyLastOperationAt
	var _ = harness.ExecutionInteractiveMetadataKeyLastOperationOffset
	var _ = harness.ExecutionInteractiveMetadataKeyLastOperationBytes
	var _ = harness.ExecutionInteractiveOperationReopen
	var _ = harness.ExecutionInteractiveOperationView
	var _ = harness.ExecutionInteractiveOperationWrite
	var _ = harness.ExecutionInteractiveOperationClose
	var _ harness.ExecutionAttachmentInput
	var _ harness.ExecutionAttachmentInputKind
	var _ harness.ExecutionAttachmentMaterialization
	var _ harness.ExecutionArtifactRef
	var _ harness.ExecutionAttachmentRef
	var _ harness.ExecutionOutputRef
	var _ harness.ExecutionOutputRefKind
	var _ harness.ExecutionProgram
	var _ harness.ExecutionProgramNode
	var _ harness.ExecutionProgramInputBinding
	var _ harness.ExecutionProgramInputBindingKind
	var _ harness.ExecutionVerificationScope
	var _ harness.ExecutionTargetSlice
	var _ harness.ExecutionBlockedRuntimeProjection
	var _ harness.ExecutionBlockedRuntimeWait
	var _ harness.ExecutionBlockedRuntimeWaitScope
	var _ harness.ExecutionBlockedRuntimeRecord
	var _ harness.ExecutionBlockedRuntimeSubject
	var _ harness.ExecutionBlockedRuntimeCondition
	var _ harness.ExecutionBlockedRuntimeConditionKind
	var _ harness.StepRunOutput
	var _ harness.SessionRunOutput
	var _ harness.InteractiveStartRequest
	var _ harness.InteractiveStartResult
	var _ harness.InteractiveReopenRequest
	var _ harness.InteractiveReopenResult
	var _ harness.InteractiveViewRequest
	var _ harness.InteractiveViewResult
	var _ harness.InteractiveWriteRequest
	var _ harness.InteractiveWriteResult
	var _ harness.InteractiveCloseRequest
	var _ harness.InteractiveCloseResult
	var _ harness.InteractiveRuntimeUpdate
	var _ harness.RuntimeHandleUpdate
	var _ harness.RuntimeHandleCloseRequest
	var _ harness.RuntimeHandleInvalidateRequest
	var _ harness.TargetResolver
	var _ harness.AttachmentMaterializeRequest
	var _ harness.AttachmentMaterializer
	var _ harness.InteractiveController
}

func TestCompanionModulesDoNotUseWorkspacePlaceholderVersions(t *testing.T) {
	t.Parallel()

	files := []string{
		"pkg/harness/builtins/go.mod",
		"modules/go.mod",
		"adapters/go.mod",
		"cmd/harness-core/go.mod",
	}

	for _, rel := range files {
		rel := rel
		t.Run(rel, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join("..", filepath.Clean(rel))
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", rel, err)
			}
			if containsWorkspacePlaceholderVersion(string(data)) {
				t.Fatalf("%s still contains workspace placeholder repo-local dependency versions", rel)
			}
			if containsReplaceDirective(string(data)) {
				t.Fatalf("%s still contains repo-local replace directives", rel)
			}
		})
	}
}

func TestRepoGoModsDoNotUseZeroPseudoPlaceholderVersions(t *testing.T) {
	t.Parallel()

	files := []string{
		"go.mod",
		"pkg/harness/builtins/go.mod",
		"modules/go.mod",
		"adapters/go.mod",
		"cmd/harness-core/go.mod",
	}

	for _, rel := range files {
		rel := rel
		t.Run(rel, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join("..", filepath.Clean(rel))
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", rel, err)
			}
			if containsZeroPseudoPlaceholderVersion(string(data)) {
				t.Fatalf("%s still contains zero-time placeholder pseudo-versions", rel)
			}
		})
	}
}

func TestCompanionModulesReferenceResolvableRepoLocalVersions(t *testing.T) {
	t.Parallel()

	tags := gitTagSet(t)
	files := []string{
		"pkg/harness/builtins/go.mod",
		"modules/go.mod",
		"adapters/go.mod",
		"cmd/harness-core/go.mod",
	}
	tagForModule := map[string]func(string) string{
		"github.com/yiiilin/harness-core":                      func(version string) string { return version },
		"github.com/yiiilin/harness-core/pkg/harness/builtins": func(version string) string { return "pkg/harness/builtins/" + version },
		"github.com/yiiilin/harness-core/modules":              func(version string) string { return "modules/" + version },
		"github.com/yiiilin/harness-core/adapters":             func(version string) string { return "adapters/" + version },
		"github.com/yiiilin/harness-core/cmd/harness-core":     func(version string) string { return "cmd/harness-core/" + version },
	}

	for _, rel := range files {
		rel := rel
		t.Run(rel, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join("..", filepath.Clean(rel))
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", rel, err)
			}
			for _, line := range strings.Split(string(data), "\n") {
				fields := strings.Fields(strings.TrimSpace(line))
				if len(fields) < 2 {
					continue
				}
				tagResolver, ok := tagForModule[fields[0]]
				if !ok {
					continue
				}
				version := fields[1]
				switch {
				case isPseudoVersion(version):
					if !gitCommitExists(t, pseudoVersionCommit(version)) {
						t.Fatalf("%s references %s %s but local commit %q is missing", rel, fields[0], version, pseudoVersionCommit(version))
					}
				default:
					expected := tagResolver(version)
					if !tags[expected] {
						t.Fatalf("%s references %s %s but local tag %q is missing", rel, fields[0], version, expected)
					}
				}
			}
		})
	}
}

func TestCompanionModulePseudoVersionsStayZeroBaseUntilTagged(t *testing.T) {
	t.Parallel()

	files := []string{
		"pkg/harness/builtins/go.mod",
		"modules/go.mod",
		"adapters/go.mod",
		"cmd/harness-core/go.mod",
	}
	companionModules := map[string]struct{}{
		"github.com/yiiilin/harness-core/pkg/harness/builtins": {},
		"github.com/yiiilin/harness-core/modules":              {},
		"github.com/yiiilin/harness-core/adapters":             {},
		"github.com/yiiilin/harness-core/cmd/harness-core":     {},
	}

	for _, rel := range files {
		rel := rel
		t.Run(rel, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join("..", filepath.Clean(rel))
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", rel, err)
			}
			for _, line := range strings.Split(string(data), "\n") {
				fields := strings.Fields(strings.TrimSpace(line))
				if len(fields) < 2 {
					continue
				}
				if _, ok := companionModules[fields[0]]; !ok {
					continue
				}
				version := fields[1]
				if !isPseudoVersion(version) {
					continue
				}
				if !strings.HasPrefix(version, "v0.0.0-") {
					t.Fatalf("%s references companion module %s with pseudo-version %s; use zero-base v0.0.0-... until a matching companion tag is published", rel, fields[0], version)
				}
			}
		})
	}
}

var pseudoVersionPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.]+)?\.[0-9]{14}-([0-9a-f]{12})$|^v[0-9]+\.[0-9]+\.[0-9]+-[0-9]{14}-([0-9a-f]{12})$`)

func isPseudoVersion(version string) bool {
	return pseudoVersionPattern.MatchString(version)
}

func pseudoVersionCommit(version string) string {
	matches := pseudoVersionPattern.FindStringSubmatch(version)
	if len(matches) < 3 {
		return ""
	}
	for _, match := range matches[1:] {
		if match != "" {
			return match
		}
	}
	return ""
}

func gitCommitExists(t *testing.T, rev string) bool {
	t.Helper()
	if rev == "" {
		return false
	}
	cmd := exec.Command("git", "rev-parse", "--verify", rev+"^{commit}")
	cmd.Dir = filepath.Join("..")
	return cmd.Run() == nil
}

var workspacePlaceholderPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?m)^\s*github\.com/yiiilin/harness-core\s+v0\.0\.0\s*$`),
	regexp.MustCompile(`(?m)^\s*github\.com/yiiilin/harness-core/modules\s+v0\.0\.0\s*$`),
	regexp.MustCompile(`(?m)^\s*github\.com/yiiilin/harness-core/pkg/harness/builtins\s+v0\.0\.0\s*$`),
	regexp.MustCompile(`(?m)^\s*github\.com/yiiilin/harness-core/adapters\s+v0\.0\.0\s*$`),
}

var zeroPseudoPlaceholderPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?m)^\s*github\.com/yiiilin/harness-core(?:/modules|/pkg/harness/builtins|/adapters|/cmd/harness-core)?\s+v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.]+)?\.00010101000000-000000000000(?:\s|$)`),
	regexp.MustCompile(`(?m)^\s*github\.com/yiiilin/harness-core(?:/modules|/pkg/harness/builtins|/adapters|/cmd/harness-core)?\s+v[0-9]+\.[0-9]+\.[0-9]+-00010101000000-000000000000(?:\s|$)`),
}

func containsWorkspacePlaceholderVersion(goMod string) bool {
	for _, pattern := range workspacePlaceholderPatterns {
		if pattern.MatchString(goMod) {
			return true
		}
	}
	return false
}

func containsZeroPseudoPlaceholderVersion(goMod string) bool {
	for _, pattern := range zeroPseudoPlaceholderPatterns {
		if pattern.MatchString(goMod) {
			return true
		}
	}
	return false
}

func containsReplaceDirective(goMod string) bool {
	for _, line := range strings.Split(goMod, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "replace ") {
			return true
		}
	}
	return false
}

func gitTagSet(t *testing.T) map[string]bool {
	t.Helper()

	cmd := exec.Command("git", "tag", "--list")
	cmd.Dir = filepath.Join("..")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("list git tags: %v", err)
	}
	out := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		tag := strings.TrimSpace(line)
		if tag != "" {
			out[tag] = true
		}
	}
	return out
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
