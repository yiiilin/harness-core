package evals_test

import (
	"context"
	"strings"
	"testing"

	"github.com/yiiilin/harness-core/internal/workflowscenarios"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

func TestWorkflowEvalConcreteShellScenarios(t *testing.T) {
	result, err := workflowscenarios.Run(context.Background())
	if err != nil {
		t.Fatalf("run workflow scenarios: %v", err)
	}

	if result.Planner.Phase != session.PhaseComplete {
		t.Fatalf("expected planner scenario to complete, got %#v", result.Planner)
	}
	if !result.Planner.VerifySuccess || !strings.Contains(result.Planner.Stdout, "planner walkthrough") {
		t.Fatalf("expected planner scenario to verify shell output, got %#v", result.Planner)
	}

	if !result.Approval.FirstRunApprovalPending || result.Approval.PendingApprovalID == "" {
		t.Fatalf("expected approval scenario to pause before approval, got %#v", result.Approval)
	}
	if result.Approval.ActionsBeforeApproval != 0 {
		t.Fatalf("expected approval scenario to avoid action execution before approval, got %#v", result.Approval)
	}
	if result.Approval.Phase != session.PhaseComplete || !strings.Contains(result.Approval.Stdout, "approval walkthrough") {
		t.Fatalf("expected approval scenario to complete after approval, got %#v", result.Approval)
	}

	if !result.Recovery.Recovered || !result.Recovery.LeaseReleased {
		t.Fatalf("expected recovery scenario to recover and release lease, got %#v", result.Recovery)
	}
	if result.Recovery.Phase != session.PhaseComplete || !strings.Contains(result.Recovery.Stdout, "recovery walkthrough") {
		t.Fatalf("expected recovery scenario to complete with shell output, got %#v", result.Recovery)
	}
}
