package runtime

import (
	"errors"

	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

type ErrorKind string

const (
	ErrorKindUnknown       ErrorKind = "unknown"
	ErrorKindConflict      ErrorKind = "conflict"
	ErrorKindNotFound      ErrorKind = "not_found"
	ErrorKindBudget        ErrorKind = "budget"
	ErrorKindLease         ErrorKind = "lease"
	ErrorKindRuntimeHandle ErrorKind = "runtime_handle"
	ErrorKindInvalid       ErrorKind = "invalid"
	ErrorKindState         ErrorKind = "state"
)

type ErrorInfo struct {
	Kind      ErrorKind `json:"kind"`
	Retryable bool      `json:"retryable"`
}

func ClassifyError(err error) ErrorInfo {
	switch {
	case err == nil:
		return ErrorInfo{Kind: ErrorKindUnknown, Retryable: false}
	case errors.Is(err, session.ErrSessionVersionConflict),
		errors.Is(err, approval.ErrApprovalVersionConflict):
		return ErrorInfo{Kind: ErrorKindConflict, Retryable: true}
	case errors.Is(err, session.ErrSessionNotFound),
		errors.Is(err, approval.ErrApprovalNotFound),
		errors.Is(err, execution.ErrRecordNotFound),
		errors.Is(err, ErrContextSummaryNotFound):
		return ErrorInfo{Kind: ErrorKindNotFound, Retryable: false}
	case errors.Is(err, ErrStepBackoffActive):
		return ErrorInfo{Kind: ErrorKindBudget, Retryable: true}
	case errors.Is(err, ErrPlanRevisionBudgetExceeded),
		errors.Is(err, ErrRuntimeBudgetExceeded),
		errors.Is(err, ErrStepRetryBudgetExceeded):
		return ErrorInfo{Kind: ErrorKindBudget, Retryable: false}
	case errors.Is(err, session.ErrSessionLeaseNotHeld):
		return ErrorInfo{Kind: ErrorKindLease, Retryable: true}
	case errors.Is(err, ErrRuntimeHandleNotActive):
		return ErrorInfo{Kind: ErrorKindRuntimeHandle, Retryable: false}
	case errors.Is(err, approval.ErrInvalidReply),
		errors.Is(err, ErrInvalidLeaseTTL),
		errors.Is(err, ErrDirectActionInvokeUnsupported):
		return ErrorInfo{Kind: ErrorKindInvalid, Retryable: false}
	case errors.Is(err, ErrSessionTerminal),
		errors.Is(err, ErrSessionAwaitingApproval),
		errors.Is(err, ErrNoPendingApproval),
		errors.Is(err, ErrApprovalNotResolved):
		return ErrorInfo{Kind: ErrorKindState, Retryable: false}
	default:
		return ErrorInfo{Kind: ErrorKindUnknown, Retryable: false}
	}
}

func IsRetryable(err error) bool {
	return ClassifyError(err).Retryable
}
