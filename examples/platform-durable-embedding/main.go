// Command platform-durable-embedding shows a platform-style durable embedding
// flow around the public Postgres bootstrap path.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/builtins"
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hpostgres "github.com/yiiilin/harness-core/pkg/harness/postgres"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type platformApprovalPolicy struct{}

func (platformApprovalPolicy) Evaluate(_ context.Context, _ session.State, _ plan.StepSpec) (permission.Decision, error) {
	return permission.Decision{Action: permission.Ask, Reason: "external approval ui", MatchedRule: "example/platform-approval"}, nil
}

type platformRunIndex struct {
	byRunID map[string]string
}

func newPlatformRunIndex() *platformRunIndex {
	return &platformRunIndex{byRunID: map[string]string{}}
}

func (idx *platformRunIndex) Remember(runID, sessionID string) {
	idx.byRunID[runID] = sessionID
}

func (idx *platformRunIndex) Resolve(runID string) string {
	return idx.byRunID[runID]
}

type DemoResult struct {
	RunID           string
	SessionID       string
	MappedSessionID string
	ApprovalID      string
	FinalPhase      session.Phase
	Output          string
	ActionCount     int
}

func main() {
	dsn := strings.TrimSpace(os.Getenv("HARNESS_POSTGRES_DSN"))
	if dsn == "" {
		panic("HARNESS_POSTGRES_DSN is required")
	}

	cfg := hpostgres.Config{
		DSN:             dsn,
		Schema:          strings.TrimSpace(os.Getenv("HARNESS_POSTGRES_SCHEMA")),
		MaxOpenConns:    4,
		MaxIdleConns:    2,
		ApplyMigrations: true,
	}
	result, err := RunDurableEmbeddingDemo(context.Background(), cfg)
	if err != nil {
		panic(err)
	}

	fmt.Printf("run_id: %s\n", result.RunID)
	fmt.Printf("session_id: %s\n", result.SessionID)
	fmt.Printf("approval_id: %s\n", result.ApprovalID)
	fmt.Printf("final_phase: %s\n", result.FinalPhase)
	fmt.Printf("output: %s\n", strings.TrimSpace(result.Output))
}

// RunDurableEmbeddingDemo demonstrates a realistic platform wrapper:
// - maps external run ids to kernel session ids
// - pauses on approval
// - reopens the durable runtime
// - responds to approval and resumes the same session
func RunDurableEmbeddingDemo(ctx context.Context, cfg hpostgres.Config) (DemoResult, error) {
	opts := durableExampleOptions()
	rt1, db1, err := hpostgres.OpenServiceWithConfig(ctx, cfg, opts)
	if err != nil {
		return DemoResult{}, err
	}

	index := newPlatformRunIndex()
	runID := "run_platform_durable_embedding"

	sess, err := rt1.CreateSession("platform-durable-embedding", "resume the same session after external approval")
	if err != nil {
		_ = db1.Close()
		return DemoResult{}, err
	}
	tsk, err := rt1.CreateTask(task.Spec{
		TaskType: "demo",
		Goal:     "require approval and then continue after restart",
	})
	if err != nil {
		_ = db1.Close()
		return DemoResult{}, err
	}
	sess, err = rt1.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		_ = db1.Close()
		return DemoResult{}, err
	}
	index.Remember(runID, sess.SessionID)

	step := plan.StepSpec{
		StepID: "step_platform_durable_embedding",
		Title:  "approval-gated durable shell command",
		Action: action.Spec{
			ToolName: "shell.exec",
			Args: map[string]any{
				"mode":       "pipe",
				"command":    "echo approved durable embedding",
				"timeout_ms": 5000,
			},
		},
		Verify: verify.Spec{
			Mode: verify.ModeAll,
			Checks: []verify.Check{
				{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
				{Kind: "output_contains", Args: map[string]any{"text": "approved durable embedding"}},
			},
		},
	}
	pl, err := rt1.CreatePlan(sess.SessionID, "platform durable example", []plan.StepSpec{step})
	if err != nil {
		_ = db1.Close()
		return DemoResult{}, err
	}
	initial, err := rt1.RunStep(ctx, sess.SessionID, pl.Steps[0])
	if err != nil {
		_ = db1.Close()
		return DemoResult{}, err
	}
	if initial.Execution.PendingApproval == nil {
		_ = db1.Close()
		return DemoResult{}, fmt.Errorf("expected pending approval from first durable run")
	}
	approvalID := initial.Execution.PendingApproval.ApprovalID
	if err := db1.Close(); err != nil {
		return DemoResult{}, err
	}

	rt2, db2, err := hpostgres.OpenServiceWithConfig(ctx, cfg, durableExampleOptions())
	if err != nil {
		return DemoResult{}, err
	}
	defer db2.Close()

	mappedSessionID := index.Resolve(runID)
	if mappedSessionID == "" {
		return DemoResult{}, fmt.Errorf("run id %q did not resolve to a session", runID)
	}
	if _, _, err := rt2.RespondApproval(approvalID, approval.Response{Reply: approval.ReplyOnce}); err != nil {
		return DemoResult{}, err
	}
	resumed, err := rt2.ResumePendingApproval(ctx, mappedSessionID)
	if err != nil {
		return DemoResult{}, err
	}
	actions, err := rt2.ListActions(mappedSessionID)
	if err != nil {
		return DemoResult{}, err
	}
	stdout, _ := resumed.Execution.Action.Data["stdout"].(string)
	return DemoResult{
		RunID:           runID,
		SessionID:       sess.SessionID,
		MappedSessionID: mappedSessionID,
		ApprovalID:      approvalID,
		FinalPhase:      resumed.Session.Phase,
		Output:          stdout,
		ActionCount:     len(actions),
	}, nil
}

func durableExampleOptions() hruntime.Options {
	var opts hruntime.Options
	builtins.Register(&opts)
	opts.Policy = platformApprovalPolicy{}
	return opts
}
