package shell_test

import (
	"context"
	"path/filepath"
	"testing"

	shellexec "github.com/yiiilin/harness-core/pkg/harness/executor/shell"
)

func TestPipeExecutorTruncatesOutputAndReportsMetadata(t *testing.T) {
	exec := shellexec.PipeExecutor{MaxOutputBytes: 5}

	result, err := exec.Execute(context.Background(), shellexec.Request{
		Command:   "printf 'hello world'",
		TimeoutMS: 5000,
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !result.OK {
		t.Fatalf("expected ok result, got %#v", result)
	}
	stdout, _ := result.Data["stdout"].(string)
	if stdout != "hello" {
		t.Fatalf("expected truncated stdout, got %q", stdout)
	}
	if truncated, _ := result.Meta["stdout_truncated"].(bool); !truncated {
		t.Fatalf("expected stdout_truncated metadata, got %#v", result.Meta)
	}
	if originalBytes, _ := result.Meta["stdout_original_bytes"].(int); originalBytes != len("hello world") {
		t.Fatalf("expected stdout_original_bytes %d, got %#v", len("hello world"), result.Meta["stdout_original_bytes"])
	}
}

func TestPipeExecutorRejectsDisallowedWorkingDirectory(t *testing.T) {
	allowed := t.TempDir()
	exec := shellexec.PipeExecutor{AllowedCWDPrefixes: []string{allowed}}

	result, err := exec.Execute(context.Background(), shellexec.Request{
		Command:   "pwd",
		CWD:       "/etc",
		TimeoutMS: 5000,
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.OK {
		t.Fatalf("expected blocked result, got %#v", result)
	}
	if result.Error == nil || result.Error.Code != "CWD_NOT_ALLOWED" {
		t.Fatalf("expected CWD_NOT_ALLOWED, got %#v", result)
	}
	status, _ := result.Data["status"].(string)
	if status != "blocked" {
		t.Fatalf("expected blocked status, got %#v", result.Data["status"])
	}

	allowedResult, err := exec.Execute(context.Background(), shellexec.Request{
		Command:   "pwd",
		CWD:       filepath.Clean(allowed),
		TimeoutMS: 5000,
	})
	if err != nil {
		t.Fatalf("execute allowed cwd: %v", err)
	}
	if !allowedResult.OK {
		t.Fatalf("expected allowed cwd to succeed, got %#v", allowedResult)
	}
}

func TestPipeExecutorClassifiesTimeoutStartFailureAndExitFailure(t *testing.T) {
	exec := shellexec.PipeExecutor{}

	timeoutResult, err := exec.Execute(context.Background(), shellexec.Request{
		Command:   "sleep 1",
		TimeoutMS: 10,
	})
	if err != nil {
		t.Fatalf("execute timeout: %v", err)
	}
	if timeoutResult.Error == nil || timeoutResult.Error.Code != "COMMAND_TIMED_OUT" {
		t.Fatalf("expected COMMAND_TIMED_OUT, got %#v", timeoutResult)
	}
	if status, _ := timeoutResult.Data["status"].(string); status != "timed_out" {
		t.Fatalf("expected timed_out status, got %#v", timeoutResult.Data["status"])
	}

	startFailureResult, err := exec.Execute(context.Background(), shellexec.Request{
		Command:   "pwd",
		CWD:       "/definitely/not/a/real/path",
		TimeoutMS: 5000,
	})
	if err != nil {
		t.Fatalf("execute start failure: %v", err)
	}
	if startFailureResult.Error == nil || startFailureResult.Error.Code != "COMMAND_START_FAILED" {
		t.Fatalf("expected COMMAND_START_FAILED, got %#v", startFailureResult)
	}
	if status, _ := startFailureResult.Data["status"].(string); status != "start_failed" {
		t.Fatalf("expected start_failed status, got %#v", startFailureResult.Data["status"])
	}

	exitFailureResult, err := exec.Execute(context.Background(), shellexec.Request{
		Command:   "false",
		TimeoutMS: 5000,
	})
	if err != nil {
		t.Fatalf("execute exit failure: %v", err)
	}
	if exitFailureResult.Error == nil || exitFailureResult.Error.Code != "COMMAND_EXIT_NONZERO" {
		t.Fatalf("expected COMMAND_EXIT_NONZERO, got %#v", exitFailureResult)
	}
	if exitCode, _ := exitFailureResult.Data["exit_code"].(int); exitCode != 1 {
		t.Fatalf("expected exit code 1, got %#v", exitFailureResult.Data["exit_code"])
	}
}
