package runtime_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/execution"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
)

type stubInteractiveController struct {
	startCalls   int
	reopenCalls  int
	viewCalls    int
	writeCalls   int
	closeCalls   int
	viewClosed   bool
	lastStart    hruntime.InteractiveStartRequest
	lastReopen   hruntime.InteractiveReopenRequest
	lastView     hruntime.InteractiveViewRequest
	lastWrite    hruntime.InteractiveWriteRequest
	lastClose    hruntime.InteractiveCloseRequest
	lastHandleID string
}

func (s *stubInteractiveController) StartInteractive(_ context.Context, request hruntime.InteractiveStartRequest) (hruntime.InteractiveStartResult, error) {
	s.startCalls++
	s.lastStart = request
	return hruntime.InteractiveStartResult{
		Kind:  firstNonEmpty(request.Kind, "stub"),
		Value: "backend-started",
		Capabilities: execution.InteractiveCapabilities{
			Reopen: true,
			View:   true,
			Write:  true,
			Close:  true,
		},
		Observation: execution.InteractiveObservation{
			NextOffset:   0,
			Status:       "active",
			StatusReason: "stub ready",
		},
		Metadata: map[string]any{"backend": "stub"},
	}, nil
}

func (s *stubInteractiveController) ReopenInteractive(_ context.Context, handle execution.RuntimeHandle, request hruntime.InteractiveReopenRequest) (hruntime.InteractiveReopenResult, error) {
	s.reopenCalls++
	s.lastHandleID = handle.HandleID
	s.lastReopen = request
	return hruntime.InteractiveReopenResult{
		Observation: &execution.InteractiveObservation{
			NextOffset:   7,
			Status:       "active",
			StatusReason: "reopened",
		},
		Metadata: map[string]any{"reopened": true},
	}, nil
}

func (s *stubInteractiveController) ViewInteractive(_ context.Context, handle execution.RuntimeHandle, request hruntime.InteractiveViewRequest) (hruntime.InteractiveViewResult, error) {
	s.viewCalls++
	s.lastHandleID = handle.HandleID
	s.lastView = request
	observation := execution.InteractiveObservation{
		NextOffset:   5,
		Status:       "active",
		StatusReason: "viewed",
	}
	if s.viewClosed {
		exitCode := 0
		observation.Closed = true
		observation.ExitCode = &exitCode
		observation.Status = "closed"
		observation.StatusReason = "remote session exited"
	}
	return hruntime.InteractiveViewResult{
		Data: "hello",
		Runtime: execution.InteractiveRuntime{
			Handle:      handle,
			Observation: observation,
		},
	}, nil
}

func (s *stubInteractiveController) WriteInteractive(_ context.Context, handle execution.RuntimeHandle, request hruntime.InteractiveWriteRequest) (hruntime.InteractiveWriteResult, error) {
	s.writeCalls++
	s.lastHandleID = handle.HandleID
	s.lastWrite = request
	return hruntime.InteractiveWriteResult{
		Bytes: int64(len(request.Input)),
		Runtime: execution.InteractiveRuntime{
			Handle: handle,
			Observation: execution.InteractiveObservation{
				NextOffset:   5,
				Status:       "active",
				StatusReason: "written",
			},
		},
	}, nil
}

func (s *stubInteractiveController) CloseInteractive(_ context.Context, handle execution.RuntimeHandle, request hruntime.InteractiveCloseRequest) (hruntime.InteractiveCloseResult, error) {
	s.closeCalls++
	s.lastHandleID = handle.HandleID
	s.lastClose = request
	exitCode := 0
	return hruntime.InteractiveCloseResult{
		Runtime: execution.InteractiveRuntime{
			Handle: handle,
			Observation: execution.InteractiveObservation{
				Closed:       true,
				ExitCode:     &exitCode,
				Status:       "closed",
				StatusReason: firstNonEmpty(request.Reason, "closed"),
			},
		},
	}, nil
}

