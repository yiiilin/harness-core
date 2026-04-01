package shellmodule_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	shellmodule "github.com/yiiilin/harness-core/modules/shell"
	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

func TestShellPTYModeReturnsRuntimeHandleAndStreamMetadata(t *testing.T) {
	ctx := context.Background()
	manager := shellmodule.NewPTYManager(shellmodule.PTYManagerOptions{})
	t.Cleanup(func() {
		_ = manager.CloseAll(ctx, "test cleanup")
	})

	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	shellmodule.RegisterWithOptions(tools, verifiers, shellmodule.Options{PTYManager: manager})

	result, err := tools.Invoke(ctx, action.Spec{
		ToolName: "shell.exec",
		Args: map[string]any{
			"mode":    "pty",
			"command": "cat",
		},
	})
	if err != nil {
		t.Fatalf("invoke pty shell: %v", err)
	}
	if !result.OK {
		t.Fatalf("expected successful PTY start, got %#v", result)
	}

	handle, ok := result.Data["runtime_handle"].(execution.RuntimeHandle)
	if !ok {
		t.Fatalf("expected typed runtime handle, got %#v", result.Data["runtime_handle"])
	}
	if handle.HandleID == "" || handle.Kind != "pty" {
		t.Fatalf("unexpected runtime handle: %#v", handle)
	}

	stream, ok := result.Data["shell_stream"].(shellmodule.PTYStreamInfo)
	if !ok {
		t.Fatalf("expected PTY stream info, got %#v", result.Data["shell_stream"])
	}
	if stream.HandleID != handle.HandleID || stream.NextOffset != 0 || stream.Status != "active" {
		t.Fatalf("unexpected stream info: %#v", stream)
	}

	if _, err := manager.Write(ctx, handle.HandleID, "hello from pty\n"); err != nil {
		t.Fatalf("write pty: %v", err)
	}

	read := readPTYOutputEventually(t, manager, handle.HandleID, 0, "hello from pty")
	if !strings.Contains(read.Data, "hello from pty") {
		t.Fatalf("expected PTY output to contain echoed input, got %#v", read)
	}

	if err := manager.Close(ctx, handle.HandleID, "test done"); err != nil {
		t.Fatalf("close pty: %v", err)
	}
}

func TestPTYManagerInteractiveControllerDrivesKernelInteractiveLifecycle(t *testing.T) {
	ctx := context.Background()
	manager := shellmodule.NewPTYManager(shellmodule.PTYManagerOptions{})
	t.Cleanup(func() {
		_ = manager.CloseAll(ctx, "test cleanup")
	})

	rt := hruntime.New(hruntime.Options{
		InteractiveController: shellmodule.NewInteractiveController(manager),
	})
	sess, err := rt.CreateSession("interactive shell controller", "exercise PTY controller through core runtime")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	started, err := rt.StartInteractive(ctx, sess.SessionID, hruntime.InteractiveStartRequest{
		Kind: "pty",
		Spec: map[string]any{
			"command": "cat",
		},
	})
	if err != nil {
		t.Fatalf("start interactive: %v", err)
	}
	if started.Handle.HandleID == "" || started.Handle.Value == "" || !started.Capabilities.View || !started.Capabilities.Write || !started.Capabilities.Close {
		t.Fatalf("unexpected started interactive runtime: %#v", started)
	}

	written, err := rt.WriteInteractive(ctx, started.Handle.HandleID, hruntime.InteractiveWriteRequest{Input: "from-controller\n"})
	if err != nil {
		t.Fatalf("write interactive: %v", err)
	}
	if written.Bytes <= 0 || written.Runtime.LastOperation.Kind != execution.InteractiveOperationWrite {
		t.Fatalf("unexpected write result: %#v", written)
	}

	viewed := waitForInteractiveViewContains(t, rt, started.Handle.HandleID, "from-controller")
	if viewed.Runtime.LastOperation.Kind != execution.InteractiveOperationView {
		t.Fatalf("expected view operation metadata, got %#v", viewed)
	}

	reopened, err := rt.ReopenInteractive(ctx, started.Handle.HandleID, hruntime.InteractiveReopenRequest{})
	if err != nil {
		t.Fatalf("reopen interactive: %v", err)
	}
	if reopened.LastOperation.Kind != execution.InteractiveOperationReopen {
		t.Fatalf("expected reopen metadata, got %#v", reopened)
	}

	closed, err := rt.CloseInteractive(ctx, started.Handle.HandleID, hruntime.InteractiveCloseRequest{Reason: "done"})
	if err != nil {
		t.Fatalf("close interactive: %v", err)
	}
	if !closed.Observation.Closed || closed.Observation.Status != "closed" || closed.LastOperation.Kind != execution.InteractiveOperationClose {
		t.Fatalf("unexpected close result: %#v", closed)
	}
}

