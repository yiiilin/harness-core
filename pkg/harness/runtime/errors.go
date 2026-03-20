package runtime

import "errors"

var ErrSessionTerminal = errors.New("session is terminal")
var ErrSessionAwaitingApproval = errors.New("session is awaiting approval")
var ErrNoPendingApproval = errors.New("session has no pending approval")
var ErrApprovalNotResolved = errors.New("approval is not ready to resume")
