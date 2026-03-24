// Command platform-embedding shows how an existing platform can wrap the
// kernel behind an accepted-first API without moving platform concepts into
// kernel types.
package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	shellmodule "github.com/yiiilin/harness-core/modules/shell"
	"github.com/yiiilin/harness-core/pkg/harness"
	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	shellexec "github.com/yiiilin/harness-core/pkg/harness/executor/shell"
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

const (
	demoExternalRunID = "run-ext-123"
	demoCommand       = "ssh demo@example.internal"
)

type AcceptedRun struct {
	ExternalRunID string `json:"external_run_id"`
	SessionID     string `json:"session_id"`
	TaskID        string `json:"task_id"`
	Status        string `json:"status"`
}

type DemoResult struct {
	Accepted                     AcceptedRun                      `json:"accepted"`
	StoredExternalRunID          string                           `json:"stored_external_run_id"`
	FirstWorkerRun               harness.WorkerResult             `json:"first_worker_run"`
	PendingApproval              harness.ApprovalRecord           `json:"pending_approval"`
	ApprovalRecord               harness.ApprovalRecord           `json:"approval_record"`
	SecondWorkerRun              harness.WorkerResult             `json:"second_worker_run"`
	RuntimeHandles               []harness.ExecutionRuntimeHandle `json:"runtime_handles"`
	Projection                   harness.ReplaySessionProjection  `json:"projection"`
	PTYVerifierRegistered        bool                             `json:"pty_verifier_registered"`
	RemotePTYCallsBeforeApproval int                              `json:"remote_pty_calls_before_approval"`
	RemotePTYCallsAfterApproval  int                              `json:"remote_pty_calls_after_approval"`
}

type Platform struct {
	Runtime   *harness.Service
	Worker    *harness.WorkerHelper
	verifiers *verify.Registry
	remotePTY *remotePTYBackend
	runIndex  map[string]string
}

type remotePTYBackend struct {
	calls int
}

type ptyApprovalPolicy struct{}

func main() {
	result, err := RunEmbeddingDemo(context.Background())
	if err != nil {
		panic(err)
	}

	fmt.Printf("accepted run: %s -> %s\n", result.Accepted.ExternalRunID, result.Accepted.SessionID)
	fmt.Printf("approval pending: %s\n", result.PendingApproval.ApprovalID)
	fmt.Printf("remote PTY calls: before=%d after=%d\n", result.RemotePTYCallsBeforeApproval, result.RemotePTYCallsAfterApproval)
	fmt.Printf("runtime handle: %s\n", result.RuntimeHandles[0].HandleID)
	fmt.Printf("projection cycles: %d\n", len(result.Projection.Cycles))
}

// RunEmbeddingDemo demonstrates the recommended accepted-first platform flow:
// keep platform-owned IDs outside kernel types, use worker helper for background
// claim/run/release, approve through public approval APIs, execute PTY work
// through an external PTY backend, and read replay/debug state from public
// execution facts.
func RunEmbeddingDemo(ctx context.Context) (DemoResult, error) {
	platform, err := NewPlatform()
	if err != nil {
		return DemoResult{}, err
	}

	accepted, err := platform.SubmitInteractiveRun(ctx, demoExternalRunID, demoCommand)
	if err != nil {
		return DemoResult{}, err
	}

	storedRunID, err := platform.StoredExternalRunID(accepted.ExternalRunID)
	if err != nil {
		return DemoResult{}, err
	}

	first, err := platform.RunWorkerOnce(ctx)
	if err != nil {
		return DemoResult{}, err
	}

	pending, err := platform.PendingApproval(accepted.ExternalRunID)
	if err != nil {
		return DemoResult{}, err
	}
	callsBeforeApproval := platform.remotePTY.Calls()

	approved, err := platform.RespondToApproval(accepted.ExternalRunID, approval.ReplyOnce)
	if err != nil {
		return DemoResult{}, err
	}

	second, err := platform.RunWorkerOnce(ctx)
	if err != nil {
		return DemoResult{}, err
	}

	handles, err := platform.RuntimeHandlesForRun(accepted.ExternalRunID)
	if err != nil {
		return DemoResult{}, err
	}
	projection, err := platform.ProjectionForRun(accepted.ExternalRunID)
	if err != nil {
		return DemoResult{}, err
	}

	return DemoResult{
		Accepted:                     accepted,
		StoredExternalRunID:          storedRunID,
		FirstWorkerRun:               first,
		PendingApproval:              pending,
		ApprovalRecord:               approved,
		SecondWorkerRun:              second,
		RuntimeHandles:               handles,
		Projection:                   projection,
		PTYVerifierRegistered:        platform.HasPTYVerifier("pty_handle_active"),
		RemotePTYCallsBeforeApproval: callsBeforeApproval,
		RemotePTYCallsAfterApproval:  platform.remotePTY.Calls(),
	}, nil
}

