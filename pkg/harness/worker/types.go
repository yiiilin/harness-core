package worker

import (
	"context"
	"time"

	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

// Runtime is the minimal worker-facing runtime surface required by the helper.
// A concrete *runtime.Service satisfies this interface, but embedders can also
// provide compatible wrappers in tests or alternate compositions.
type Runtime interface {
	ClaimRunnableSession(ctx context.Context, leaseTTL time.Duration) (session.State, bool, error)
	ClaimRecoverableSession(ctx context.Context, leaseTTL time.Duration) (session.State, bool, error)
	RenewSessionLease(ctx context.Context, sessionID, leaseID string, leaseTTL time.Duration) (session.State, error)
	ReleaseSessionLease(ctx context.Context, sessionID, leaseID string) (session.State, error)
	RunClaimedSession(ctx context.Context, sessionID, leaseID string) (hruntime.SessionRunOutput, error)
	RecoverClaimedSession(ctx context.Context, sessionID, leaseID string) (hruntime.SessionRunOutput, error)
}

// Options configures a rendered worker helper instance.
type Options struct {
	// Name is an optional identifier for this helper instance, intended for
	// embedding-platform observability and logs. It does not alter kernel
	// semantics.
	Name          string
	Runtime       Runtime
	LeaseTTL      time.Duration
	RenewInterval time.Duration
	ClaimModes    []session.ClaimMode
}

// Result captures what happened during a single claim/run/release iteration.
type Result struct {
	WorkerName      string
	NoWork          bool
	Mode            session.ClaimMode
	Claimed         session.State
	Run             hruntime.SessionRunOutput
	Released        session.State
	RenewalCount    int
	ApprovalPending bool
}

// LoopIteration captures the outcome of one RunLoop iteration.
// It exists so embedding platforms can observe polling behavior without forking
// the worker helper.
type LoopIteration struct {
	WorkerName string
	Result     Result
	Err        error
	Wait       time.Duration
	Stop       bool
}

// LoopOptions configures the outer RunLoop polling behavior.
type LoopOptions struct {
	IdleWait          time.Duration
	MaxIdleWait       time.Duration
	IdleBackoffFactor float64

	ErrorWait          time.Duration
	MaxErrorWait       time.Duration
	ErrorBackoffFactor float64

	Observe    func(LoopIteration)
	ShouldStop func(Result, error) bool
}
