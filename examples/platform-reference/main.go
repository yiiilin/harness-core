// Command platform-reference shows a small platform-side worker around claimed PTY execution.
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	shellmodule "github.com/yiiilin/harness-core/modules/shell"
	"github.com/yiiilin/harness-core/pkg/harness"
	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

const (
	exampleLeaseTTL      = 2 * time.Second
	exampleRenewInterval = 500 * time.Millisecond
	examplePTYInput      = "hello from platform reference\n"
)

type PlatformWorker struct {
	Runtime       *harness.Service
	LeaseTTL      time.Duration
	RenewInterval time.Duration
}

type WorkerRunResult struct {
	Claimed  harness.SessionState
	Run      hruntime.SessionRunOutput
	Released harness.SessionState
}

type DemoResult struct {
	Worker                    WorkerRunResult
	PersistedRuntimeHandle    execution.RuntimeHandle
	ClosedRuntimeHandle       execution.RuntimeHandle
	InteractiveRuntime        harness.ExecutionInteractiveRuntime
	ActiveVerify              verify.Result
	StreamVerify              verify.Result
	AttachOutput              string
	AttachDetached            bool
	StreamRead                shellmodule.PTYReadResult
	InteractiveHandleReleased bool
}

func main() {
	result, err := RunReferenceDemo(context.Background())
	if err != nil {
		panic(err)
	}

	fmt.Printf("session: %s\n", result.Worker.Run.Session.SessionID)
	fmt.Printf("phase: %s\n", result.Worker.Run.Session.Phase)
	fmt.Printf("runtime handle: %s (%s)\n", result.PersistedRuntimeHandle.HandleID, result.ClosedRuntimeHandle.Status)
	fmt.Printf("active verify: %v\n", result.ActiveVerify.Success)
	fmt.Printf("stream verify: %v\n", result.StreamVerify.Success)
	fmt.Printf("attach output: %s\n", strings.TrimSpace(result.AttachOutput))
	fmt.Printf("attach detached: %v\n", result.AttachDetached)
	fmt.Printf("lease released: %v\n", result.InteractiveHandleReleased)
}

