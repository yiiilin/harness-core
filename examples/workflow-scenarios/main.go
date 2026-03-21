// Command workflow-scenarios runs a few concrete tasks through the public harness workflow
// surfaces and prints the resulting execution summaries.
package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/yiiilin/harness-core/internal/workflowscenarios"
)

func main() {
	results, err := workflowscenarios.Run(context.Background())
	if err != nil {
		panic(err)
	}

	printScenario("planner-pipe", []string{
		fmt.Sprintf("tool: %s", results.Planner.ToolName),
		fmt.Sprintf("phase: %s", results.Planner.Phase),
		fmt.Sprintf("stdout: %s", trimLine(results.Planner.Stdout)),
		fmt.Sprintf("verify: %v", results.Planner.VerifySuccess),
		fmt.Sprintf("persisted: attempts=%d actions=%d verifications=%d replay_cycles=%d replay_events=%d",
			results.Planner.AttemptCount,
			results.Planner.ActionCount,
			results.Planner.VerificationCount,
			results.Planner.ReplayCycles,
			results.Planner.ReplayEvents,
		),
	})

	printScenario("approval-resume", []string{
		fmt.Sprintf("first_run_approval_pending: %v", results.Approval.FirstRunApprovalPending),
		fmt.Sprintf("pending_approval: %s", results.Approval.PendingApprovalID),
		fmt.Sprintf("actions_before_approval: %d", results.Approval.ActionsBeforeApproval),
		fmt.Sprintf("approval_status: %s", results.Approval.ApprovalStatus),
		fmt.Sprintf("phase: %s", results.Approval.Phase),
		fmt.Sprintf("stdout: %s", trimLine(results.Approval.Stdout)),
		fmt.Sprintf("persisted: attempts=%d actions=%d verifications=%d replay_cycles=%d replay_events=%d",
			results.Approval.AttemptCount,
			results.Approval.ActionCount,
			results.Approval.VerificationCount,
			results.Approval.ReplayCycles,
			results.Approval.ReplayEvents,
		),
	})

	printScenario("recover-interrupted", []string{
		fmt.Sprintf("recovered: %v", results.Recovery.Recovered),
		fmt.Sprintf("lease_released: %v", results.Recovery.LeaseReleased),
		fmt.Sprintf("phase: %s", results.Recovery.Phase),
		fmt.Sprintf("stdout: %s", trimLine(results.Recovery.Stdout)),
		fmt.Sprintf("persisted: attempts=%d actions=%d verifications=%d replay_cycles=%d replay_events=%d",
			results.Recovery.AttemptCount,
			results.Recovery.ActionCount,
			results.Recovery.VerificationCount,
			results.Recovery.ReplayCycles,
			results.Recovery.ReplayEvents,
		),
	})
}

func printScenario(name string, lines []string) {
	fmt.Printf("scenario: %s\n", name)
	for _, line := range lines {
		fmt.Printf("  %s\n", line)
	}
}

func trimLine(value string) string {
	return strings.TrimSpace(value)
}
