package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/builtins"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hpostgres "github.com/yiiilin/harness-core/pkg/harness/postgres"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

const (
	workerLeaseTTL      = 2 * time.Second
	workerRenewInterval = 200 * time.Millisecond
)

type Worker struct {
	Name          string
	Runtime       *hruntime.Service
	LeaseTTL      time.Duration
	RenewInterval time.Duration
}

type WorkerResult struct {
	Name         string
	Mode         string
	Session      session.State
	FinalLeaseID string
	Renewals     int
	Output       string
}

type DemoResult struct {
	Runnable      session.State
	Recoverable   session.State
	Workers       []WorkerResult
	AttemptCount  int
	TotalRenewals int
}

func main() {
	dsn := strings.TrimSpace(os.Getenv("HARNESS_POSTGRES_DSN"))
	if dsn == "" {
		panic("HARNESS_POSTGRES_DSN is required")
	}

	result, err := RunWorkersDemo(context.Background(), dsn)
	if err != nil {
		panic(err)
	}

	fmt.Printf("runnable: %s\n", result.Runnable.SessionID)
	fmt.Printf("recoverable: %s\n", result.Recoverable.SessionID)
	fmt.Printf("attempts: %d\n", result.AttemptCount)
	fmt.Printf("renewals: %d\n", result.TotalRenewals)
	for _, worker := range result.Workers {
		fmt.Printf("%s handled %s session %s\n", worker.Name, worker.Mode, worker.Session.SessionID)
	}
}

func RunWorkersDemo(ctx context.Context, dsn string) (DemoResult, error) {
	seedRT, seedDB, err := openRuntime(ctx, dsn)
	if err != nil {
		return DemoResult{}, err
	}
	defer seedDB.Close()

	recoverable, recoverableStep, err := seedDemoSession(seedRT, "recoverable-worker-session", "sleep 1; echo recovered by durable worker")
	if err != nil {
		return DemoResult{}, err
	}
	if err := makeRecoverable(ctx, seedRT, recoverable.SessionID, recoverableStep.StepID); err != nil {
		return DemoResult{}, err
	}

	runnable, _, err := seedDemoSession(seedRT, "runnable-worker-session", "sleep 1; echo handled by durable worker")
	if err != nil {
		return DemoResult{}, err
	}

	rt1, db1, err := openRuntime(ctx, dsn)
	if err != nil {
		return DemoResult{}, err
	}
	defer db1.Close()
	rt2, db2, err := openRuntime(ctx, dsn)
	if err != nil {
		return DemoResult{}, err
	}
	defer db2.Close()

	workers := []Worker{
		{Name: "worker-a", Runtime: rt1, LeaseTTL: workerLeaseTTL, RenewInterval: workerRenewInterval},
		{Name: "worker-b", Runtime: rt2, LeaseTTL: workerLeaseTTL, RenewInterval: workerRenewInterval},
	}

	results := make([]WorkerResult, len(workers))
	errs := make(chan error, len(workers))
	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			result, err := workers[idx].RunOnce(ctx)
			if err != nil {
				errs <- err
				return
			}
			results[idx] = result
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			return DemoResult{}, err
		}
	}

	runnableAttempts, err := seedRT.ListAttempts(runnable.SessionID)
	if err != nil {
		return DemoResult{}, err
	}
	recoverableAttempts, err := seedRT.ListAttempts(recoverable.SessionID)
	if err != nil {
		return DemoResult{}, err
	}

	totalRenewals := 0
	for _, worker := range results {
		totalRenewals += worker.Renewals
	}

	return DemoResult{
		Runnable:      runnable,
		Recoverable:   recoverable,
		Workers:       results,
		AttemptCount:  len(runnableAttempts) + len(recoverableAttempts),
		TotalRenewals: totalRenewals,
	}, nil
}

