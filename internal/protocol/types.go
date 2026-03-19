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
