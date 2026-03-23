package runtime

import (
	"context"

	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/capability"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/persistence"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/planning"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

func (s *Service) readRepositories(ctx context.Context, fn func(repos persistence.RepositorySet) error) error {
	if s.Runner != nil {
		return s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
			return fn(s.repositoriesWithFallback(repos))
		})
	}
	return fn(s.repositoriesWithFallback(persistence.RepositorySet{}))
}

func (s *Service) getSessionRecord(ctx context.Context, id string) (session.State, error) {
	var out session.State
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.Sessions == nil {
			return session.ErrSessionNotFound
		}
		var err error
		out, err = repos.Sessions.Get(id)
		return err
	})
	return out, err
}

func (s *Service) listSessionRecords(ctx context.Context) ([]session.State, error) {
	var out []session.State
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.Sessions == nil {
			return nil
		}
		var err error
		out, err = repos.Sessions.List()
		return err
	})
	return out, err
}

func (s *Service) getTaskRecord(ctx context.Context, id string) (task.Record, error) {
	var out task.Record
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.Tasks == nil {
			return task.ErrTaskNotFound
		}
		var err error
		out, err = repos.Tasks.Get(id)
		return err
	})
	return out, err
}

func (s *Service) listTaskRecords(ctx context.Context) ([]task.Record, error) {
	var out []task.Record
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.Tasks == nil {
			return nil
		}
		var err error
		out, err = repos.Tasks.List()
		return err
	})
	return out, err
}

func (s *Service) getPlanRecord(ctx context.Context, planID string) (plan.Spec, error) {
	var out plan.Spec
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.Plans == nil {
			return plan.ErrPlanNotFound
		}
		var err error
		out, err = repos.Plans.Get(planID)
		return err
	})
	return out, err
}

func (s *Service) listPlanRecords(ctx context.Context, sessionID string) ([]plan.Spec, error) {
	var out []plan.Spec
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.Plans == nil {
			return nil
		}
		var err error
		out, err = repos.Plans.ListBySession(sessionID)
		return err
	})
	return out, err
}

func (s *Service) latestPlanForSession(ctx context.Context, sessionID string) (plan.Spec, bool, error) {
	var (
		out plan.Spec
		ok  bool
	)
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.Plans == nil {
			return nil
		}
		var err error
		out, ok, err = repos.Plans.LatestBySession(sessionID)
		return err
	})
	return out, ok, err
}

func (s *Service) getApprovalRecord(ctx context.Context, id string) (approval.Record, error) {
	var out approval.Record
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.Approvals == nil {
			return approval.ErrApprovalNotFound
		}
		var err error
		out, err = repos.Approvals.Get(id)
		return err
	})
	return out, err
}

func (s *Service) listApprovalRecords(ctx context.Context, sessionID string) ([]approval.Record, error) {
	var out []approval.Record
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.Approvals == nil {
			return nil
		}
		var err error
		out, err = repos.Approvals.List(sessionID)
		return err
	})
	return out, err
}

func (s *Service) listAttemptRecords(ctx context.Context, sessionID string) ([]execution.Attempt, error) {
	var out []execution.Attempt
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.Attempts == nil {
			return nil
		}
		var err error
		out, err = repos.Attempts.List(sessionID)
		return err
	})
	return out, err
}

func (s *Service) listActionRecords(ctx context.Context, sessionID string) ([]execution.ActionRecord, error) {
	var out []execution.ActionRecord
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.Actions == nil {
			return nil
		}
		var err error
		out, err = repos.Actions.List(sessionID)
		return err
	})
	return out, err
}

func (s *Service) listVerificationRecords(ctx context.Context, sessionID string) ([]execution.VerificationRecord, error) {
	var out []execution.VerificationRecord
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.Verifications == nil {
			return nil
		}
		var err error
		out, err = repos.Verifications.List(sessionID)
		return err
	})
	return out, err
}

func (s *Service) listArtifactRecords(ctx context.Context, sessionID string) ([]execution.Artifact, error) {
	var out []execution.Artifact
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.Artifacts == nil {
			return nil
		}
		var err error
		out, err = repos.Artifacts.List(sessionID)
		return err
	})
	return out, err
}

func (s *Service) getRuntimeHandleRecord(ctx context.Context, id string) (execution.RuntimeHandle, error) {
	var out execution.RuntimeHandle
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.RuntimeHandles == nil {
			return execution.ErrRecordNotFound
		}
		var err error
		out, err = repos.RuntimeHandles.Get(id)
		return err
	})
	return out, err
}

func (s *Service) listRuntimeHandleRecords(ctx context.Context, sessionID string) ([]execution.RuntimeHandle, error) {
	var out []execution.RuntimeHandle
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.RuntimeHandles == nil {
			return nil
		}
		var err error
		out, err = repos.RuntimeHandles.List(sessionID)
		return err
	})
	return out, err
}

func (s *Service) listCapabilitySnapshotRecords(ctx context.Context, sessionID string) ([]capability.Snapshot, error) {
	var out []capability.Snapshot
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.CapabilitySnapshots == nil {
			return nil
		}
		var err error
		out, err = repos.CapabilitySnapshots.List(sessionID)
		return err
	})
	return out, err
}

func (s *Service) getPlanningRecord(ctx context.Context, id string) (planning.Record, error) {
	var out planning.Record
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.PlanningRecords == nil {
			return planning.ErrPlanningRecordNotFound
		}
		var err error
		out, err = repos.PlanningRecords.Get(id)
		return err
	})
	return out, err
}

func (s *Service) listPlanningRecords(ctx context.Context, sessionID string) ([]planning.Record, error) {
	var out []planning.Record
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.PlanningRecords == nil {
			return nil
		}
		var err error
		out, err = repos.PlanningRecords.List(sessionID)
		return err
	})
	return out, err
}

func (s *Service) listAuditStoreEvents(ctx context.Context, sessionID string) ([]audit.Event, error) {
	var out []audit.Event
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.Audits == nil {
			return nil
		}
		var err error
		out, err = repos.Audits.List(sessionID)
		return err
	})
	return out, err
}

func (s *Service) latestPlanHasRemainingSteps(ctx context.Context, sessionID, completedStepID string) (bool, error) {
	latest, ok, err := s.latestPlanForSession(ctx, sessionID)
	if err != nil || !ok {
		return false, err
	}
	for _, st := range latest.Steps {
		status := st.Status
		if st.StepID == completedStepID {
			status = plan.StepCompleted
		}
		if status != plan.StepCompleted {
			return true, nil
		}
	}
	return false, nil
}