func TestInteractiveRuntimeReadAndUpdateSurface(t *testing.T) {
	rt := hruntime.New(hruntime.Options{})
	sess := mustCreateSession(t, rt, "interactive runtime", "project typed interactive runtime state")

	_, err := rt.RuntimeHandles.Create(execution.RuntimeHandle{
		HandleID:  "hdl_interactive_runtime",
		SessionID: sess.SessionID,
		Status:    execution.RuntimeHandleActive,
		Metadata: map[string]any{
			execution.InteractiveMetadataKeyEnabled:       true,
			execution.InteractiveMetadataKeySupportsView:  true,
			execution.InteractiveMetadataKeySupportsWrite: true,
			execution.InteractiveMetadataKeySupportsClose: true,
			execution.InteractiveMetadataKeyStatus:        "active",
			execution.InteractiveMetadataKeyNextOffset:    int64(0),
		},
	})
	if err != nil {
		t.Fatalf("seed interactive runtime handle: %v", err)
	}

	projected, err := rt.GetInteractiveRuntime("hdl_interactive_runtime")
	if err != nil {
		t.Fatalf("get interactive runtime: %v", err)
	}
	if !projected.Capabilities.View || !projected.Capabilities.Write || projected.Observation.NextOffset != 0 {
		t.Fatalf("unexpected projected interactive runtime: %#v", projected)
	}

	exitCode := 0
	updated, err := rt.UpdateInteractiveRuntime(context.Background(), "hdl_interactive_runtime", hruntime.InteractiveRuntimeUpdate{
		Observation: &execution.InteractiveObservation{
			NextOffset:   99,
			Closed:       true,
			ExitCode:     &exitCode,
			Status:       "closed",
			StatusReason: "remote process exited",
			Snapshot: execution.InteractiveSnapshot{
				ArtifactID: "art_snapshot_updated",
			},
		},
		LastOperation: &execution.InteractiveOperation{
			Kind:   execution.InteractiveOperationView,
			At:     5000,
			Offset: 88,
		},
	})
	if err != nil {
		t.Fatalf("update interactive runtime: %v", err)
	}
	if updated.Observation.NextOffset != 99 || !updated.Observation.Closed || updated.Observation.ExitCode == nil || *updated.Observation.ExitCode != 0 {
		t.Fatalf("unexpected updated observation: %#v", updated)
	}
	if updated.Observation.Snapshot.ArtifactID != "art_snapshot_updated" {
		t.Fatalf("expected snapshot artifact to persist, got %#v", updated.Observation.Snapshot)
	}
	if updated.LastOperation.Kind != execution.InteractiveOperationView || updated.LastOperation.Offset != 88 {
		t.Fatalf("expected last operation metadata, got %#v", updated.LastOperation)
	}

	all, err := rt.ListInteractiveRuntimes(sess.SessionID)
	if err != nil {
		t.Fatalf("list interactive runtimes: %v", err)
	}
	if len(all) != 1 || all[0].Handle.HandleID != "hdl_interactive_runtime" {
		t.Fatalf("expected one interactive runtime projection, got %#v", all)
	}
}

