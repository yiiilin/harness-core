package runtime_test

import (
	"errors"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/capability"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type failingSessionStore struct {
	createErr error
	listErr   error
}

func (s failingSessionStore) Create(string, string) (session.State, error) {
	return session.State{}, s.createErr
}
func (s failingSessionStore) Get(string) (session.State, error) {
	return session.State{}, session.ErrSessionNotFound
}
func (s failingSessionStore) Update(session.State) error { return nil }
func (s failingSessionStore) ClaimNext(session.ClaimMode, string, int64, int64) (session.State, bool, error) {
	return session.State{}, false, s.listErr
}
func (s failingSessionStore) RenewLease(string, string, int64, int64) (session.State, error) {
	return session.State{}, session.ErrSessionLeaseNotHeld
}
func (s failingSessionStore) ReleaseLease(string, string) (session.State, error) {
	return session.State{}, session.ErrSessionLeaseNotHeld
}
func (s failingSessionStore) List() ([]session.State, error) { return nil, s.listErr }

type failingTaskStore struct {
	createErr error
	listErr   error
}

func (s failingTaskStore) Create(task.Spec) (task.Record, error) { return task.Record{}, s.createErr }
func (s failingTaskStore) Get(string) (task.Record, error) {
	return task.Record{}, task.ErrTaskNotFound
}
func (s failingTaskStore) Update(task.Record) error     { return nil }
func (s failingTaskStore) List() ([]task.Record, error) { return nil, s.listErr }

type failingPlanStore struct {
	createErr error
	listErr   error
}

func (s failingPlanStore) Create(string, string, []plan.StepSpec) (plan.Spec, error) {
	return plan.Spec{}, s.createErr
}
func (s failingPlanStore) Get(string) (plan.Spec, error)             { return plan.Spec{}, plan.ErrPlanNotFound }
func (s failingPlanStore) ListBySession(string) ([]plan.Spec, error) { return nil, s.listErr }
func (s failingPlanStore) LatestBySession(string) (plan.Spec, bool, error) {
	return plan.Spec{}, false, s.listErr
}
func (s failingPlanStore) Update(plan.Spec) error { return nil }

type failingAuditStoreForList struct{ listErr error }

func (s failingAuditStoreForList) Emit(audit.Event) error             { return nil }
func (s failingAuditStoreForList) List(string) ([]audit.Event, error) { return nil, s.listErr }

func TestServiceCreateAndListSurfaceStoreErrors(t *testing.T) {
	createErr := errors.New("create failed")
	listErr := errors.New("list failed")

	rt := hruntime.New(hruntime.Options{
		Sessions:            failingSessionStore{createErr: createErr, listErr: listErr},
		Tasks:               failingTaskStore{createErr: createErr, listErr: listErr},
		Plans:               failingPlanStore{createErr: createErr, listErr: listErr},
		Audit:               failingAuditStoreForList{listErr: listErr},
		Approvals:           approval.NewMemoryStore(),
		Attempts:            execution.NewMemoryAttemptStore(),
		Actions:             execution.NewMemoryActionStore(),
		Verifications:       execution.NewMemoryVerificationStore(),
		Artifacts:           execution.NewMemoryArtifactStore(),
		CapabilitySnapshots: capability.NewMemorySnapshotStore(),
		ContextSummaries:    hruntime.NewMemoryContextSummaryStore(),
	})

	if _, err := rt.CreateSession("bad", "create"); !errors.Is(err, createErr) {
		t.Fatalf("expected session create error, got %v", err)
	}
	if _, err := rt.CreateTask(task.Spec{TaskType: "demo", Goal: "create"}); !errors.Is(err, createErr) {
		t.Fatalf("expected task create error, got %v", err)
	}
	if _, err := rt.CreatePlan("sess", "bad", []plan.StepSpec{{StepID: "step_1", Title: "x", Action: action.Spec{}, Verify: verify.Spec{}}}); !errors.Is(err, session.ErrSessionNotFound) {
		t.Fatalf("expected session lookup error before plan create, got %v", err)
	}

	if _, err := rt.ListSessions(); !errors.Is(err, listErr) {
		t.Fatalf("expected session list error, got %v", err)
	}
	if _, err := rt.ListTasks(); !errors.Is(err, listErr) {
		t.Fatalf("expected task list error, got %v", err)
	}
	if _, err := rt.ListPlans("sess"); !errors.Is(err, listErr) {
		t.Fatalf("expected plan list error, got %v", err)
	}
	if _, err := rt.ListAuditEvents("sess"); !errors.Is(err, listErr) {
		t.Fatalf("expected audit list error, got %v", err)
	}
}
