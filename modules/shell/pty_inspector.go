package shellmodule

import "context"

// PTYInspector is the module-level read/inspection surface needed by PTY
// verifiers. Local PTYManager implements it, and remote embedders can provide
// their own inspector without exposing manager internals.
type PTYInspector interface {
	Inspect(ctx context.Context, handleID string) (PTYInspectResult, error)
	Read(ctx context.Context, handleID string, req PTYReadRequest) (PTYReadResult, error)
}

type PTYInspectResult struct {
	HandleID     string `json:"handle_id"`
	Closed       bool   `json:"closed,omitempty"`
	ExitCode     int    `json:"exit_code,omitempty"`
	Status       string `json:"status"`
	StatusReason string `json:"status_reason,omitempty"`
}
