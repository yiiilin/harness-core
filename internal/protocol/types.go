package protocol

import "encoding/json"

const (
	EnvelopeTypeAuth     = "auth"
	EnvelopeTypeRequest  = "request"
	EnvelopeTypeResponse = "response"
	EnvelopeTypeEvent    = "event"
)

type Envelope struct {
	ID      string          `json:"id,omitempty"`
	Type    string          `json:"type"`
	Action  string          `json:"action,omitempty"`
	Token   string          `json:"token,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Response struct {
	ID     string      `json:"id,omitempty"`
	Type   string      `json:"type"`
	OK     bool        `json:"ok"`
	Result interface{} `json:"result,omitempty"`
	Error  *ErrorBody  `json:"error,omitempty"`
}

type SessionCreatePayload struct {
	Title string `json:"title"`
	Goal  string `json:"goal,omitempty"`
}

type SessionGetPayload struct {
	ID string `json:"id"`
}

type TaskCreatePayload struct {
	TaskType    string         `json:"task_type"`
	Goal        string         `json:"goal"`
	Constraints map[string]any `json:"constraints,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type SessionAttachTaskPayload struct {
	SessionID string `json:"session_id"`
	TaskID    string `json:"task_id"`
}

type PlanCreatePayload struct {
	SessionID    string            `json:"session_id"`
	ChangeReason string            `json:"change_reason,omitempty"`
	Steps        []json.RawMessage `json:"steps"`
}

type PlanGetPayload struct {
	PlanID string `json:"plan_id"`
}

type PlanListPayload struct {
	SessionID string `json:"session_id"`
}

type StepRunPayload struct {
	SessionID string          `json:"session_id"`
	Step      json.RawMessage `json:"step"`
}

type SessionResumePayload struct {
	SessionID string `json:"session_id"`
}

type AuditListPayload struct {
	SessionID string `json:"session_id,omitempty"`
}

type ApprovalGetPayload struct {
	ApprovalID string `json:"approval_id"`
}

type ApprovalListPayload struct {
	SessionID string `json:"session_id,omitempty"`
}

type ApprovalRespondPayload struct {
	ApprovalID string         `json:"approval_id"`
	Reply      string         `json:"reply"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type SessionScopedPayload struct {
	SessionID string `json:"session_id,omitempty"`
}

type RuntimeHandleGetPayload struct {
	HandleID string `json:"handle_id"`
}