func NewPlatform() (*Platform, error) {
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()
	remotePTY := &remotePTYBackend{}

	shellmodule.RegisterWithOptions(tools, verifiers, shellmodule.Options{PTYBackend: remotePTY})
	rt := harness.New(harness.Options{
		Tools:     tools,
		Verifiers: verifiers,
		Policy:    ptyApprovalPolicy{},
	})
	workerHelper, err := harness.NewWorkerHelper(harness.WorkerOptions{
		Name:          "platform-worker",
		Runtime:       rt,
		LeaseTTL:      time.Minute,
		RenewInterval: 25 * time.Millisecond,
	})
	if err != nil {
		return nil, err
	}

	return &Platform{
		Runtime:   rt,
		Worker:    workerHelper,
		verifiers: verifiers,
		remotePTY: remotePTY,
		runIndex:  map[string]string{},
	}, nil
}

func (p *Platform) SubmitInteractiveRun(ctx context.Context, externalRunID, command string) (AcceptedRun, error) {
	if p == nil || p.Runtime == nil {
		return AcceptedRun{}, errors.New("platform runtime is required")
	}
	sess, err := p.Runtime.CreateSession("platform embedding", "accepted-first remote PTY request")
	if err != nil {
		return AcceptedRun{}, err
	}
	tsk, err := p.Runtime.CreateTask(task.Spec{
		TaskType: "external-run",
		Goal:     command,
		Metadata: map[string]any{
			"external_run_id": externalRunID,
			"accepted_by":     "platform-api",
		},
	})
	if err != nil {
		return AcceptedRun{}, err
	}
	sess, err = p.Runtime.AttachTaskToSession(sess.SessionID, tsk.TaskID)
	if err != nil {
		return AcceptedRun{}, err
	}
	_, err = p.Runtime.CreatePlan(sess.SessionID, "platform accepted request", []plan.StepSpec{{
		StepID: "step_remote_pty",
		Title:  "start remote PTY session",
		Action: action.Spec{
			ToolName: "shell.exec",
			Args: map[string]any{
				"mode":    "pty",
				"command": command,
				"metadata": map[string]any{
					"external_run_id": externalRunID,
					"execution_mode":  "remote-pty",
				},
			},
		},
	}})
	if err != nil {
		return AcceptedRun{}, err
	}
	p.runIndex[externalRunID] = sess.SessionID
	_ = ctx
	return AcceptedRun{
		ExternalRunID: externalRunID,
		SessionID:     sess.SessionID,
		TaskID:        tsk.TaskID,
		Status:        "accepted",
	}, nil
}

func (p *Platform) RunWorkerOnce(ctx context.Context) (harness.WorkerResult, error) {
	if p == nil || p.Worker == nil {
		return harness.WorkerResult{}, errors.New("platform worker is required")
	}
	return p.Worker.RunOnce(ctx)
}

func (p *Platform) PendingApproval(externalRunID string) (harness.ApprovalRecord, error) {
	sessionID, err := p.sessionIDForRun(externalRunID)
	if err != nil {
		return harness.ApprovalRecord{}, err
	}
	records, err := p.Runtime.ListApprovals(sessionID)
	if err != nil {
		return harness.ApprovalRecord{}, err
	}
	for i := len(records) - 1; i >= 0; i-- {
		if records[i].Status == approval.StatusPending {
			return records[i], nil
		}
	}
	return harness.ApprovalRecord{}, errors.New("no pending approval for run")
}

