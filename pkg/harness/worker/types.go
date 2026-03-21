package worker

import (
	"time"

	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

// Options configures a rendered worker helper instance.
type Options struct {
	Runtime       *hruntime.Service
	LeaseTTL      time.Duration
	RenewInterval time.Duration
	ClaimModes    []session.ClaimMode
}

// Result captures what happened during a single claim/run/release iteration.
type Result struct {
	NoWork          bool
	Mode            session.ClaimMode
	Claimed         session.State
	Run             hruntime.SessionRunOutput
	Released        session.State
	RenewalCount    int
	ApprovalPending bool
}
