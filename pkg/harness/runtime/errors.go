package runtime

import "errors"

var ErrSessionTerminal = errors.New("session is terminal")
var ErrSessionAwaitingApproval = errors.New("session is awaiting approval")
var ErrNoPendingApproval = errors.New("session has no pending approval")
var ErrApprovalNotResolved = errors.New("approval is not ready to resume")
var ErrDirectActionInvokeUnsupported = errors.New("direct action invocation is unsupported; use the step runtime path")
var ErrPlanRevisionBudgetExceeded = errors.New("plan revision budget exceeded")
var ErrRuntimeBudgetExceeded = errors.New("total runtime budget exceeded")
var ErrStepRetryBudgetExceeded = errors.New("step retry budget exceeded")