func TestInteractiveControlPlaneLifecyclePersistsRuntimeState(t *testing.T) {
	controller := &stubInteractiveController{}
	rt := hruntime.New(hruntime.Options{InteractiveController: controller})
	sess := mustCreateSession(t, rt, "interactive control", "exercise kernel interactive lifecycle")

	started, err := rt.StartInteractive(context.Background(), sess.SessionID, hruntime.InteractiveStartRequest{
		Kind:     "stub",
		Spec:     map[string]any{"command": "demo"},
		Metadata: map[string]any{"origin": "test"},
	})
	if err != nil {
		t.Fatalf("start interactive: %v", err)
	}
	if controller.startCalls != 1 {
		t.Fatalf("expected one start call, got %d", controller.startCalls)
	}
	if started.Handle.HandleID == "" || started.Handle.SessionID != sess.SessionID || started.Handle.Status != execution.RuntimeHandleActive {
		t.Fatalf("unexpected started runtime: %#v", started)
	}
	if started.Handle.Value != "backend-started" || !started.Capabilities.View || !started.Capabilities.Write || !started.Capabilities.Close {
		t.Fatalf("unexpected started interactive capabilities: %#v", started)
	}

	viewed, err := rt.ViewInteractive(context.Background(), started.Handle.HandleID, hruntime.InteractiveViewRequest{
		Offset:   0,
		MaxBytes: 32,
		Metadata: map[string]any{"viewer": "test"},
	})
	if err != nil {
		t.Fatalf("view interactive: %v", err)
	}
	if controller.viewCalls != 1 || viewed.Data != "hello" || viewed.Runtime.Observation.NextOffset != 5 {
		t.Fatalf("unexpected view result: %#v", viewed)
	}
	if viewed.Runtime.LastOperation.Kind != execution.InteractiveOperationView {
		t.Fatalf("expected view operation metadata, got %#v", viewed.Runtime.LastOperation)
	}

	written, err := rt.WriteInteractive(context.Background(), started.Handle.HandleID, hruntime.InteractiveWriteRequest{
		Input:    "ping\n",
		Metadata: map[string]any{"writer": "test"},
	})
	if err != nil {
		t.Fatalf("write interactive: %v", err)
	}
	if controller.writeCalls != 1 || written.Bytes != 5 {
		t.Fatalf("unexpected write result: %#v", written)
	}
	if written.Runtime.LastOperation.Kind != execution.InteractiveOperationWrite || written.Runtime.LastOperation.Bytes != 5 {
		t.Fatalf("expected write operation metadata, got %#v", written.Runtime.LastOperation)
	}

	reopened, err := rt.ReopenInteractive(context.Background(), started.Handle.HandleID, hruntime.InteractiveReopenRequest{
		Metadata: map[string]any{"reopen": true},
	})
	if err != nil {
		t.Fatalf("reopen interactive: %v", err)
	}
	if controller.reopenCalls != 1 || reopened.Observation.NextOffset != 7 {
		t.Fatalf("unexpected reopen result: %#v", reopened)
	}
	if reopened.LastOperation.Kind != execution.InteractiveOperationReopen {
		t.Fatalf("expected reopen operation metadata, got %#v", reopened.LastOperation)
	}

	closed, err := rt.CloseInteractive(context.Background(), started.Handle.HandleID, hruntime.InteractiveCloseRequest{
		Reason:   "operator done",
		Metadata: map[string]any{"closer": "test"},
	})
	if err != nil {
		t.Fatalf("close interactive: %v", err)
	}
	if controller.closeCalls != 1 || !closed.Observation.Closed || closed.Observation.Status != "closed" {
		t.Fatalf("unexpected close result: %#v", closed)
	}

	persisted, err := rt.GetRuntimeHandle(started.Handle.HandleID)
	if err != nil {
		t.Fatalf("get persisted handle: %v", err)
	}
	if persisted.Status != execution.RuntimeHandleClosed || persisted.StatusReason != "operator done" {
		t.Fatalf("expected persisted closed handle state, got %#v", persisted)
	}
	if persisted.Metadata[execution.InteractiveMetadataKeyLastOperationKind] != string(execution.InteractiveOperationClose) {
		t.Fatalf("expected last operation metadata to persist, got %#v", persisted.Metadata)
	}
}

func TestInteractiveControlPlaneRequiresController(t *testing.T) {
	rt := hruntime.New(hruntime.Options{})
	sess := mustCreateSession(t, rt, "interactive unsupported", "error without interactive controller")

	if _, err := rt.StartInteractive(context.Background(), sess.SessionID, hruntime.InteractiveStartRequest{Kind: "stub"}); err == nil {
		t.Fatalf("expected start interactive without controller to fail")
	} else if got := err.Error(); got == "" {
		t.Fatalf("expected concrete error, got %v", err)
	}
}