func TestInteractiveViewExposesRecoverablePreviewMetadata(t *testing.T) {
	ctx := context.Background()
	manager := shellmodule.NewPTYManager(shellmodule.PTYManagerOptions{})
	t.Cleanup(func() {
		_ = manager.CloseAll(ctx, "test cleanup")
	})

	rt := hruntime.New(hruntime.Options{
		InteractiveController: shellmodule.NewInteractiveController(manager),
	})
	sess, err := rt.CreateSession("interactive preview metadata", "expose recoverable preview metadata for PTY reads")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	started, err := rt.StartInteractive(ctx, sess.SessionID, hruntime.InteractiveStartRequest{
		Kind: "pty",
		Spec: map[string]any{
			"command": "printf 'hello world'; sleep 2",
		},
	})
	if err != nil {
		t.Fatalf("start interactive: %v", err)
	}

	_ = waitForInteractiveViewContains(t, rt, started.Handle.HandleID, "hello world")

	viewed, err := rt.ViewInteractive(ctx, started.Handle.HandleID, hruntime.InteractiveViewRequest{
		Offset:   0,
		MaxBytes: 5,
	})
	if err != nil {
		t.Fatalf("view interactive with preview budget: %v", err)
	}
	if viewed.Data != "hello" {
		t.Fatalf("expected preview data to be bounded, got %#v", viewed)
	}
	if viewed.Window == nil || !viewed.Window.Truncated {
		t.Fatalf("expected truncated preview metadata, got %#v", viewed)
	}
	if viewed.Window.OriginalBytes < len("hello world") {
		t.Fatalf("expected original_bytes to reflect full PTY buffer, got %#v", viewed)
	}
	if viewed.Window.ReturnedBytes != len("hello") {
		t.Fatalf("expected returned_bytes %d, got %#v", len("hello"), viewed)
	}
	if !viewed.Window.HasMore {
		t.Fatalf("expected has_more metadata, got %#v", viewed)
	}
	if viewed.Window.NextOffset != int64(len("hello")) {
		t.Fatalf("expected next_offset %d, got %#v", len("hello"), viewed)
	}
	if viewed.RawHandle == nil || viewed.RawHandle.Ref != started.Handle.HandleID {
		t.Fatalf("expected raw_ref to reuse the PTY handle id %q, got %#v", started.Handle.HandleID, viewed)
	}
}

func TestPTYManagerReadWriteAndCloseLifecycle(t *testing.T) {
	ctx := context.Background()
	manager := shellmodule.NewPTYManager(shellmodule.PTYManagerOptions{})
	t.Cleanup(func() {
		_ = manager.CloseAll(ctx, "test cleanup")
	})

	started, err := manager.Start(ctx, "cat", shellmodule.PTYStartOptions{})
	if err != nil {
		t.Fatalf("start pty session: %v", err)
	}
	if started.RuntimeHandle.HandleID == "" {
		t.Fatalf("expected non-empty runtime handle from start: %#v", started)
	}

	if _, err := manager.Write(ctx, started.RuntimeHandle.HandleID, "ping\n"); err != nil {
		t.Fatalf("write pty: %v", err)
	}

	read := readPTYOutputEventually(t, manager, started.RuntimeHandle.HandleID, 0, "ping")
	if read.Window == nil || read.Window.NextOffset <= 0 {
		t.Fatalf("expected positive next offset after reading PTY output, got %#v", read)
	}
	if read.Closed {
		t.Fatalf("expected PTY session to remain active before close, got %#v", read)
	}

	if err := manager.Close(ctx, started.RuntimeHandle.HandleID, "operator stop"); err != nil {
		t.Fatalf("close pty: %v", err)
	}

	closed := waitForPTYClosed(t, manager, started.RuntimeHandle.HandleID)
	if !closed.Closed || closed.StatusReason != "operator stop" {
		t.Fatalf("expected closed PTY read result after close, got %#v", closed)
	}
}

