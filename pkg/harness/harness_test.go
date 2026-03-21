package harness_test

import (
	"context"
	"testing"
	"time"

	"github.com/yiiilin/harness-core/pkg/harness"
)

func TestNewDefaultProvidesCoreComponents(t *testing.T) {
	rt := harness.NewDefault()
	if rt == nil {
		t.Fatalf("expected runtime, got nil")
	}
	info := rt.RuntimeInfo()
	if !info.HasPlanner {
		t.Fatalf("expected default planner to be present")
	}
	if !info.HasContextAssembler {
		t.Fatalf("expected default context assembler to be present")
	}
	if !info.HasEventSink {
		t.Fatalf("expected default event sink to be present")
	}
	if len(rt.ListTools()) != 0 {
		t.Fatalf("expected bare-kernel default path to keep builtin modules out, got %d tools", len(rt.ListTools()))
	}
}

func TestNewWithBuiltinsRegistersBuiltins(t *testing.T) {
	rt := harness.NewWithBuiltins()
	if len(rt.ListTools()) < 2 {
		t.Fatalf("expected built-in tools to be registered")
	}
	if len(rt.ListVerifiers()) < 2 {
		t.Fatalf("expected built-in verifiers to be registered")
	}
}

func TestFacadeReexportsKernelRuntimeControlTypes(t *testing.T) {
	var _ harness.StepRunOutput
	var _ harness.SessionRunOutput
	var _ harness.AbortRequest
	var _ harness.AbortOutput
	var _ harness.RuntimeHandleUpdate
	var _ harness.RuntimeHandleCloseRequest
	var _ harness.RuntimeHandleInvalidateRequest
	var _ harness.CompactionPolicy
	var _ harness.CompactionTrigger = harness.CompactionTriggerPlan
	var _ harness.WorkerOptions
	var _ harness.WorkerResult
	var _ harness.ReplaySessionProjection
	var _ harness.ReplayExecutionCycleProjection
}

func TestFacadeExposesClaimAwareKernelEntryPoints(t *testing.T) {
	var _ = (*harness.Service).RunClaimedStep
	var _ = (*harness.Service).RunClaimedSession
	var _ = (*harness.Service).ResumeClaimedApproval
	var _ = (*harness.Service).RecoverClaimedSession
	var _ = (*harness.Service).MarkClaimedSessionInFlight
	var _ = (*harness.Service).MarkClaimedSessionInterrupted
}

func TestFacadeSupportsKernelSessionControlEntryPoints(t *testing.T) {
	rt := harness.NewDefault()

	sess, err := rt.CreateSession("facade", "exercise kernel entry points")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	tsk, err := rt.CreateTask(harness.TaskSpec{TaskType: "demo", Goal: "facade session control"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	sess, err = rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		t.Fatalf("attach task: %v", err)
	}

	if _, _, err := rt.CompactSessionContext(context.Background(), sess.SessionID, harness.CompactionTriggerPlan); err != nil {
		t.Fatalf("compact session context: %v", err)
	}

	claimed, ok, err := rt.ClaimRunnableSession(context.Background(), time.Minute)
	if err != nil {
		t.Fatalf("claim runnable session: %v", err)
	}
	if !ok || claimed.SessionID != sess.SessionID {
		t.Fatalf("expected claimed session %s, got %#v ok=%v", sess.SessionID, claimed, ok)
	}

	if _, err := rt.ReleaseSessionLease(context.Background(), claimed.SessionID, claimed.LeaseID); err != nil {
		t.Fatalf("release session lease: %v", err)
	}
	if _, err := rt.AbortSession(context.Background(), sess.SessionID, harness.AbortRequest{Code: "facade.abort", Reason: "stop from facade"}); err != nil {
		t.Fatalf("abort session: %v", err)
	}
}

func TestFacadeConstructsWorkerAndReplayHelpers(t *testing.T) {
	rt := harness.NewDefault()
	workerHelper, err := harness.NewWorkerHelper(harness.WorkerOptions{
		Runtime:  rt,
		LeaseTTL: time.Minute,
	})
	if err != nil {
		t.Fatalf("new worker helper: %v", err)
	}
	if workerHelper == nil {
		t.Fatalf("expected worker helper")
	}
	replayReader := harness.NewReplayReader(rt)
	if replayReader == nil {
		t.Fatalf("expected replay reader")
	}
}
