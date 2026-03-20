package audit

const (
	EventTaskCreated       = "task.created"
	EventSessionCreated    = "session.created"
	EventPlanGenerated     = "plan.generated"
	EventStepStarted       = "step.started"
	EventApprovalRequested = "approval.requested"
	EventApprovalApproved  = "approval.approved"
	EventApprovalRejected  = "approval.rejected"
	EventToolCalled        = "tool.called"
	EventToolCompleted     = "tool.completed"
	EventToolFailed        = "tool.failed"
	EventVerifyCompleted   = "verify.completed"
	EventStateChanged      = "state.changed"
	EventSessionAborted    = "session.aborted"
	EventPolicyDenied      = "policy.denied"
	EventTaskCompleted     = "task.completed"
	EventTaskFailed        = "task.failed"
	EventTaskAborted       = "task.aborted"
)

type Event struct {
	EventID        string         `json:"event_id"`
	Type           string         `json:"type"`
	SessionID      string         `json:"session_id,omitempty"`
	TaskID         string         `json:"task_id,omitempty"`
	PlanningID     string         `json:"planning_id,omitempty"`
	StepID         string         `json:"step_id,omitempty"`
	AttemptID      string         `json:"attempt_id,omitempty"`
	ActionID       string         `json:"action_id,omitempty"`
	VerificationID string         `json:"verification_id,omitempty"`
	TraceID        string         `json:"trace_id,omitempty"`
	CausationID    string         `json:"causation_id,omitempty"`
	Payload        map[string]any `json:"payload,omitempty"`
	CreatedAt      int64          `json:"created_at"`
}
