package worker

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/yiiilin/harness-core/pkg/harness/session"
)

var (
	ErrMissingRuntime = errors.New("worker: runtime required")
)

type Worker struct {
	name          string
	runtime       Runtime
	leaseTTL      time.Duration
	renewInterval time.Duration
	claimModes    []session.ClaimMode
}

func New(opts Options) (*Worker, error) {
	if opts.Runtime == nil {
		return nil, ErrMissingRuntime
	}
	if opts.LeaseTTL <= 0 {
		return nil, fmt.Errorf("worker: lease ttl must be positive")
	}
	renew := opts.RenewInterval
	if renew <= 0 {
		renew = opts.LeaseTTL / 2
		if renew <= 0 {
			renew = opts.LeaseTTL
		}
	}
	modes := opts.ClaimModes
	if len(modes) == 0 {
		modes = []session.ClaimMode{session.ClaimModeRunnable, session.ClaimModeRecoverable}
	}
	return &Worker{
		name:          opts.Name,
		runtime:       opts.Runtime,
		leaseTTL:      opts.LeaseTTL,
		renewInterval: renew,
		claimModes:    modes,
	}, nil
}

func (w *Worker) RunOnce(ctx context.Context) (Result, error) {
	res := Result{WorkerName: w.name}
	claimed, mode, err := w.claim(ctx)
	if err != nil {
		return res, err
	}
	if claimed.SessionID == "" {
		res.NoWork = true
		return res, nil
	}
	res.Claimed = claimed
	res.Mode = mode

	runFn := w.runtime.RunClaimedSession
	if mode == session.ClaimModeRecoverable {
		runFn = w.runtime.RecoverClaimedSession
	}

	renewCtx, renewCancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	renewErrCh := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(w.renewInterval)
		defer ticker.Stop()
		for {
			select {
			case <-renewCtx.Done():
				return
			case <-ticker.C:
				if _, err := w.runtime.RenewSessionLease(ctx, claimed.SessionID, claimed.LeaseID, w.leaseTTL); err != nil {
					select {
					case renewErrCh <- err:
					default:
					}
					return
				}
				res.RenewalCount++
			}
		}
	}()

	runOut, runErr := runFn(ctx, claimed.SessionID, claimed.LeaseID)
	renewCancel()
	wg.Wait()
	cancelRenewErr := drainError(renewErrCh)

	if cancelRenewErr != nil && runErr == nil {
		runErr = cancelRenewErr
	}

	res.Run = runOut
	res.ApprovalPending = runOut.Session.PendingApprovalID != ""

	rel, relErr := w.runtime.ReleaseSessionLease(ctx, claimed.SessionID, claimed.LeaseID)
	res.Released = rel

	switch {
	case runErr != nil && relErr != nil:
		return res, fmt.Errorf("run error: %w; release error: %v", runErr, relErr)
	case runErr != nil:
		return res, runErr
	case relErr != nil:
		return res, relErr
	default:
		return res, nil
	}
}

// RunLoop repeatedly calls RunOnce until the context ends or the stop callback
// indicates the caller has seen enough work for now.
func (w *Worker) RunLoop(ctx context.Context, opts LoopOptions) error {
	idleWait := opts.IdleWait
	if idleWait <= 0 {
		idleWait = 250 * time.Millisecond
	}
	maxIdleWait := opts.MaxIdleWait
	if maxIdleWait < idleWait {
		maxIdleWait = idleWait
	}
	idleBackoffFactor := opts.IdleBackoffFactor
	if idleBackoffFactor < 1 {
		idleBackoffFactor = 1
	}

	errorWait := opts.ErrorWait
	if errorWait <= 0 {
		errorWait = time.Second
	}
	maxErrorWait := opts.MaxErrorWait
	if maxErrorWait < errorWait {
		maxErrorWait = errorWait
	}
	errorBackoffFactor := opts.ErrorBackoffFactor
	if errorBackoffFactor < 1 {
		errorBackoffFactor = 1
	}

	nextIdleWait := idleWait
	nextErrorWait := errorWait

	for {
		result, err := w.RunOnce(ctx)
		waitFor := time.Duration(0)
		switch {
		case err != nil:
			waitFor = nextErrorWait
			nextErrorWait = backoffWait(nextErrorWait, maxErrorWait, errorBackoffFactor)
			nextIdleWait = idleWait
		case result.NoWork:
			waitFor = nextIdleWait
			nextIdleWait = backoffWait(nextIdleWait, maxIdleWait, idleBackoffFactor)
			nextErrorWait = errorWait
		default:
			nextIdleWait = idleWait
			nextErrorWait = errorWait
		}

		stop := opts.ShouldStop != nil && opts.ShouldStop(result, err)
		if opts.Observe != nil {
			opts.Observe(LoopIteration{
				WorkerName: w.name,
				Result:     result,
				Err:        err,
				Wait:       waitFor,
				Stop:       stop,
			})
		}
		if stop {
			return err
		}
		if err != nil {
			if waitErr := sleepContext(ctx, waitFor); waitErr != nil {
				return waitErr
			}
			continue
		}
		if result.NoWork {
			if waitErr := sleepContext(ctx, waitFor); waitErr != nil {
				return waitErr
			}
			continue
		}
	}
}

func (w *Worker) claim(ctx context.Context) (session.State, session.ClaimMode, error) {
	for _, mode := range w.claimModes {
		var st session.State
		var ok bool
		var err error
		switch mode {
		case session.ClaimModeRunnable:
			st, ok, err = w.runtime.ClaimRunnableSession(ctx, w.leaseTTL)
		case session.ClaimModeRecoverable:
			st, ok, err = w.runtime.ClaimRecoverableSession(ctx, w.leaseTTL)
		default:
			continue
		}
		if err != nil {
			return session.State{}, "", err
		}
		if ok {
			return st, mode, nil
		}
	}
	return session.State{}, "", nil
}

func drainError(ch <-chan error) error {
	select {
	case err := <-ch:
		return err
	default:
		return nil
	}
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func backoffWait(current, max time.Duration, factor float64) time.Duration {
	if current <= 0 {
		return 0
	}
	if max > 0 && current >= max {
		return max
	}
	if factor <= 1 {
		return current
	}
	next := time.Duration(math.Ceil(float64(current) * factor))
	if next < current {
		next = current
	}
	if max > 0 && next > max {
		return max
	}
	return next
}
