package runtime_test

import (
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

func TestClassifyErrorProvidesTransportNeutralKernelCategories(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		wantKind  hruntime.ErrorKind
		retryable bool
	}{
		{name: "session conflict", err: session.ErrSessionVersionConflict, wantKind: hruntime.ErrorKindConflict, retryable: true},
		{name: "approval conflict", err: approval.ErrApprovalVersionConflict, wantKind: hruntime.ErrorKindConflict, retryable: true},
		{name: "runtime handle conflict", err: execution.ErrRuntimeHandleVersionConflict, wantKind: hruntime.ErrorKindConflict, retryable: true},
		{name: "session not found", err: session.ErrSessionNotFound, wantKind: hruntime.ErrorKindNotFound, retryable: false},
		{name: "approval not found", err: approval.ErrApprovalNotFound, wantKind: hruntime.ErrorKindNotFound, retryable: false},
		{name: "execution record not found", err: execution.ErrRecordNotFound, wantKind: hruntime.ErrorKindNotFound, retryable: false},
		{name: "runtime budget", err: hruntime.ErrRuntimeBudgetExceeded, wantKind: hruntime.ErrorKindBudget, retryable: false},
		{name: "plan revision budget", err: hruntime.ErrPlanRevisionBudgetExceeded, wantKind: hruntime.ErrorKindBudget, retryable: false},
		{name: "step retry budget", err: hruntime.ErrStepRetryBudgetExceeded, wantKind: hruntime.ErrorKindBudget, retryable: false},
		{name: "step backoff", err: hruntime.ErrStepBackoffActive, wantKind: hruntime.ErrorKindBudget, retryable: true},
		{name: "lease not held", err: session.ErrSessionLeaseNotHeld, wantKind: hruntime.ErrorKindLease, retryable: true},
		{name: "runtime handle inactive", err: hruntime.ErrRuntimeHandleNotActive, wantKind: hruntime.ErrorKindRuntimeHandle, retryable: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info := hruntime.ClassifyError(tc.err)
			if info.Kind != tc.wantKind || info.Retryable != tc.retryable {
				t.Fatalf("expected %+v for %v, got %+v", struct {
					Kind      hruntime.ErrorKind
					Retryable bool
				}{Kind: tc.wantKind, Retryable: tc.retryable}, tc.err, info)
			}
			if got := hruntime.IsRetryable(tc.err); got != tc.retryable {
				t.Fatalf("expected IsRetryable(%v)=%v, got %v", tc.err, tc.retryable, got)
			}
		})
	}
}

func TestClassifyErrorFallsBackToUnknownForUnmappedErrors(t *testing.T) {
	info := hruntime.ClassifyError(assertionError("boom"))
	if info.Kind != hruntime.ErrorKindUnknown || info.Retryable {
		t.Fatalf("expected unknown/non-retryable for unmapped error, got %+v", info)
	}
}

type assertionError string

func (e assertionError) Error() string { return string(e) }
