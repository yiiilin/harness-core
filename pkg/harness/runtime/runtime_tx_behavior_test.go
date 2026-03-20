package runtime_test

import (
	"context"
	"errors"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	shellexec "github.com/yiiilin/harness-core/pkg/harness/executor/shell"
	"github.com/yiiilin/harness-core/pkg/harness/persistence"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type txRecorder struct {
	repos     persistence.RepositorySet
	calls     int
	commits   int
	rollbacks int
}

func (r *txRecorder) Within(ctx context.Context, fn func(repos persistence.RepositorySet) error) error {
	r.calls++
	_ = ctx
	if err := fn(r.repos); err != nil {
		r.rollbacks++
		return err
	}
	r.commits++
	return nil
}

type failingAuditStore struct{}

func (failingAuditStore) Emit(audit.Event) error             { return errors.New("audit sink failed") }
func (failingAuditStore) List(string) ([]audit.Event, error) { return nil, nil }

func newRuntimeWithRecorder(rec *txRecorder, aud audit.Store) *hruntime.Service {
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	if aud == nil {
		aud = audit.NewMemoryStore()
	}
	rec.repos = persistence.RepositorySet{Sessions: sessions, Tasks: tasks, Plans: plans, Audits: aud}

	tools.Register(tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true}, shellexec.PipeExecutor{})
	verifiers.Register(verify.Definition{Kind: "exit_code", Description: "Verify exit code."}, verify.ExitCodeChecker{})
	verifiers.Register(verify.Definition{Kind: "output_contains", Description: "Verify output contains substring."}, verify.OutputContainsChecker{})

	return hruntime.New(hruntime.Options{
		Sessions:  sessions,
		Tasks:     tasks,
		Plans:     plans,
		Tools:     tools,
		Verifiers: verifiers,
		Audit:     aud,
		Runner:    rec,
	})
}

func seedHappyStep(tb testing.TB, rt *hruntime.Service) (session.State, plan.StepSpec) {
	tb.Helper()
	sess, err := rt.Sessions.Create("tx", "runner tx")
	if err != nil {
		tb.Fatalf("seed session: %v", err)
	}
	tsk, err := rt.Tasks.Create(task.Spec{TaskType: "demo", Goal: "transaction behavior"})
	if err != nil {
		tb.Fatalf("seed task: %v", err)
	}
	sess.TaskID = tsk.TaskID
	sess.Goal = tsk.Goal
	sess.Phase = session.PhaseReceived
	if err := rt.Sessions.Update(sess); err != nil {
		tb.Fatalf("seed session attach: %v", err)
	}
	tsk.SessionID = sess.SessionID
	tsk.Status = task.StatusRunning
	if err := rt.Tasks.Update(tsk); err != nil {
		tb.Fatalf("seed task attach: %v", err)
	}
	pl, err := rt.Plans.Create(sess.SessionID, "initial", []plan.StepSpec{{
		StepID: "step_1",
		Title:  "echo hello",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{"mode": "pipe", "command": "echo hello", "timeout_ms": 5000}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}}, {Kind: "output_contains", Args: map[string]any{"text": "hello"}}}},
	}})
	if err != nil {
		tb.Fatalf("seed plan: %v", err)
	}
	return sess, pl.Steps[0]
}

func TestRunStepTransactionCommitOnSuccess(t *testing.T) {
	rec := &txRecorder{}
	rt := newRuntimeWithRecorder(rec, nil)
	sess, step := seedHappyStep(t, rt)
	_, err := rt.RunStep(context.Background(), sess.SessionID, step)
	if err != nil {
		t.Fatalf("run step: %v", err)
	}
	if rec.calls == 0 {
		t.Fatalf("expected runner call")
	}
	if rec.commits == 0 {
		t.Fatalf("expected commit count > 0")
	}
	if rec.rollbacks != 0 {
		t.Fatalf("did not expect rollback on success")
	}
}

func TestRunStepTransactionRollbackOnAuditFailure(t *testing.T) {
	rec := &txRecorder{}
	rt := newRuntimeWithRecorder(rec, failingAuditStore{})
	sess, step := seedHappyStep(t, rt)
	_, err := rt.RunStep(context.Background(), sess.SessionID, step)
	if err == nil {
		t.Fatalf("expected error when audit store fails inside runner boundary")
	}
	if rec.calls == 0 {
		t.Fatalf("expected runner call")
	}
	if rec.rollbacks == 0 {
		t.Fatalf("expected rollback count > 0")
	}
}