func TestInteractiveViewClosesHandleWhenBackendReportsClosed(t *testing.T) {
	controller := &stubInteractiveController{viewClosed: true}
	rt := hruntime.New(hruntime.Options{InteractiveController: controller})
	sess := mustCreateSession(t, rt, "interactive view close", "close handle when view reports closed")

	started, err := rt.StartInteractive(context.Background(), sess.SessionID, hruntime.InteractiveStartRequest{Kind: "stub"})
	if err != nil {
		t.Fatalf("start interactive: %v", err)
	}

	viewed, err := rt.ViewInteractive(context.Background(), started.Handle.HandleID, hruntime.InteractiveViewRequest{})
	if err != nil {
		t.Fatalf("view interactive: %v", err)
	}
	if !viewed.Runtime.Observation.Closed || viewed.Runtime.Handle.Status != execution.RuntimeHandleClosed {
		t.Fatalf("expected viewed runtime to close the handle, got %#v", viewed)
	}

	handle, err := rt.GetRuntimeHandle(started.Handle.HandleID)
	if err != nil {
		t.Fatalf("get runtime handle: %v", err)
	}
	if handle.Status != execution.RuntimeHandleClosed || handle.Metadata[execution.InteractiveMetadataKeyLastOperationKind] != string(execution.InteractiveOperationView) {
		t.Fatalf("expected persisted closed handle with view operation metadata, got %#v", handle)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return fmt.Sprintf("fallback-%d", len(values))
}

func TestBlockedRuntimeProjectionIncludesWaitAndInteractiveState(t *testing.T) {
	rt := newBlockedRuntimeTestService()
	attached, initial := seedApprovalBlockedSession(t, rt, "blocked runtime projection", "project blocked wait state")

	attempts := mustListAttempts(t, rt, attached.SessionID)
	if len(attempts) != 1 {
		t.Fatalf("expected one blocked attempt, got %#v", attempts)
	}
	_, err := rt.RuntimeHandles.Create(execution.RuntimeHandle{
		HandleID:  "hdl_blocked_projection",
		SessionID: attached.SessionID,
		TaskID:    attached.TaskID,
		AttemptID: attempts[0].AttemptID,
		CycleID:   attempts[0].CycleID,
		Status:    execution.RuntimeHandleActive,
		Metadata: map[string]any{
			execution.InteractiveMetadataKeyEnabled:      true,
			execution.InteractiveMetadataKeySupportsView: true,
			execution.InteractiveMetadataKeyStatus:       "active",
			execution.InteractiveMetadataKeyNextOffset:   int64(12),
		},
	})
	if err != nil {
		t.Fatalf("seed blocked interactive handle: %v", err)
	}

	view, err := rt.GetBlockedRuntimeProjection(attached.SessionID)
	if err != nil {
		t.Fatalf("get blocked runtime projection: %v", err)
	}
	if view.Runtime.BlockedRuntimeID != initial.Execution.PendingApproval.ApprovalID {
		t.Fatalf("unexpected blocked runtime projection identity: %#v", view)
	}
	if view.Wait.WaitingFor != "approval" || view.Wait.StepID == "" {
		t.Fatalf("expected wait projection to point at approval step, got %#v", view.Wait)
	}
	if len(view.InteractiveRuntimes) != 1 || view.InteractiveRuntimes[0].Handle.HandleID != "hdl_blocked_projection" {
		t.Fatalf("expected blocked projection to expose interactive runtime state, got %#v", view)
	}

	items, err := rt.ListBlockedRuntimeProjections()
	if err != nil {
		t.Fatalf("list blocked runtime projections: %v", err)
	}
	if len(items) != 1 || items[0].Runtime.BlockedRuntimeID != view.Runtime.BlockedRuntimeID {
		t.Fatalf("expected list projection to contain the same blocked runtime, got %#v", items)
	}

	byApproval, err := rt.GetBlockedRuntimeProjectionByApproval(initial.Execution.PendingApproval.ApprovalID)
	if err != nil {
		t.Fatalf("get blocked runtime projection by approval: %v", err)
	}
	if byApproval.Runtime.BlockedRuntimeID != view.Runtime.BlockedRuntimeID {
		t.Fatalf("expected approval lookup to return same blocked projection, got %#v", byApproval)
	}
}
