package shell_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

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
	if stdout != "h...d" {
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

func TestPipeExecutorPreservesRawOutputAndReportsRecoverablePreviewMetadata(t *testing.T) {
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
	if stdout != "h...d" {
		t.Fatalf("expected preview stdout to be truncated, got %#v", result.Data["stdout"])
	}
	if result.Raw == nil {
		t.Fatalf("expected raw result channel, got %#v", result)
	}
	rawStdout, _ := result.Raw.Data["stdout"].(string)
	if rawStdout != "hello world" {
		t.Fatalf("expected raw stdout to remain recoverable, got %#v", result.Raw)
	}

	window, ok := result.Meta["stdout_preview"].(map[string]any)
	if !ok {
		t.Fatalf("expected standardized stdout_preview metadata, got %#v", result.Meta)
	}
	if truncated, _ := window["truncated"].(bool); !truncated {
		t.Fatalf("expected preview truncation metadata, got %#v", window)
	}
	if originalBytes, _ := window["original_bytes"].(int); originalBytes != len("hello world") {
		t.Fatalf("expected original_bytes %d, got %#v", len("hello world"), window["original_bytes"])
	}
	if returnedBytes, _ := window["returned_bytes"].(int); returnedBytes != len("hello") {
		t.Fatalf("expected returned_bytes %d, got %#v", len("hello"), window["returned_bytes"])
	}
	if mode, _ := window["preview_mode"].(string); mode != "head_tail" {
		t.Fatalf("expected head_tail preview mode, got %#v", window)
	}
	if hasMore, _ := window["has_more"].(bool); !hasMore {
		t.Fatalf("expected has_more metadata, got %#v", window)
	}
	if _, exists := window["next_offset"]; exists {
		t.Fatalf("did not expect head-tail preview metadata to expose next_offset, got %#v", window)
	}
}

func TestPipeExecutorUsesHeadTailPreviewWhenOutputIsTruncated(t *testing.T) {
	exec := shellexec.PipeExecutor{MaxOutputBytes: 13}

	result, err := exec.Execute(context.Background(), shellexec.Request{
		Command:   "printf 'abcdefghijklmnopqrst'",
		TimeoutMS: 5000,
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !result.OK {
		t.Fatalf("expected ok result, got %#v", result)
	}

	stdout, _ := result.Data["stdout"].(string)
	assertHeadTailPreview(t, stdout, 13, "abcd", "opqrst")
	if result.Raw == nil {
		t.Fatalf("expected raw result channel, got %#v", result)
	}
	rawStdout, _ := result.Raw.Data["stdout"].(string)
	if rawStdout != "abcdefghijklmnopqrst" {
		t.Fatalf("expected raw stdout to remain intact, got %#v", result.Raw)
	}
}

func TestPipeExecutorReportsHeadTailPreviewMetadataWhenOutputIsTruncated(t *testing.T) {
	exec := shellexec.PipeExecutor{MaxOutputBytes: 13}

	result, err := exec.Execute(context.Background(), shellexec.Request{
		Command:   "printf 'abcdefghijklmnopqrst'",
		TimeoutMS: 5000,
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	window, ok := result.Meta["stdout_preview"].(map[string]any)
	if !ok {
		t.Fatalf("expected stdout_preview metadata, got %#v", result.Meta)
	}
	if mode, _ := window["preview_mode"].(string); mode != "head_tail" {
		t.Fatalf("expected head_tail preview mode, got %#v", window)
	}
	if headBytes, _ := window["head_bytes"].(int); headBytes != 4 {
		t.Fatalf("expected head_bytes=4, got %#v", window)
	}
	if tailBytes, _ := window["tail_bytes"].(int); tailBytes != 6 {
		t.Fatalf("expected tail_bytes=6, got %#v", window)
	}
	if elidedBytes, _ := window["elided_bytes"].(int); elidedBytes != 10 {
		t.Fatalf("expected elided_bytes=10, got %#v", window)
	}
	if _, exists := window["next_offset"]; exists {
		t.Fatalf("did not expect head-tail preview metadata to expose next_offset, got %#v", window)
	}
}

func TestPipeExecutorReportsPrefixPreviewMetadataWhenTailCannotBePreserved(t *testing.T) {
	exec := shellexec.PipeExecutor{MaxOutputBytes: 5}

	result, err := exec.Execute(context.Background(), shellexec.Request{
		Command:   "printf '世界你好再见朋友'",
		TimeoutMS: 5000,
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	stdout, _ := result.Data["stdout"].(string)
	if stdout != "世" {
		t.Fatalf("expected UTF-8 fallback preview to degrade to prefix mode, got %q", stdout)
	}
	window, ok := result.Meta["stdout_preview"].(map[string]any)
	if !ok {
		t.Fatalf("expected stdout_preview metadata, got %#v", result.Meta)
	}
	if mode, _ := window["preview_mode"].(string); mode != "prefix" {
		t.Fatalf("expected prefix preview mode for fallback, got %#v", window)
	}
	if tailBytes, _ := window["tail_bytes"].(int); tailBytes != 0 {
		t.Fatalf("expected tail_bytes=0 for prefix fallback, got %#v", window)
	}
	if nextOffset, _ := window["next_offset"].(int64); nextOffset != int64(len(stdout)) {
		t.Fatalf("expected prefix fallback next_offset %d, got %#v", len(stdout), window["next_offset"])
	}
}

func TestPipeExecutorHeadTailPreviewRemainsUTF8Safe(t *testing.T) {
	exec := shellexec.PipeExecutor{MaxOutputBytes: 15}

	result, err := exec.Execute(context.Background(), shellexec.Request{
		Command:   "printf '世界你好再见朋友'",
		TimeoutMS: 5000,
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	stdout, _ := result.Data["stdout"].(string)
	if !utf8.ValidString(stdout) {
		t.Fatalf("expected UTF-8 safe preview, got %q", stdout)
	}
	if len(stdout) > 15 {
		t.Fatalf("expected preview to stay within byte limit %d, got %q (%d bytes)", 15, stdout, len(stdout))
	}
	if !strings.Contains(stdout, "...") {
		t.Fatalf("expected middle elision marker in UTF-8 preview, got %q", stdout)
	}
	if !strings.HasPrefix(stdout, "世") {
		t.Fatalf("expected UTF-8 preview head to preserve first rune, got %q", stdout)
	}
	if !strings.HasSuffix(stdout, "朋友") {
		t.Fatalf("expected UTF-8 preview tail to preserve last runes, got %q", stdout)
	}
}

func assertHeadTailPreview(t *testing.T, got string, limit int, head string, tail string) {
	t.Helper()
	if !strings.HasPrefix(got, head) {
		t.Fatalf("expected preview head %q, got %q", head, got)
	}
	if !strings.HasSuffix(got, tail) {
		t.Fatalf("expected preview tail %q, got %q", tail, got)
	}
	if !strings.Contains(got, "...") {
		t.Fatalf("expected middle truncation marker, got %q", got)
	}
	if len(got) > limit {
		t.Fatalf("expected preview to stay within byte limit %d, got %q (%d bytes)", limit, got, len(got))
	}
}
