package runtime

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/persistence"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

func (s *Service) createSessionWithAudit(title, goal string) session.State {
	ctx := context.Background()
	var created session.State
	if s.Runner != nil {
		if err := s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
			store := s.Sessions
			if repos.Sessions != nil {
				store = repos.Sessions
			}
			created = store.Create(title, goal)
			_ = s.emitEventsWithSink(ctx, s.eventSinkForRepos(repos), []audit.Event{newLifecycleEvent(audit.EventSessionCreated, created.SessionID, "", map[string]any{
				"title": created.Title,
				"goal":  created.Goal,
			})})
			return nil
		}); err != nil {
			panic(err)
		}
		return created
	}

	created = s.Sessions.Create(title, goal)
	_ = s.emitEvents(ctx, []audit.Event{newLifecycleEvent(audit.EventSessionCreated, created.SessionID, "", map[string]any{
		"title": created.Title,
		"goal":  created.Goal,
	})})
	return created
}

func (s *Service) createTaskWithAudit(spec task.Spec) task.Record {
	ctx := context.Background()
	var created task.Record
	if s.Runner != nil {
		if err := s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
			store := s.Tasks
			if repos.Tasks != nil {
				store = repos.Tasks
			}
			created = store.Create(spec)
			_ = s.emitEventsWithSink(ctx, s.eventSinkForRepos(repos), []audit.Event{newLifecycleEvent(audit.EventTaskCreated, created.SessionID, created.TaskID, map[string]any{
				"task_type": created.TaskType,
				"goal":      created.Goal,
				"status":    created.Status,
			})})
			return nil
		}); err != nil {
			panic(err)
		}
		return created
	}

	created = s.Tasks.Create(spec)
	_ = s.emitEvents(ctx, []audit.Event{newLifecycleEvent(audit.EventTaskCreated, created.SessionID, created.TaskID, map[string]any{
		"task_type": created.TaskType,
		"goal":      created.Goal,
		"status":    created.Status,
	})})
	return created
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
		created = planStore.Create(sessionID, changeReason, steps)
		_ = s.emitEventsWithSink(ctx, sink, []audit.Event{newLifecycleEvent(audit.EventPlanGenerated, sessionID, sess.TaskID, map[string]any{
			"plan_id":       created.PlanID,
			"revision":      created.Revision,
			"change_reason": created.ChangeReason,
			"step_count":    len(created.Steps),
		})})
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

	err = create(s.Plans, s.EventSink)
	return created, err
}

func (s *Service) listRelatedAuditEvents(sessionID string) []audit.Event {
	if sessionID == "" {
		return s.Audit.List("")
	}
	st, err := s.Sessions.Get(sessionID)
	if err != nil || st.TaskID == "" {
		return s.Audit.List(sessionID)
	}
	out := make([]audit.Event, 0)
	for _, event := range s.Audit.List("") {
		if event.SessionID == sessionID || event.TaskID == st.TaskID {
			out = append(out, event)
		}
	}
	return out
}

func newLifecycleEvent(eventType, sessionID, taskID string, payload map[string]any) audit.Event {
	return audit.Event{
		EventID:   "evt_" + uuid.NewString(),
		Type:      eventType,
		SessionID: sessionID,
		TaskID:    taskID,
		Payload:   payload,
		CreatedAt: time.Now().UnixMilli(),
	}
}
