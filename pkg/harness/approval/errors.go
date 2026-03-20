package approval

import "errors"

var ErrApprovalNotFound = errors.New("approval not found")
var ErrApprovalNotPending = errors.New("approval is not pending")
var ErrInvalidReply = errors.New("invalid approval reply")
var ErrApprovalVersionConflict = errors.New("approval version conflict")
