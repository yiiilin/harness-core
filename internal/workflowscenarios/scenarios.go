package workflowscenarios

import (
	"context"
	"fmt"
	"time"

	"github.com/yiiilin/harness-core/pkg/harness"
	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/builtins"
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

const (
	plannerCommand  = "echo planner walkthrough"
	approvalCommand = "echo approval walkthrough"
	recoveryCommand = "echo recovery walkthrough"
)

type Summary struct {
	SessionID         string
	Phase             session.Phase
	Stdout            string
	VerifySuccess     bool
	AttemptCount      int
	ActionCount       int
	VerificationCount int
	ReplayCycles      int
	ReplayEvents      int
}

type PlannerResult struct {
	Summary
	ToolName  string
	StepTitle string
}

type ApprovalResult struct {
	Summary
	FirstRunApprovalPending bool
	PendingApprovalID       string
	ActionsBeforeApproval   int
	ApprovalStatus          approval.Status
}

type RecoveryResult struct {
	Summary
	Recovered     bool
	LeaseReleased bool
}

type Results struct {
	Planner  PlannerResult
	Approval ApprovalResult
	Recovery RecoveryResult
}

func Run(ctx context.Context) (Results, error) {
	plannerResult, err := runPlannerScenario(ctx)
	if err != nil {
		return Results{}, err
	}
	approvalResult, err := runApprovalScenario(ctx)
	if err != nil {
		return Results{}, err
	}
	recoveryResult, err := runRecoveryScenario(ctx)
	if err != nil {
		return Results{}, err
	}
	return Results{
		Planner:  plannerResult,
		Approval: approvalResult,
		Recovery: recoveryResult,
	}, nil
}

func runPlannerScenario(ctx context.Context) (PlannerResult, error) {
	rt := newRuntimeWithBuiltins(nil).WithPlanner(hruntime.DemoPlanner{})
	sess, _, err := seedSessionAndTask(rt, "planner-pipe", plannerCommand)
	if err != nil {
		return PlannerResult{}, err
	}
	_, _, err = rt.CreatePlanFromPlanner(ctx, sess.SessionID, "planner walkthrough", 1)
	if err != nil {
		return PlannerResult{}, err
	}
	run, err := rt.RunSession(ctx, sess.SessionID)
	if err != nil {
		return PlannerResult{}, err
	}
	stepOut, err := lastStepExecution(run.Executions)
	if err != nil {
		return PlannerResult{}, err
	}
	summary, err := buildSummary(rt, sess.SessionID, run.Session.Phase, stepOut.Execution.Action, stepOut.Execution.Verify.Success)
	if err != nil {
		return PlannerResult{}, err
	}
	return PlannerResult{
		Summary:   summary,
		ToolName:  stepOut.Execution.Step.Action.ToolName,
		StepTitle: stepOut.Execution.Step.Title,
	}, nil
}

func runApprovalScenario(ctx context.Context) (ApprovalResult, error) {
	rt := newRuntimeWithBuiltins(permission.RulesEvaluator{
		Rules: []permission.Rule{
			{Permission: "shell.exec", Pattern: "*", Action: permission.Ask},
		},
		Fallback: permission.DefaultEvaluator{},
	})
	sess, _, err := seedSessionAndTask(rt, "approval-resume", approvalCommand)
	if err != nil {
		return ApprovalResult{}, err
	}
	if _, err := rt.CreatePlan(sess.SessionID, "approval walkthrough", []plan.StepSpec{shellEchoStep("step_approval", "approval-resume", approvalCommand, "approval walkthrough")}); err != nil {
		return ApprovalResult{}, err
	}
	workerHelper, err := harness.NewWorkerHelper(harness.WorkerOptions{
		Runtime:       rt,
		LeaseTTL:      time.Minute,
		RenewInterval: 10 * time.Millisecond,
	})
	if err != nil {
		return ApprovalResult{}, err
	}
	first, err := workerHelper.RunOnce(ctx)
	if err != nil {
		return ApprovalResult{}, err
	}
	actionsBeforeApproval, err := countActions(rt, sess.SessionID)
	if err != nil {
		return ApprovalResult{}, err
	}
	approvals, err := rt.ListApprovals(sess.SessionID)
	if err != nil {
		return ApprovalResult{}, err
	}
	if len(approvals) != 1 {
		return ApprovalResult{}, fmt.Errorf("expected one approval, got %d", len(approvals))
	}
	approvalRecord, _, err := rt.RespondApproval(approvals[0].ApprovalID, harness.ApprovalResponse{Reply: approval.ReplyOnce})
	if err != nil {
		return ApprovalResult{}, err
	}
	second, err := workerHelper.RunOnce(ctx)
	if err != nil {
		return ApprovalResult{}, err
	}
	stepOut, err := lastStepExecution(second.Run.Executions)
	if err != nil {
		return ApprovalResult{}, err
	}
	summary, err := buildSummary(rt, sess.SessionID, second.Run.Session.Phase, stepOut.Execution.Action, stepOut.Execution.Verify.Success)
	if err != nil {
		return ApprovalResult{}, err
	}
	return ApprovalResult{
		Summary:                 summary,
		FirstRunApprovalPending: first.ApprovalPending,
		PendingApprovalID:       approvalRecord.ApprovalID,
		ActionsBeforeApproval:   actionsBeforeApproval,
		ApprovalStatus:          approvalRecord.Status,
	}, nil
}

func runRecoveryScenario(ctx context.Context) (RecoveryResult, error) {
	rt := newRuntimeWithBuiltins(nil)
	sess, _, err := seedSessionAndTask(rt, "recover-interrupted", recoveryCommand)
	if err != nil {
		return RecoveryResult{}, err
	}
	step := shellEchoStep("step_recovery", "recover-interrupted", recoveryCommand, "recovery walkthrough")
	if _, err := rt.CreatePlan(sess.SessionID, "recovery walkthrough", []plan.StepSpec{step}); err != nil {
		return RecoveryResult{}, err
	}
	claimed, ok, err := rt.ClaimRunnableSession(ctx, time.Minute)
	if err != nil {
		return RecoveryResult{}, err
	}
	if !ok {
		return RecoveryResult{}, fmt.Errorf("expected seeded session %s to be claimable", sess.SessionID)
	}
	if _, err := rt.MarkClaimedSessionInFlight(ctx, sess.SessionID, claimed.LeaseID, step.StepID); err != nil {
		return RecoveryResult{}, err
	}
	if _, err := rt.MarkClaimedSessionInterrupted(ctx, sess.SessionID, claimed.LeaseID); err != nil {
		return RecoveryResult{}, err
	}
	if _, err := rt.ReleaseSessionLease(ctx, sess.SessionID, claimed.LeaseID); err != nil {
		return RecoveryResult{}, err
	}
	workerHelper, err := harness.NewWorkerHelper(harness.WorkerOptions{
		Runtime:       rt,
		LeaseTTL:      time.Minute,
		RenewInterval: 10 * time.Millisecond,
	})
	if err != nil {
		return RecoveryResult{}, err
	}
	run, err := workerHelper.RunOnce(ctx)
	if err != nil {
		return RecoveryResult{}, err
	}
	stepOut, err := lastStepExecution(run.Run.Executions)
	if err != nil {
		return RecoveryResult{}, err
	}
	summary, err := buildSummary(rt, sess.SessionID, run.Run.Session.Phase, stepOut.Execution.Action, stepOut.Execution.Verify.Success)
	if err != nil {
		return RecoveryResult{}, err
	}
	return RecoveryResult{
		Summary:       summary,
		Recovered:     run.Mode == session.ClaimModeRecoverable,
		LeaseReleased: run.Released.LeaseID == "" && run.Released.LeaseExpiresAt == 0,
	}, nil
}

func newRuntimeWithBuiltins(policy permission.Evaluator) *harness.Service {
	opts := harness.Options{}
	builtins.Register(&opts)
	if policy != nil {
		opts.Policy = policy
	}
	return harness.New(opts)
}

func seedSessionAndTask(rt *harness.Service, title, goal string) (harness.SessionState, harness.TaskRecord, error) {
	sess, err := rt.CreateSession(title, goal)
	if err != nil {
		return harness.SessionState{}, harness.TaskRecord{}, err
	}
	tsk, err := rt.CreateTask(task.Spec{TaskType: "demo", Goal: goal})
	if err != nil {
		return harness.SessionState{}, harness.TaskRecord{}, err
	}
	sess, err = rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		return harness.SessionState{}, harness.TaskRecord{}, err
	}
	return sess, tsk, nil
}