func (p *Platform) RespondToApproval(externalRunID string, reply harness.ApprovalReply) (harness.ApprovalRecord, error) {
	pending, err := p.PendingApproval(externalRunID)
	if err != nil {
		return harness.ApprovalRecord{}, err
	}
	record, _, err := p.Runtime.RespondApproval(pending.ApprovalID, harness.ApprovalResponse{
		Reply: reply,
		Metadata: map[string]any{
			"source":          "external-approval-ui",
			"external_run_id": externalRunID,
		},
	})
	return record, err
}

func (p *Platform) StoredExternalRunID(externalRunID string) (string, error) {
	sessionID, err := p.sessionIDForRun(externalRunID)
	if err != nil {
		return "", err
	}
	st, err := p.Runtime.GetSession(sessionID)
	if err != nil {
		return "", err
	}
	record, err := p.Runtime.GetTask(st.TaskID)
	if err != nil {
		return "", err
	}
	value, _ := record.Metadata["external_run_id"].(string)
	return value, nil
}

func (p *Platform) RuntimeHandlesForRun(externalRunID string) ([]harness.ExecutionRuntimeHandle, error) {
	sessionID, err := p.sessionIDForRun(externalRunID)
	if err != nil {
		return nil, err
	}
	return p.Runtime.ListRuntimeHandles(sessionID)
}

func (p *Platform) ProjectionForRun(externalRunID string) (harness.ReplaySessionProjection, error) {
	sessionID, err := p.sessionIDForRun(externalRunID)
	if err != nil {
		return harness.ReplaySessionProjection{}, err
	}
	reader := harness.NewReplayReader(p.Runtime)
	return reader.SessionProjection(sessionID)
}

func (p *Platform) HasPTYVerifier(kind string) bool {
	if p == nil || p.verifiers == nil {
		return false
	}
	_, ok := p.verifiers.Get(kind)
	return ok
}

func (p *Platform) sessionIDForRun(externalRunID string) (string, error) {
	if p == nil {
		return "", errors.New("platform is nil")
	}
	sessionID := p.runIndex[externalRunID]
	if sessionID == "" {
		return "", errors.New("external run id not found")
	}
	return sessionID, nil
}

func (b *remotePTYBackend) Execute(_ context.Context, req shellexec.Request) (action.Result, error) {
	b.calls++
	handleID := fmt.Sprintf("remote-pty-%d", b.calls)
	externalRunID, _ := req.Metadata["external_run_id"].(string)

	return action.Result{
		OK: true,
		Data: map[string]any{
			"mode":    "pty",
			"command": req.Command,
			"status":  "active",
			"runtime_handle": execution.RuntimeHandle{
				HandleID: handleID,
				Kind:     "pty",
				Status:   execution.RuntimeHandleActive,
				Metadata: map[string]any{
					"provider":                                     "remote-pty",
					"external_run_id":                              externalRunID,
					execution.InteractiveMetadataKeyEnabled:        true,
					execution.InteractiveMetadataKeySupportsReopen: true,
					execution.InteractiveMetadataKeySupportsView:   true,
					execution.InteractiveMetadataKeySupportsWrite:  true,
					execution.InteractiveMetadataKeySupportsClose:  true,
					execution.InteractiveMetadataKeyStatus:         "active",
					execution.InteractiveMetadataKeyStatusReason:   "remote pty active",
					execution.InteractiveMetadataKeyNextOffset:     int64(0),
				},
			},
			"shell_stream": map[string]any{
				"handle_id": handleID,
				"provider":  "remote-pty",
				"status":    "active",
			},
		},
		Meta: map[string]any{
			"backend": "remote-pty",
		},
	}, nil
}

func (b *remotePTYBackend) Calls() int {
	if b == nil {
		return 0
	}
	return b.calls
}

func (ptyApprovalPolicy) Evaluate(_ context.Context, _ session.State, step plan.StepSpec) (permission.Decision, error) {
	if step.Action.ToolName == "shell.exec" {
		if mode, _ := step.Action.Args["mode"].(string); mode == "pty" {
			return permission.Decision{
				Action:      permission.Ask,
				Reason:      "remote PTY execution requires external approval",
				MatchedRule: "platform/approval/pty",
			}, nil
		}
	}
	return permission.Decision{
		Action:      permission.Allow,
		Reason:      "platform default allow",
		MatchedRule: "platform/allow",
	}, nil
}