// RunReferenceDemo seeds one PTY-backed session, runs it under a claimed worker, bridges I/O,
// verifies the PTY state, and reconciles the runtime handle lifecycle.
func RunReferenceDemo(ctx context.Context) (DemoResult, error) {
	manager := shellmodule.NewPTYManager(shellmodule.PTYManagerOptions{})
	defer func() {
		_ = manager.CloseAll(context.Background(), "example shutdown")
	}()

	rt := newPlatformRuntime(manager)
	worker := PlatformWorker{
		Runtime:       rt,
		LeaseTTL:      exampleLeaseTTL,
		RenewInterval: exampleRenewInterval,
	}

	sessionState, step, err := seedInteractiveSession(rt)
	if err != nil {
		return DemoResult{}, err
	}

	runResult, ok, err := worker.RunOnce(ctx)
	if err != nil {
		return DemoResult{}, err
	}
	if !ok {
		return DemoResult{}, errors.New("worker did not claim the seeded session")
	}
	if runResult.Claimed.SessionID != sessionState.SessionID {
		return DemoResult{}, fmt.Errorf("worker claimed unexpected session %s", runResult.Claimed.SessionID)
	}
	if len(runResult.Run.Executions) == 0 {
		return DemoResult{}, errors.New("worker produced no step executions")
	}
	if runResult.Run.Executions[0].Execution.Action.Data["mode"] != "pty" {
		return DemoResult{}, fmt.Errorf("expected PTY action result for step %s", step.StepID)
	}

	handles, err := rt.ListRuntimeHandles(sessionState.SessionID)
	if err != nil {
		return DemoResult{}, err
	}
	if len(handles) != 1 {
		return DemoResult{}, fmt.Errorf("expected one runtime handle, got %d", len(handles))
	}
	persistedHandle := handles[0]
	actionResult := runResult.Run.Executions[0].Execution.Action
	activeVerify, err := rt.EvaluateVerify(ctx, verify.Spec{
		Mode: verify.ModeAll,
		Checks: []verify.Check{
			{Kind: "pty_handle_active"},
		},
	}, actionResult, runResult.Run.Session)
	if err != nil {
		return DemoResult{}, err
	}
	output := &lockedBuffer{}
	attachment, err := manager.Attach(ctx, persistedHandle.HandleID, shellmodule.PTYAttachOptions{
		Input:        strings.NewReader(examplePTYInput),
		Output:       output,
		PollInterval: 10 * time.Millisecond,
		MaxBytes:     4096,
	})
	if err != nil {
		return DemoResult{}, err
	}
	if err := waitForBufferContains(ctx, output, "hello from platform reference"); err != nil {
		return DemoResult{}, err
	}
	streamVerify, err := rt.EvaluateVerify(ctx, verify.Spec{
		Mode: verify.ModeAll,
		Checks: []verify.Check{
			{Kind: "pty_stream_contains", Args: map[string]any{"text": "hello from platform reference", "timeout_ms": 1000}},
		},
	}, actionResult, runResult.Run.Session)
	if err != nil {
		return DemoResult{}, err
	}
	beforeDetach := output.String()
	if err := manager.Detach(attachment.AttachmentID); err != nil {
		return DemoResult{}, err
	}
	if _, err := manager.Write(ctx, persistedHandle.HandleID, "after-detach\n"); err != nil {
		return DemoResult{}, err
	}
	read, err := readUntilContains(ctx, manager, persistedHandle.HandleID, 0, "after-detach")
	if err != nil {
		return DemoResult{}, err
	}
	time.Sleep(150 * time.Millisecond)
	attachDetached := output.String() == beforeDetach

	if err := manager.Close(ctx, persistedHandle.HandleID, "platform example shutdown"); err != nil {
		return DemoResult{}, err
	}
	closedRead, err := waitForClosedPTY(ctx, manager, persistedHandle.HandleID)
	if err != nil {
		return DemoResult{}, err
	}
	mergedRead := mergeReadResults(read, closedRead)
	exitCode := mergedRead.ExitCode
	if _, err := rt.UpdateInteractiveRuntime(ctx, persistedHandle.HandleID, harness.InteractiveRuntimeUpdate{
		Observation: &harness.ExecutionInteractiveObservation{
			NextOffset:   mergedRead.NextOffset,
			Closed:       mergedRead.Closed,
			ExitCode:     &exitCode,
			Status:       mergedRead.Status,
			StatusReason: mergedRead.StatusReason,
		},
		LastOperation: &harness.ExecutionInteractiveOperation{
			Kind:   harness.ExecutionInteractiveOperationClose,
			At:     time.Now().UnixMilli(),
			Offset: mergedRead.NextOffset,
		},
	}); err != nil {
		return DemoResult{}, err
	}

	closedHandle, err := rt.CloseRuntimeHandle(ctx, persistedHandle.HandleID, harness.RuntimeHandleCloseRequest{
		Reason: "platform example shutdown",
		Metadata: map[string]any{
			"closed_by": "platform-reference",
		},
	})
	if err != nil {
		return DemoResult{}, err
	}
	interactiveRuntime, err := rt.GetInteractiveRuntime(persistedHandle.HandleID)
	if err != nil {
		return DemoResult{}, err
	}

	return DemoResult{
		Worker:                    runResult,
		PersistedRuntimeHandle:    persistedHandle,
		ClosedRuntimeHandle:       closedHandle,
		InteractiveRuntime:        interactiveRuntime,
		ActiveVerify:              activeVerify,
		StreamVerify:              streamVerify,
		AttachOutput:              beforeDetach,
		AttachDetached:            attachDetached,
		StreamRead:                mergedRead,
		InteractiveHandleReleased: runResult.Released.LeaseID == "" && runResult.Released.LeaseExpiresAt == 0,
	}, nil
}

func newPlatformRuntime(manager *shellmodule.PTYManager) *harness.Service {
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	shellmodule.RegisterWithOptions(tools, verifiers, shellmodule.Options{PTYManager: manager})

	opts := harness.Options{
		Tools:     tools,
		Verifiers: verifiers,
		Policy: permission.RulesEvaluator{
			Rules: []permission.Rule{
				{Permission: "shell.exec", Pattern: "*", Action: permission.Allow},
			},
			Fallback: permission.DefaultEvaluator{},
		},
	}
	return harness.New(opts)
}

// seedInteractiveSession creates one session whose only step launches a PTY-backed cat process.
func seedInteractiveSession(rt *harness.Service) (harness.SessionState, plan.StepSpec, error) {
	sess, err := rt.CreateSession("platform-reference", "run a claimed PTY-backed shell step")
	if err != nil {
		return harness.SessionState{}, plan.StepSpec{}, err
	}
	tsk, err := rt.CreateTask(task.Spec{
		TaskType: "demo",
		Goal:     "start a PTY shell and interact through the platform layer",
	})
	if err != nil {
		return harness.SessionState{}, plan.StepSpec{}, err
	}
	sess, err = rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		return harness.SessionState{}, plan.StepSpec{}, err
	}

	step := plan.StepSpec{
		StepID: "step_platform_reference_pty",
		Title:  "start PTY cat session",
		Action: action.Spec{
			ToolName: "shell.exec",
			Args: map[string]any{
				"mode":    "pty",
				"command": "cat",
			},
		},
	}
	if _, err := rt.CreatePlan(sess.SessionID, "platform reference PTY plan", []plan.StepSpec{step}); err != nil {
		return harness.SessionState{}, plan.StepSpec{}, err
	}
	return sess, step, nil
}