func shellEchoStep(stepID, title, command, expected string) plan.StepSpec {
	return plan.StepSpec{
		StepID: stepID,
		Title:  title,
		Action: action.Spec{
			ToolName: "shell.exec",
			Args: map[string]any{
				"mode":       "pipe",
				"command":    command,
				"timeout_ms": 5000,
			},
		},
		Verify: verify.Spec{
			Mode: verify.ModeAll,
			Checks: []verify.Check{
				{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
				{Kind: "output_contains", Args: map[string]any{"text": expected}},
			},
		},
		OnFail: plan.OnFailSpec{Strategy: "abort"},
	}
}

func buildSummary(rt *harness.Service, sessionID string, phase session.Phase, result action.Result, verifySuccess bool) (Summary, error) {
	attempts, err := rt.ListAttempts(sessionID)
	if err != nil {
		return Summary{}, err
	}
	actions, err := rt.ListActions(sessionID)
	if err != nil {
		return Summary{}, err
	}
	verifications, err := rt.ListVerifications(sessionID)
	if err != nil {
		return Summary{}, err
	}
	projection, err := harness.NewReplayReader(rt).SessionProjection(sessionID)
	if err != nil {
		return Summary{}, err
	}
	return Summary{
		SessionID:         sessionID,
		Phase:             phase,
		Stdout:            stdoutFromResult(result),
		VerifySuccess:     verifySuccess,
		AttemptCount:      len(attempts),
		ActionCount:       len(actions),
		VerificationCount: len(verifications),
		ReplayCycles:      len(projection.Cycles),
		ReplayEvents:      len(projection.Events),
	}, nil
}

func countActions(rt *harness.Service, sessionID string) (int, error) {
	actions, err := rt.ListActions(sessionID)
	if err != nil {
		return 0, err
	}
	return len(actions), nil
}

func lastStepExecution(executions []hruntime.StepRunOutput) (hruntime.StepRunOutput, error) {
	if len(executions) == 0 {
		return hruntime.StepRunOutput{}, fmt.Errorf("expected at least one step execution")
	}
	return executions[len(executions)-1], nil
}

func stdoutFromResult(result action.Result) string {
	stdout, _ := result.Data["stdout"].(string)
	return stdout
}
