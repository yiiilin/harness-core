package shell

import "context"

type ExecRequest struct {
	Mode      string `json:"mode"`
	Command   string `json:"command"`
	CWD       string `json:"cwd,omitempty"`
	TimeoutMS int    `json:"timeout_ms,omitempty"`
}

type ExecResult struct {
	Status   string `json:"status"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	ExitCode int    `json:"exit_code,omitempty"`
}

func Exec(ctx context.Context, req ExecRequest) (ExecResult, error) {
	_ = ctx
	return ExecResult{Status: "not_implemented"}, nil
}
