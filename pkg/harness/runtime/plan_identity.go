package runtime

import (
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

const (
	sessionCurrentPlanIDKey       = "_kernel_current_plan_id"
	sessionCurrentPlanRevisionKey = "_kernel_current_plan_revision"
)

func annotatePlanIdentity(spec plan.Spec) plan.Spec {
	annotated := spec
	if len(spec.Steps) == 0 {
		return annotated
	}
	annotated.Steps = make([]plan.StepSpec, len(spec.Steps))
	for i, step := range spec.Steps {
		annotated.Steps[i] = annotateStepIdentity(step, spec.PlanID, spec.Revision)
	}
	return annotated
}

func annotateStepIdentity(step plan.StepSpec, planID string, planRevision int) plan.StepSpec {
	annotated := step
	if annotated.PlanID == "" {
		annotated.PlanID = planID
	}
	if annotated.PlanRevision == 0 {
		annotated.PlanRevision = planRevision
	}
	return annotated
}

func planRefFromSession(st session.State) (string, int, bool) {
	if len(st.Metadata) == 0 {
		return "", 0, false
	}
	planID, _ := st.Metadata[sessionCurrentPlanIDKey].(string)
	if planID == "" {
		return "", 0, false
	}
	return planID, intValue(st.Metadata[sessionCurrentPlanRevisionKey]), true
}

func setSessionPlanRef(st session.State, step plan.StepSpec) session.State {
	if step.PlanID == "" {
		return st
	}
	next := st
	next.Metadata = cloneAnyMap(st.Metadata)
	next.Metadata[sessionCurrentPlanIDKey] = step.PlanID
	if step.PlanRevision > 0 {
		next.Metadata[sessionCurrentPlanRevisionKey] = step.PlanRevision
	}
	return next
}

func annotateStepFromSession(st session.State, step plan.StepSpec) plan.StepSpec {
	if step.PlanID != "" {
		return step
	}
	planID, planRevision, ok := planRefFromSession(st)
	if !ok {
		return step
	}
	return annotateStepIdentity(step, planID, planRevision)
}

func planForStepInStore(store plan.Store, sessionID string, step plan.StepSpec) (plan.Spec, bool, error) {
	if store == nil {
		return plan.Spec{}, false, nil
	}
	if step.PlanID != "" {
		item, err := store.Get(step.PlanID)
		if err == nil {
			return annotatePlanIdentity(item), true, nil
		}
		if err != nil && err != plan.ErrPlanNotFound {
			return plan.Spec{}, false, err
		}
	}
	latest, ok, err := store.LatestBySession(sessionID)
	if err != nil || !ok {
		return plan.Spec{}, ok, err
	}
	return annotatePlanIdentity(latest), true, nil
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}
