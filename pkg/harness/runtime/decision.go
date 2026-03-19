package runtime

import (
	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type Transition string

const (
	TransitionStay     Transition = "stay"
	TransitionPrepare  Transition = "prepare"
	TransitionPlan     Transition = "plan"
	TransitionExecute  Transition = "execute"
	TransitionVerify   Transition = "verify"
	TransitionRecover  Transition = "recover"
	TransitionComplete Transition = "complete"
	TransitionFailed   Transition = "failed"
	TransitionAborted  Transition = "aborted"
)

type PolicyDecision struct {
	Decision permission.Decision `json:"decision"`
}

type ExecutionResult struct {
	Step   plan.StepSpec  `json:"step"`
	Action action.Result  `json:"action"`
	Verify verify.Result  `json:"verify"`
	Policy PolicyDecision `json:"policy"`
}

type TransitionDecision struct {
	From   session.Phase `json:"from"`
	To     Transition    `json:"to"`
	Reason string        `json:"reason,omitempty"`
	StepID string        `json:"step_id,omitempty"`
}
