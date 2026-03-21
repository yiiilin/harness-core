package worker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

var (
	ErrMissingRuntime = errors.New("worker: runtime required")
)

type Worker struct {
	runtime       *hruntime.Service
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
		runtime:       opts.Runtime,
		leaseTTL:      opts.LeaseTTL,
		renewInterval: renew,
		claimModes:    modes,
	}, nil
}

func (w *Worker) RunOnce(ctx context.Context) (Result, error) {
	var res Result
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

	renewCtx, renewCancel := context.WithCancel(context.Background())
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
