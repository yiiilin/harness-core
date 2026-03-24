package audit

const (
	EventTaskCreated              = "task.created"
	EventSessionCreated           = "session.created"
	EventSessionTaskAttached      = "session.task_attached"
	EventPlanGenerated            = "plan.generated"
	EventStepStarted              = "step.started"
	EventApprovalRequested        = "approval.requested"
	EventApprovalApproved         = "approval.approved"
	EventApprovalRejected         = "approval.rejected"
	EventToolCalled               = "tool.called"
	EventToolCompleted            = "tool.completed"
	EventToolFailed               = "tool.failed"
	EventVerifyCompleted          = "verify.completed"
	EventStateChanged             = "state.changed"
	EventSessionAborted           = "session.aborted"
	EventPolicyDenied             = "policy.denied"
	EventTaskCompleted            = "task.completed"
	EventTaskFailed               = "task.failed"
	EventTaskAborted              = "task.aborted"
	EventLeaseClaimed             = "lease.claimed"
	EventLeaseRenewed             = "lease.renewed"
	EventLeaseReleased            = "lease.released"
	EventRecoveryStateChanged     = "recovery.state_changed"
	EventRuntimeHandleCreated     = "runtime_handle.created"
	EventRuntimeHandleUpdated     = "runtime_handle.updated"
	EventRuntimeHandleClosed      = "runtime_handle.closed"
	EventRuntimeHandleInvalidated = "runtime_handle.invalidated"
	EventBlockedRuntimeCreated    = "blocked_runtime.created"
	EventBlockedRuntimeResponded  = "blocked_runtime.responded"
	EventBlockedRuntimeResumed    = "blocked_runtime.resumed"
	EventBlockedRuntimeAborted    = "blocked_runtime.aborted"
)

type Event struct {
	EventID        string         `json:"event_id"`
	Sequence       int64          `json:"sequence,omitempty"`
	Type           string         `json:"type"`
	SessionID      string         `json:"session_id,omitempty"`
	TaskID         string         `json:"task_id,omitempty"`
	PlanningID     string         `json:"planning_id,omitempty"`
	ApprovalID     string         `json:"approval_id,omitempty"`
	StepID         string         `json:"step_id,omitempty"`
	AttemptID      string         `json:"attempt_id,omitempty"`
	ActionID       string         `json:"action_id,omitempty"`
	VerificationID string         `json:"verification_id,omitempty"`
	CycleID        string         `json:"cycle_id,omitempty"`
	TraceID        string         `json:"trace_id,omitempty"`
	CausationID    string         `json:"causation_id,omitempty"`
	Payload        map[string]any `json:"payload,omitempty"`
	CreatedAt      int64          `json:"created_at"`
}