// RunOnce is the minimal claimed worker loop: claim, renew lease, run, then release.
func (w PlatformWorker) RunOnce(ctx context.Context) (WorkerRunResult, bool, error) {
	if w.Runtime == nil {
		return WorkerRunResult{}, false, errors.New("runtime is required")
	}
	if w.LeaseTTL <= 0 {
		w.LeaseTTL = exampleLeaseTTL
	}
	if w.RenewInterval <= 0 {
		w.RenewInterval = exampleRenewInterval
	}

	claimed, ok, err := w.Runtime.ClaimRunnableSession(ctx, w.LeaseTTL)
	if err != nil || !ok {
		return WorkerRunResult{}, ok, err
	}

	stopRenew := make(chan struct{})
	renewErr := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(w.RenewInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stopRenew:
				return
			case <-ticker.C:
				if _, err := w.Runtime.RenewSessionLease(ctx, claimed.SessionID, claimed.LeaseID, w.LeaseTTL); err != nil {
					renewErr <- err
					return
				}
			}
		}
	}()

	run, runErr := w.Runtime.RunClaimedSession(ctx, claimed.SessionID, claimed.LeaseID)
	close(stopRenew)
	select {
	case err := <-renewErr:
		if err != nil && runErr == nil {
			runErr = err
		}
	default:
	}

	released, releaseErr := w.Runtime.ReleaseSessionLease(ctx, claimed.SessionID, claimed.LeaseID)
	if runErr != nil {
		return WorkerRunResult{Claimed: claimed, Run: run, Released: released}, true, runErr
	}
	if releaseErr != nil {
		return WorkerRunResult{Claimed: claimed, Run: run, Released: released}, true, releaseErr
	}

	return WorkerRunResult{
		Claimed:  claimed,
		Run:      run,
		Released: released,
	}, true, nil
}

// readUntilContains polls the PTY stream directly to prove the process stays alive after detach.
func readUntilContains(ctx context.Context, manager *shellmodule.PTYManager, handleID string, offset int64, needle string) (shellmodule.PTYReadResult, error) {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		read, err := manager.Read(ctx, handleID, shellmodule.PTYReadRequest{
			Offset:   offset,
			MaxBytes: 4096,
		})
		if err == nil && strings.Contains(read.Data, needle) {
			return read, nil
		}
		time.Sleep(25 * time.Millisecond)
	}
	return shellmodule.PTYReadResult{}, fmt.Errorf("timed out waiting for PTY output containing %q", needle)
}

// waitForClosedPTY waits until the PTY manager reports the session as closed.
func waitForClosedPTY(ctx context.Context, manager *shellmodule.PTYManager, handleID string) (shellmodule.PTYReadResult, error) {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		read, err := manager.Read(ctx, handleID, shellmodule.PTYReadRequest{MaxBytes: 4096})
		if err == nil && read.Closed {
			return read, nil
		}
		time.Sleep(25 * time.Millisecond)
	}
	return shellmodule.PTYReadResult{}, fmt.Errorf("timed out waiting for PTY session %s to close", handleID)
}

func mergeReadResults(active, closed shellmodule.PTYReadResult) shellmodule.PTYReadResult {
	out := active
	if closed.HandleID != "" {
		out.HandleID = closed.HandleID
	}
	if closed.NextOffset > out.NextOffset {
		out.NextOffset = closed.NextOffset
	}
	out.Closed = closed.Closed
	if closed.ExitCode != 0 {
		out.ExitCode = closed.ExitCode
	}
	if closed.Status != "" {
		out.Status = closed.Status
	}
	if closed.StatusReason != "" {
		out.StatusReason = closed.StatusReason
	}
	return out
}

// waitForBufferContains confirms that Attach bridged PTY output into an external writer.
func waitForBufferContains(ctx context.Context, buf *lockedBuffer, needle string) error {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), needle) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
	return fmt.Errorf("timed out waiting for attached output containing %q", needle)
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
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