func (w Worker) RunOnce(ctx context.Context) (WorkerResult, error) {
	if w.Runtime == nil {
		return WorkerResult{}, fmt.Errorf("%s runtime is required", w.Name)
	}
	if w.LeaseTTL <= 0 {
		w.LeaseTTL = workerLeaseTTL
	}
	if w.RenewInterval <= 0 {
		w.RenewInterval = workerRenewInterval
	}

	claimed, ok, err := w.Runtime.ClaimRunnableSession(ctx, w.LeaseTTL)
	mode := "runnable"
	if err != nil {
		return WorkerResult{}, err
	}
	if !ok {
		claimed, ok, err = w.Runtime.ClaimRecoverableSession(ctx, w.LeaseTTL)
		mode = "recoverable"
		if err != nil {
			return WorkerResult{}, err
		}
		if !ok {
			return WorkerResult{}, fmt.Errorf("%s found no runnable or recoverable work", w.Name)
		}
	}

	stopRenew := make(chan struct{})
	renewDone := make(chan struct{})
	renewErr := make(chan error, 1)
	var renewals int32
	go func() {
		defer close(renewDone)
		ticker := time.NewTicker(w.RenewInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if _, err := w.Runtime.RenewSessionLease(ctx, claimed.SessionID, claimed.LeaseID, w.LeaseTTL); err != nil {
					renewErr <- err
					return
				}
				atomic.AddInt32(&renewals, 1)
			case <-stopRenew:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	var run hruntime.SessionRunOutput
	if mode == "recoverable" {
		run, err = w.Runtime.RecoverClaimedSession(ctx, claimed.SessionID, claimed.LeaseID)
	} else {
		run, err = w.Runtime.RunClaimedSession(ctx, claimed.SessionID, claimed.LeaseID)
	}
	close(stopRenew)
	<-renewDone
	if err != nil {
		return WorkerResult{}, err
	}
	select {
	case err := <-renewErr:
		if err != nil {
			return WorkerResult{}, err
		}
	default:
	}

	released, err := w.Runtime.ReleaseSessionLease(ctx, claimed.SessionID, claimed.LeaseID)
	if err != nil {
		return WorkerResult{}, err
	}

	output := ""
	if len(run.Executions) > 0 {
		output, _ = run.Executions[0].Execution.Action.Data["stdout"].(string)
	}
	return WorkerResult{
		Name:         w.Name,
		Mode:         mode,
		Session:      claimed,
		FinalLeaseID: released.LeaseID,
		Renewals:     int(atomic.LoadInt32(&renewals)),
		Output:       strings.TrimSpace(output),
	}, nil
}

func openRuntime(ctx context.Context, dsn string) (*hruntime.Service, *sql.DB, error) {
	var opts hruntime.Options
	builtins.Register(&opts)
	return hpostgres.OpenService(ctx, dsn, opts)
}

func seedDemoSession(rt *hruntime.Service, title, command string) (session.State, plan.StepSpec, error) {
	sess, err := rt.CreateSession(title, "durable multi-worker reference example")
	if err != nil {
		return session.State{}, plan.StepSpec{}, err
	}
	tsk, err := rt.CreateTask(task.Spec{TaskType: "demo", Goal: command})
	if err != nil {
		return session.State{}, plan.StepSpec{}, err
	}
	sess, err = rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		return session.State{}, plan.StepSpec{}, err
	}

	step := plan.StepSpec{
		StepID: "step_" + strings.ReplaceAll(sess.SessionID, "sess_", ""),
		Title:  title,
		Action: action.Spec{
			ToolName: "shell.exec",
			Args: map[string]any{
				"mode":       "pipe",
				"command":    command,
				"timeout_ms": 5000,
			},
		},
		Verify: verify.Spec{
			Mode: verify.ModeAll,
			Checks: []verify.Check{
				{Kind: "exit_code", Args: map[string]any{"allowed": []any{0}}},
			},
		},
	}
	if _, err := rt.CreatePlan(sess.SessionID, title, []plan.StepSpec{step}); err != nil {
		return session.State{}, plan.StepSpec{}, err
	}
	return sess, step, nil
}

func makeRecoverable(ctx context.Context, rt *hruntime.Service, sessionID, stepID string) error {
	claimed, ok, err := rt.ClaimRunnableSession(ctx, workerLeaseTTL)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("expected a runnable session to seed recovery")
	}
	if claimed.SessionID != sessionID {
		return fmt.Errorf("claimed unexpected session %s while seeding recoverable %s", claimed.SessionID, sessionID)
	}
	if _, err := rt.MarkClaimedSessionInFlight(ctx, sessionID, claimed.LeaseID, stepID); err != nil {
		return err
	}
	if _, err := rt.MarkClaimedSessionInterrupted(ctx, sessionID, claimed.LeaseID); err != nil {
		return err
	}
	_, err = rt.ReleaseSessionLease(ctx, sessionID, claimed.LeaseID)
	return err
}
