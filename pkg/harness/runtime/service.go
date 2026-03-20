package runtime

import (
	"context"
	"errors"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/capability"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/observability"
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/persistence"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type Info struct {
	Name                string `json:"name"`
	Mode                string `json:"mode"`
	Transport           string `json:"transport"`
	AuthMode            string `json:"auth_mode"`
	StorageMode         string `json:"storage_mode"`
	ToolCount           int    `json:"tool_count"`
	VerifierCount       int    `json:"verifier_count"`
	HasPlanner          bool   `json:"has_planner"`
	HasContextAssembler bool   `json:"has_context_assembler"`
	HasEventSink        bool   `json:"has_event_sink"`
	HasMetrics          bool   `json:"has_metrics"`
}

type Service struct {
	Sessions            session.Store
	Tasks               task.Store
	Plans               plan.Store
	Approvals           approval.Store
	Attempts            execution.AttemptStore
	Actions             execution.ActionStore
	Verifications       execution.VerificationStore
	Artifacts           execution.ArtifactStore
	RuntimeHandles      execution.RuntimeHandleStore
	CapabilitySnapshots capability.SnapshotStore
	ResumePolicy        approval.ResumePolicy
	Tools               *tool.Registry
	CapabilityResolver  capability.Resolver
	Verifiers           *verify.Registry
	Audit               audit.Store
	Runner              persistence.Runner
	Policy              permission.Evaluator
	ContextAssembler    ContextAssembler
	ContextSummaries    ContextSummaryStore
	Compactor           Compactor
	LoopBudgets         LoopBudgets
	Planner             Planner
	EventSink           EventSink
	Metrics             Metrics
	MetricsRecorder     *observability.MemoryRecorder
	StorageMode         string
}

func New(opts Options) *Service {
	opts = WithDefaults(opts)
	return &Service{
		Sessions:            opts.Sessions,
		Tasks:               opts.Tasks,
		Plans:               opts.Plans,
		Approvals:           opts.Approvals,
		Attempts:            opts.Attempts,
		Actions:             opts.Actions,
		Verifications:       opts.Verifications,
		Artifacts:           opts.Artifacts,
		RuntimeHandles:      opts.RuntimeHandles,
		CapabilitySnapshots: opts.CapabilitySnapshots,
		ResumePolicy:        opts.ResumePolicy,
		Tools:               opts.Tools,
		CapabilityResolver:  opts.CapabilityResolver,
		Verifiers:           opts.Verifiers,
		Audit:               opts.Audit,
		Runner:              opts.Runner,
		Policy:              opts.Policy,
		ContextAssembler:    opts.ContextAssembler,
		ContextSummaries:    opts.ContextSummaries,
		Compactor:           opts.Compactor,
		LoopBudgets:         opts.LoopBudgets,
		Planner:             opts.Planner,
		EventSink:           opts.EventSink,
		Metrics:             metricsOrNoop(opts.Metrics),
		MetricsRecorder:     opts.MetricsRecorder,
		StorageMode:         opts.StorageMode,
	}
}

func (s *Service) WithPolicyEvaluator(policy permission.Evaluator) *Service {
	if policy != nil {
		s.Policy = policy
	}
	return s
}

func (s *Service) WithContextAssembler(assembler ContextAssembler) *Service {
	if assembler != nil {
		s.ContextAssembler = assembler
	}
	return s
}

func (s *Service) WithPlanner(planner Planner) *Service {
	if planner != nil {
		s.Planner = planner
	}
	return s
}

func (s *Service) WithEventSink(sink EventSink) *Service {
	if sink != nil {
		s.EventSink = sink
	}
	return s
}

func (s *Service) Ping() map[string]any {
	return map[string]any{"pong": true}
}

