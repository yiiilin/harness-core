package runtime

import (
	"context"
	"errors"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type Info struct {
	Name          string `json:"name"`
	Mode          string `json:"mode"`
	Transport     string `json:"transport"`
	AuthMode      string `json:"auth_mode"`
	StorageMode   string `json:"storage_mode"`
	ToolCount     int    `json:"tool_count"`
	VerifierCount int    `json:"verifier_count"`
}

type Service struct {
	Sessions  session.Store
	Tasks     task.Store
	Plans     plan.Store
	Tools     *tool.Registry
	Verifiers *verify.Registry
}

func New(sessions session.Store, tasks task.Store, plans plan.Store, tools *tool.Registry, verifiers *verify.Registry) *Service {
	return &Service{Sessions: sessions, Tasks: tasks, Plans: plans, Tools: tools, Verifiers: verifiers}
}

func (s *Service) Ping() map[string]any {
	return map[string]any{"pong": true}
}

func (s *Service) RuntimeInfo() Info {
	return Info{
		Name:          "harness-core",
		Mode:          "kernel-first",
		Transport:     "adapter-defined",
		AuthMode:      "shared-token-v1",
		StorageMode:   "in-memory-dev",
		ToolCount:     len(s.Tools.List()),
		VerifierCount: len(s.Verifiers.List()),
	}
}

func (s *Service) CreateSession(title, goal string) session.State {
	return s.Sessions.Create(title, goal)
}

func (s *Service) GetSession(id string) (session.State, error) {
	return s.Sessions.Get(id)
}

func (s *Service) ListSessions() []session.State {
	return s.Sessions.List()
}

func (s *Service) CreateTask(spec task.Spec) task.Record {
	return s.Tasks.Create(spec)
}

func (s *Service) GetTask(id string) (task.Record, error) {
	return s.Tasks.Get(id)
}

func (s *Service) ListTasks() []task.Record {
	return s.Tasks.List()
}

func (s *Service) AttachTaskToSession(sessionID, taskID string) (session.State, error) {
	sess, err := s.Sessions.Get(sessionID)
	if err != nil {
		return session.State{}, err
	}
	tsk, err := s.Tasks.Get(taskID)
	if err != nil {
		return session.State{}, err
	}
	sess.TaskID = tsk.TaskID
	sess.Goal = tsk.Goal
	sess.Phase = session.PhaseReceived
	if err := s.Sessions.Update(sess); err != nil {
		return session.State{}, err
	}
	tsk.SessionID = sess.SessionID
	tsk.Status = task.StatusRunning
	if err := s.Tasks.Update(tsk); err != nil {
		return session.State{}, err
	}
	return sess, nil
}

func (s *Service) CreatePlan(sessionID, changeReason string, steps []plan.StepSpec) (plan.Spec, error) {
	if _, err := s.Sessions.Get(sessionID); err != nil {
		return plan.Spec{}, err
	}
	return s.Plans.Create(sessionID, changeReason, steps), nil
}

func (s *Service) GetPlan(planID string) (plan.Spec, error) {
	return s.Plans.Get(planID)
}

func (s *Service) ListPlans(sessionID string) []plan.Spec {
	return s.Plans.ListBySession(sessionID)
}

func (s *Service) ListTools() []tool.Definition {
	return s.Tools.List()
}

func (s *Service) ListVerifiers() []verify.Definition {
	return s.Verifiers.List()
}

func (s *Service) EnsureTool(name string) error {
	_, ok := s.Tools.Get(name)
	if !ok {
		return errors.New("tool not registered")
	}
	return nil
}

func (s *Service) InvokeAction(ctx context.Context, spec action.Spec) (action.Result, error) {
	return s.Tools.Invoke(ctx, spec)
}
