package runtime

import (
	"context"

	"github.com/google/uuid"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/persistence"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

func (s *Service) createSessionWithAudit(title, goal string) (session.State, error) {
	ctx := context.Background()
	var created session.State
	if s.Runner != nil {
		if err := s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
			store := s.Sessions
			if repos.Sessions != nil {
				store = repos.Sessions
			}
			var err error
			created, err = store.Create(title, goal)
			if err != nil {
				return err
			}
			if err := s.emitEventsWithSink(ctx, s.eventSinkForRepos(repos), []audit.Event{newLifecycleEventAt(s.nowMilli(), audit.EventSessionCreated, created.SessionID, "", map[string]any{
				"title": created.Title,
				"goal":  created.Goal,
			})}); err != nil {
				return err
			}
			return nil
		}); err != nil {
			return session.State{}, err
		}
		return created, nil
	}

	var err error
	created, err = s.Sessions.Create(title, goal)
	if err != nil {
		return session.State{}, err
	}
	_ = s.emitEvents(ctx, []audit.Event{newLifecycleEventAt(s.nowMilli(), audit.EventSessionCreated, created.SessionID, "", map[string]any{
		"title": created.Title,
		"goal":  created.Goal,
	})})
	return created, nil
}

func (s *Service) createTaskWithAudit(spec task.Spec) (task.Record, error) {
	ctx := context.Background()
	var created task.Record
	if s.Runner != nil {
		if err := s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
			store := s.Tasks
			if repos.Tasks != nil {
				store = repos.Tasks
			}
			var err error
			created, err = store.Create(spec)
			if err != nil {
				return err
			}
			if err := s.emitEventsWithSink(ctx, s.eventSinkForRepos(repos), []audit.Event{newLifecycleEventAt(s.nowMilli(), audit.EventTaskCreated, created.SessionID, created.TaskID, map[string]any{
				"task_type": created.TaskType,
				"goal":      created.Goal,
				"status":    created.Status,
			})}); err != nil {
				return err
			}
			return nil
		}); err != nil {
			return task.Record{}, err
		}
		return created, nil
	}

	var err error
	created, err = s.Tasks.Create(spec)
	if err != nil {
		return task.Record{}, err
	}
	_ = s.emitEvents(ctx, []audit.Event{newLifecycleEventAt(s.nowMilli(), audit.EventTaskCreated, created.SessionID, created.TaskID, map[string]any{
		"task_type": created.TaskType,
		"goal":      created.Goal,
		"status":    created.Status,
	})})
	return created, nil
}

func (s *Service) attachTaskToSession(sessionID, taskID string) (session.State, error) {
	ctx := context.Background()
	var updated session.State
	update := func(sessStore session.Store, taskStore task.Store) error {
		sess, err := sessStore.Get(sessionID)
		if err != nil {
			return err
		}
		tsk, err := taskStore.Get(taskID)
		if err != nil {
			return err
		}
		sess.TaskID = tsk.TaskID
		sess.Goal = tsk.Goal
		sess.Phase = session.PhaseReceived
		sess.Version++
		if err := sessStore.Update(sess); err != nil {
			return err
		}
		tsk.SessionID = sess.SessionID
		tsk.Status = task.StatusRunning
		if err := taskStore.Update(tsk); err != nil {
			return err
		}
		updated = sess
		return nil
	}

	if s.Runner != nil {
		err := s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
			sessStore := s.Sessions
			if repos.Sessions != nil {
				sessStore = repos.Sessions
			}
			taskStore := s.Tasks
			if repos.Tasks != nil {
				taskStore = repos.Tasks
			}
			return update(sessStore, taskStore)
		})
		return updated, err
	}

	if err := update(s.Sessions, s.Tasks); err != nil {
		return session.State{}, err
	}
	return updated, nil
}

func (s *Service) createPlanWithAudit(sessionID, changeReason string, steps []plan.StepSpec) (plan.Spec, error) {
	ctx := context.Background()
	sess, err := s.Sessions.Get(sessionID)
	if err != nil {
		return plan.Spec{}, err
	}

	var created plan.Spec
	create := func(planStore plan.Store, sink EventSink) error {
		if err := ensurePlanRevisionBudgetInStore(planStore, sessionID, s.LoopBudgets); err != nil {
			return err
		}
		var err error
		created, err = planStore.Create(sessionID, changeReason, steps)
		if err != nil {
			return err
		}
		if err := s.emitEventsWithSink(ctx, sink, []audit.Event{newLifecycleEventAt(s.nowMilli(), audit.EventPlanGenerated, sessionID, sess.TaskID, map[string]any{
			"plan_id":       created.PlanID,
			"revision":      created.Revision,
			"change_reason": created.ChangeReason,
			"step_count":    len(created.Steps),
		})}); err != nil {
			return err
		}
		return nil
	}

	if s.Runner != nil {
		err := s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
			store := s.Plans
			if repos.Plans != nil {
				store = repos.Plans
			}
			return create(store, s.eventSinkForRepos(repos))
		})
		return created, err
	}

	if err := ensurePlanRevisionBudgetInStore(s.Plans, sessionID, s.LoopBudgets); err != nil {
		return plan.Spec{}, err
	}
	created, err = s.Plans.Create(sessionID, changeReason, steps)
	if err != nil {
		return plan.Spec{}, err
	}
	_ = s.emitEvents(ctx, []audit.Event{newLifecycleEventAt(s.nowMilli(), audit.EventPlanGenerated, sessionID, sess.TaskID, map[string]any{
		"plan_id":       created.PlanID,
		"revision":      created.Revision,
		"change_reason": created.ChangeReason,
		"step_count":    len(created.Steps),
	})})
	return created, nil
}

func (s *Service) listRelatedAuditEvents(sessionID string) ([]audit.Event, error) {
	if sessionID == "" {
		return s.Audit.List("")
	}
	st, err := s.Sessions.Get(sessionID)
	if err != nil || st.TaskID == "" {
		return s.Audit.List(sessionID)
	}
	out := make([]audit.Event, 0)
	events, err := s.Audit.List("")
	if err != nil {
		return nil, err
	}
	for _, event := range events {
		if event.SessionID == sessionID || event.TaskID == st.TaskID {
			out = append(out, event)
		}
	}
	return out, nil
}

func newLifecycleEventAt(now int64, eventType, sessionID, taskID string, payload map[string]any) audit.Event {
	return audit.Event{
		EventID:   "evt_" + uuid.NewString(),
		Type:      eventType,
		SessionID: sessionID,
		TaskID:    taskID,
		Payload:   payload,
		CreatedAt: now,
	}
}