func (s *Service) RuntimeInfo() Info {
	return Info{
		Name:                "harness-core",
		Mode:                "kernel-first",
		Transport:           "adapter-defined",
		AuthMode:            "shared-token-v1",
		StorageMode:         s.StorageMode,
		ToolCount:           len(s.Tools.List()),
		VerifierCount:       len(s.Verifiers.List()),
		HasPlanner:          s.Planner != nil,
		HasContextAssembler: s.ContextAssembler != nil,
		HasEventSink:        s.EventSink != nil,
		HasMetrics:          s.Metrics != nil,
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

func (s *Service) GetApproval(id string) (approval.Record, error) {
	if s.Approvals == nil {
		return approval.Record{}, approval.ErrApprovalNotFound
	}
	return s.Approvals.Get(id)
}

func (s *Service) ListApprovals(sessionID string) []approval.Record {
	if s.Approvals == nil {
		return nil
	}
	return s.Approvals.List(sessionID)
}

func (s *Service) ListAttempts(sessionID string) []execution.Attempt {
	if s.Attempts == nil {
		return nil
	}
	return s.Attempts.List(sessionID)
}

func (s *Service) ListActions(sessionID string) []execution.ActionRecord {
	if s.Actions == nil {
		return nil
	}
	return s.Actions.List(sessionID)
}

func (s *Service) ListVerifications(sessionID string) []execution.VerificationRecord {
	if s.Verifications == nil {
		return nil
	}
	return s.Verifications.List(sessionID)
}

func (s *Service) ListArtifacts(sessionID string) []execution.Artifact {
	if s.Artifacts == nil {
		return nil
	}
	return s.Artifacts.List(sessionID)
}

func (s *Service) ListCapabilitySnapshots(sessionID string) []capability.Snapshot {
	if s.CapabilitySnapshots == nil {
		return nil
	}
	return s.CapabilitySnapshots.List(sessionID)
}

func (s *Service) ListContextSummaries(sessionID string) []ContextSummary {
	if s.ContextSummaries == nil {
		return nil
	}
	return s.ContextSummaries.List(sessionID)
}

func (s *Service) ListTools() []tool.Definition {
	return s.Tools.List()
}

func (s *Service) ListVerifiers() []verify.Definition {
	return s.Verifiers.List()
}

func (s *Service) ListAuditEvents(sessionID string) []audit.Event {
	if s.Audit == nil {
		return nil
	}
	return s.Audit.List(sessionID)
}

func (s *Service) MetricsSnapshot() observability.Snapshot {
	return SnapshotMetrics(s.MetricsRecorder)
}

func (s *Service) EnsureTool(name string) error {
	_, ok := s.Tools.Get(name)
	if !ok {
		return errors.New("tool not registered")
	}
	return nil
}

func (s *Service) EvaluatePolicy(ctx context.Context, state session.State, step plan.StepSpec) (permission.Decision, error) {
	return s.Policy.Evaluate(ctx, state, step)
}

func (s *Service) ResolveCapability(ctx context.Context, req capability.Request) (capability.Resolution, error) {
	if s.CapabilityResolver == nil {
		return capability.Resolution{}, capability.ErrCapabilityNotFound
	}
	return s.CapabilityResolver.Resolve(ctx, req)
}

func (s *Service) InvokeAction(ctx context.Context, spec action.Spec) (action.Result, error) {
	resolution, err := s.ResolveCapability(ctx, capability.Request{Action: spec})
	if err != nil {
		return capabilityErrorResult(spec, err), err
	}
	if resolution.Handler == nil {
		return action.Result{OK: false, Error: &action.Error{Code: "TOOL_NOT_IMPLEMENTED", Message: spec.ToolName}}, nil
	}
	return resolution.Handler.Invoke(ctx, spec.Args)
}

func (s *Service) EvaluateVerify(ctx context.Context, spec verify.Spec, result action.Result, state session.State) (verify.Result, error) {
	return s.Verifiers.Evaluate(ctx, spec, result, state)
}

func (s *Service) RunStep(ctx context.Context, sessionID string, step plan.StepSpec) (StepRunOutput, error) {
	return s.runStep(ctx, sessionID, step)
}
