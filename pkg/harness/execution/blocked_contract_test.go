package execution_test

import (
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/execution"
)

func TestBlockedRuntimeSubjectHelpers(t *testing.T) {
	subject := execution.BlockedRuntimeSubject{
		ActionID: "action-1",
		Target:   execution.TargetRef{TargetID: "target-1"},
	}
	if !subject.ReferencesAction() {
		t.Fatalf("expected action reference")
	}
	if !subject.ReferencesTarget() {
		t.Fatalf("expected target reference")
	}
}

func TestBlockedRuntimeConditionHelpers(t *testing.T) {
	condition := execution.BlockedRuntimeCondition{
		Kind:        execution.BlockedRuntimeConditionInteractive,
		ReferenceID: "handle-1",
	}
	if !condition.HasReference() {
		t.Fatalf("expected condition reference")
	}
}

func TestBlockedRuntimeRecordSupportsGenericKindsAndStatuses(t *testing.T) {
	record := execution.BlockedRuntimeRecord{
		BlockedRuntimeID: "blocked-1",
		Kind:             execution.BlockedRuntimeInteractive,
		Status:           execution.BlockedRuntimeAborted,
		SessionID:        "session-1",
		Subject:          execution.BlockedRuntimeSubject{StepID: "step-1"},
		Condition:        execution.BlockedRuntimeCondition{Kind: execution.BlockedRuntimeConditionInteractive},
	}
	if record.Kind != execution.BlockedRuntimeInteractive {
		t.Fatalf("unexpected kind: %s", record.Kind)
	}
	if record.Status != execution.BlockedRuntimeAborted {
		t.Fatalf("unexpected status: %s", record.Status)
	}
}