func waitForInteractiveViewContains(t *testing.T, rt *hruntime.Service, handleID, text string) hruntime.InteractiveViewResult {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		viewed, err := rt.ViewInteractive(context.Background(), handleID, hruntime.InteractiveViewRequest{
			Offset:   0,
			MaxBytes: 4096,
		})
		if err != nil {
			t.Fatalf("view interactive: %v", err)
		}
		if strings.Contains(viewed.Data, text) {
			return viewed
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("interactive view never contained %q", text)
	return hruntime.InteractiveViewResult{}
}

func TestShellPTYVerifiers(t *testing.T) {
	ctx := context.Background()
	manager := shellmodule.NewPTYManager(shellmodule.PTYManagerOptions{})
	t.Cleanup(func() {
		_ = manager.CloseAll(ctx, "test cleanup")
	})

	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	shellmodule.RegisterWithOptions(tools, verifiers, shellmodule.Options{PTYManager: manager})

	activeResult, err := tools.Invoke(ctx, action.Spec{
		ToolName: "shell.exec",
		Args: map[string]any{
			"mode":    "pty",
			"command": "cat",
		},
	})
	if err != nil {
		t.Fatalf("invoke pty shell: %v", err)
	}

	activeVerify, err := verifiers.Evaluate(ctx, verify.Spec{
		Mode: verify.ModeAll,
		Checks: []verify.Check{
			{Kind: "pty_handle_active"},
		},
	}, activeResult, session.State{})
	if err != nil {
		t.Fatalf("evaluate pty_handle_active: %v", err)
	}
	if !activeVerify.Success {
		t.Fatalf("expected pty_handle_active to succeed, got %#v", activeVerify)
	}

	handle, ok := activeResult.Data["runtime_handle"].(execution.RuntimeHandle)
	if !ok {
		t.Fatalf("expected typed runtime handle, got %#v", activeResult.Data["runtime_handle"])
	}
	if _, err := manager.Write(ctx, handle.HandleID, "verifier-echo\n"); err != nil {
		t.Fatalf("write pty for stream verifier: %v", err)
	}

	streamVerify, err := verifiers.Evaluate(ctx, verify.Spec{
		Mode: verify.ModeAll,
		Checks: []verify.Check{
			{Kind: "pty_stream_contains", Args: map[string]any{"text": "verifier-echo", "timeout_ms": 1500}},
		},
	}, activeResult, session.State{})
	if err != nil {
		t.Fatalf("evaluate pty_stream_contains: %v", err)
	}
	if !streamVerify.Success {
		t.Fatalf("expected pty_stream_contains to succeed, got %#v", streamVerify)
	}

	exitResult, err := tools.Invoke(ctx, action.Spec{
		ToolName: "shell.exec",
		Args: map[string]any{
			"mode":    "pty",
			"command": "printf verifier-exit; exit 7",
		},
	})
	if err != nil {
		t.Fatalf("invoke exit pty shell: %v", err)
	}

	exitVerify, err := verifiers.Evaluate(ctx, verify.Spec{
		Mode: verify.ModeAll,
		Checks: []verify.Check{
			{Kind: "pty_exit_code", Args: map[string]any{"allowed": []any{7}, "timeout_ms": 1500}},
		},
	}, exitResult, session.State{})
	if err != nil {
		t.Fatalf("evaluate pty_exit_code: %v", err)
	}
	if !exitVerify.Success {
		t.Fatalf("expected pty_exit_code to succeed, got %#v", exitVerify)
	}
}

func TestPTYManagerAttachStreamsExternalInputAndOutput(t *testing.T) {
	ctx := context.Background()
	manager := shellmodule.NewPTYManager(shellmodule.PTYManagerOptions{})
	t.Cleanup(func() {
		_ = manager.CloseAll(ctx, "test cleanup")
	})

	started, err := manager.Start(ctx, "cat", shellmodule.PTYStartOptions{})
	if err != nil {
		t.Fatalf("start pty session: %v", err)
	}

	output := &lockedBuffer{}
	attachment, err := manager.Attach(ctx, started.RuntimeHandle.HandleID, shellmodule.PTYAttachOptions{
		Input:        strings.NewReader("attached-bridge\n"),
		Output:       output,
		PollInterval: 10 * time.Millisecond,
		MaxBytes:     4096,
	})
	if err != nil {
		t.Fatalf("attach pty: %v", err)
	}
	if attachment.AttachmentID == "" {
		t.Fatalf("expected attachment id, got %#v", attachment)
	}

	waitForBufferContains(t, output, "attached-bridge")

	if err := manager.Detach(attachment.AttachmentID); err != nil {
		t.Fatalf("detach pty: %v", err)
	}

	beforeDetach := output.String()
	if _, err := manager.Write(ctx, started.RuntimeHandle.HandleID, "after-detach\n"); err != nil {
		t.Fatalf("write after detach: %v", err)
	}
	waitForPTYOutputEventually(t, manager, started.RuntimeHandle.HandleID, 0, "after-detach")
	time.Sleep(150 * time.Millisecond)
	if output.String() != beforeDetach {
		t.Fatalf("expected detached output bridge to stop receiving bytes, got before=%q after=%q", beforeDetach, output.String())
	}
}

func TestPTYManagerDetachDoesNotCloseUnderlyingSession(t *testing.T) {
	ctx := context.Background()
	manager := shellmodule.NewPTYManager(shellmodule.PTYManagerOptions{})
	t.Cleanup(func() {
		_ = manager.CloseAll(ctx, "test cleanup")
	})

	started, err := manager.Start(ctx, "cat", shellmodule.PTYStartOptions{})
	if err != nil {
		t.Fatalf("start pty session: %v", err)
	}

	output := &lockedBuffer{}
	attachment, err := manager.Attach(ctx, started.RuntimeHandle.HandleID, shellmodule.PTYAttachOptions{
		Output:       output,
		PollInterval: 10 * time.Millisecond,
		MaxBytes:     4096,
	})
	if err != nil {
		t.Fatalf("attach output bridge: %v", err)
	}

	if err := manager.Detach(attachment.AttachmentID); err != nil {
		t.Fatalf("detach output bridge: %v", err)
	}

	if _, err := manager.Write(ctx, started.RuntimeHandle.HandleID, "session-still-alive\n"); err != nil {
		t.Fatalf("write after detach: %v", err)
	}
	read := waitForPTYOutputEventually(t, manager, started.RuntimeHandle.HandleID, 0, "session-still-alive")
	if read.Closed {
		t.Fatalf("expected PTY session to remain active after detach, got %#v", read)
	}
}

func TestShellPTYVerifiersSupportExplicitInspector(t *testing.T) {
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	shellmodule.RegisterWithOptions(tools, verifiers, shellmodule.Options{
		PTYBackend: &stubPTYBackend{},
		PTYInspector: fakePTYInspector{
			inspect: map[string]shellmodule.PTYInspectResult{
				"hdl-active": {Status: "active", Closed: false},
				"hdl-exit":   {Status: "closed", Closed: true, ExitCode: 7},
			},
			read: map[string]shellmodule.PTYReadResult{
				"hdl-active": {Status: "active", Data: "remote verifier output", Window: &shellmodule.ResultWindow{NextOffset: 22}},
			},
		},
	})

	activeVerify, err := verifiers.Evaluate(context.Background(), verify.Spec{
		Mode: verify.ModeAll,
		Checks: []verify.Check{
			{Kind: "pty_handle_active"},
		},
	}, action.Result{
		OK: true,
		Data: map[string]any{
			"runtime_handle": map[string]any{"handle_id": "hdl-active"},
		},
	}, session.State{})
	if err != nil {
		t.Fatalf("evaluate pty_handle_active via explicit inspector: %v", err)
	}
	if !activeVerify.Success {
		t.Fatalf("expected active verifier to succeed via explicit inspector, got %#v", activeVerify)
	}

	streamVerify, err := verifiers.Evaluate(context.Background(), verify.Spec{
		Mode: verify.ModeAll,
		Checks: []verify.Check{
			{Kind: "pty_stream_contains", Args: map[string]any{"text": "verifier output", "timeout_ms": 50}},
		},
	}, action.Result{
		OK: true,
		Data: map[string]any{
			"runtime_handle": map[string]any{"handle_id": "hdl-active"},
		},
	}, session.State{})
	if err != nil {
		t.Fatalf("evaluate pty_stream_contains via explicit inspector: %v", err)
	}
	if !streamVerify.Success {
		t.Fatalf("expected stream verifier to succeed via explicit inspector, got %#v", streamVerify)
	}

	exitVerify, err := verifiers.Evaluate(context.Background(), verify.Spec{
		Mode: verify.ModeAll,
		Checks: []verify.Check{
			{Kind: "pty_exit_code", Args: map[string]any{"allowed": []any{7}, "timeout_ms": 50}},
		},
	}, action.Result{
		OK: true,
		Data: map[string]any{
			"runtime_handle": map[string]any{"handle_id": "hdl-exit"},
		},
	}, session.State{})
	if err != nil {
		t.Fatalf("evaluate pty_exit_code via explicit inspector: %v", err)
	}
	if !exitVerify.Success {
		t.Fatalf("expected exit verifier to succeed via explicit inspector, got %#v", exitVerify)
	}
}

func TestPTYHandleActiveVerifierUsesCallerContext(t *testing.T) {
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	shellmodule.RegisterWithOptions(tools, verifiers, shellmodule.Options{
		PTYBackend:   &stubPTYBackend{},
		PTYInspector: contextAwarePTYInspector{},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := verifiers.Evaluate(ctx, verify.Spec{
		Mode: verify.ModeAll,
		Checks: []verify.Check{
			{Kind: "pty_handle_active"},
		},
	}, action.Result{
		OK: true,
		Data: map[string]any{
			"runtime_handle": map[string]any{"handle_id": "hdl-cancelled"},
		},
	}, session.State{})
	if err != nil {
		t.Fatalf("evaluate pty_handle_active with canceled context: %v", err)
	}
	if result.Success || !strings.Contains(result.Reason, context.Canceled.Error()) {
		t.Fatalf("expected canceled context to reach inspector, got %#v", result)
	}
}

func TestPTYStreamVerifierSupportsRuntimeHandlesSliceAndShellStreamOffset(t *testing.T) {
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	inspector := &recordingPTYInspector{
		fakePTYInspector: fakePTYInspector{
			inspect: map[string]shellmodule.PTYInspectResult{
				"hdl-slice": {Status: "active"},
			},
			read: map[string]shellmodule.PTYReadResult{
				"hdl-slice": {Status: "active", Data: "remote verifier output", Window: &shellmodule.ResultWindow{NextOffset: 41}},
			},
		},
	}
	shellmodule.RegisterWithOptions(tools, verifiers, shellmodule.Options{
		PTYBackend:   &stubPTYBackend{},
		PTYInspector: inspector,
	})

	result, err := verifiers.Evaluate(context.Background(), verify.Spec{
		Mode: verify.ModeAll,
		Checks: []verify.Check{
			{Kind: "pty_stream_contains", Args: map[string]any{"text": "verifier output", "timeout_ms": 50}},
		},
	}, action.Result{
		OK: true,
		Data: map[string]any{
			"runtime_handles": []map[string]any{
				{"handle_id": "hdl-slice"},
			},
			"shell_stream": map[string]any{
				"handle_id":   "hdl-slice",
				"next_offset": 19,
				"status":      "active",
			},
		},
	}, session.State{})
	if err != nil {
		t.Fatalf("evaluate pty_stream_contains with runtime_handles slice: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected stream verifier to succeed from runtime_handles slice, got %#v", result)
	}
	if len(inspector.readRequests) != 1 || inspector.readRequests[0].Offset != 19 {
		t.Fatalf("expected verifier to start from shell_stream next_offset, got %#v", inspector.readRequests)
	}
}

func readPTYOutputEventually(t *testing.T, manager *shellmodule.PTYManager, handleID string, offset int64, needle string) shellmodule.PTYReadResult {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		read, err := manager.Read(context.Background(), handleID, shellmodule.PTYReadRequest{
			Offset:   offset,
			MaxBytes: 4096,
		})
		if err == nil && strings.Contains(read.Data, needle) {
			return read
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for PTY output containing %q", needle)
	return shellmodule.PTYReadResult{}
}

func waitForPTYOutputEventually(t *testing.T, manager *shellmodule.PTYManager, handleID string, offset int64, needle string) shellmodule.PTYReadResult {
	t.Helper()
	return readPTYOutputEventually(t, manager, handleID, offset, needle)
}

func waitForPTYClosed(t *testing.T, manager *shellmodule.PTYManager, handleID string) shellmodule.PTYReadResult {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		read, err := manager.Read(context.Background(), handleID, shellmodule.PTYReadRequest{MaxBytes: 4096})
		if err == nil && read.Closed {
			return read
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for PTY session %s to close", handleID)
	return shellmodule.PTYReadResult{}
}

func waitForBufferContains(t *testing.T, buf *lockedBuffer, needle string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), needle) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for attached output to contain %q; got %q", needle, buf.String())
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

type fakePTYInspector struct {
	inspect map[string]shellmodule.PTYInspectResult
	read    map[string]shellmodule.PTYReadResult
}

type recordingPTYInspector struct {
	fakePTYInspector
	readRequests []shellmodule.PTYReadRequest
}

type contextAwarePTYInspector struct{}

func (f fakePTYInspector) Inspect(_ context.Context, handleID string) (shellmodule.PTYInspectResult, error) {
	result, ok := f.inspect[handleID]
	if !ok {
		return shellmodule.PTYInspectResult{}, shellmodule.ErrPTYSessionNotFound
	}
	result.HandleID = handleID
	return result, nil
}

func (f fakePTYInspector) Read(_ context.Context, handleID string, _ shellmodule.PTYReadRequest) (shellmodule.PTYReadResult, error) {
	result, ok := f.read[handleID]
	if !ok {
		return shellmodule.PTYReadResult{}, shellmodule.ErrPTYSessionNotFound
	}
	result.HandleID = handleID
	return result, nil
}

func (r *recordingPTYInspector) Inspect(ctx context.Context, handleID string) (shellmodule.PTYInspectResult, error) {
	return r.fakePTYInspector.Inspect(ctx, handleID)
}

func (r *recordingPTYInspector) Read(ctx context.Context, handleID string, req shellmodule.PTYReadRequest) (shellmodule.PTYReadResult, error) {
	r.readRequests = append(r.readRequests, req)
	return r.fakePTYInspector.Read(ctx, handleID, req)
}

func (contextAwarePTYInspector) Inspect(ctx context.Context, handleID string) (shellmodule.PTYInspectResult, error) {
	select {
	case <-ctx.Done():
		return shellmodule.PTYInspectResult{}, ctx.Err()
	default:
		return shellmodule.PTYInspectResult{HandleID: handleID, Status: "active"}, nil
	}
}

func (contextAwarePTYInspector) Read(ctx context.Context, handleID string, _ shellmodule.PTYReadRequest) (shellmodule.PTYReadResult, error) {
	select {
	case <-ctx.Done():
		return shellmodule.PTYReadResult{}, ctx.Err()
	default:
		return shellmodule.PTYReadResult{HandleID: handleID, Status: "active"}, nil
	}
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *lockedBuffer) Read(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.buf.Len() == 0 {
		return 0, io.EOF
	}
	return b.buf.Read(p)
}
