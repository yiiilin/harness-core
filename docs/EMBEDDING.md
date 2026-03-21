# EMBEDDING.md

## Goal

Provide a practical integration guide for embedding `harness-core` into an existing platform without expanding kernel scope.

This document focuses on:
- external run id mapping
- external approval UI
- remote PTY execution
- restart recovery
- accepted-first API wrappers
- adapter-facing transport guidance and what stays out of core

## Integration Principles

- keep kernel contracts in `pkg/harness/*`
- keep product concerns in your platform layer
- avoid importing `internal/*`
- treat `modules/*` and `adapters/*` as extension/reference layers, not kernel contracts
- if you expose the kernel over a transport, follow `docs/ADAPTERS.md` rather than inventing parallel runtime semantics

## Recommended Public Building Blocks

- `pkg/harness`
- `pkg/harness/postgres`
- `pkg/harness/worker`
- `pkg/harness/replay`

## Pattern 0: LLM-Backed Planner

The kernel intentionally does not include provider-specific model clients.

Recommended shape:

1. your platform calls the model it prefers
2. your planner implementation converts model output into `plan.StepSpec`
3. the kernel persists and executes those steps

Minimal shape:

```go
type ModelPlanner struct {
	Client MyModelClient
}

func (p ModelPlanner) PlanNext(ctx context.Context, state session.State, spec task.Spec, assembled runtime.ContextPackage) (plan.StepSpec, error) {
	reply, err := p.Client.Plan(ctx, assembled)
	if err != nil {
		return plan.StepSpec{}, err
	}
	return translateReplyIntoStep(reply, spec, assembled)
}
```

Keep these concerns outside the kernel:

- provider SDK choice
- prompt format
- model fallback/routing
- retrieval and memory policy
- product-specific planning heuristics

## Pattern 1: External Run ID

Kernel sessions use `session_id`.
If your platform already has `run_id`, keep a mapping table in your platform store:

- `platform_run_id -> session_id`
- optional reverse index `session_id -> platform_run_id`

Do not add `run_id` into kernel domain types.
Use wrapper APIs in your service boundary:
- accept `run_id`
- resolve `session_id`
- call kernel APIs

## Pattern 2: External Approval UI

Recommended flow:

1. execute through `RunSession`, `RunStep`, or worker helper
2. when `PendingApprovalID` is present, persist/emit approval task to your UI pipeline
3. UI/operator decides and calls your platform API
4. platform API maps to:
   - `RespondApproval(...)`
   - `ResumePendingApproval(...)` or claimed/session driver flow

Kernel owns approval state machine correctness.
Your platform owns human workflow, notification, and UI experience.

## Pattern 3: Remote PTY Executor

Use shell module options with explicit PTY backend:

```go
shellmodule.RegisterWithOptions(tools, verifiers, shellmodule.Options{
	PTYBackend: remotePTYBackend,
})
```

Key semantics:
- this path avoids hard dependency on local `PTYManager`
- PTY-specific verifiers (`pty_handle_active`, `pty_stream_contains`, `pty_exit_code`) are registered only when PTY inspection is available
  - local `PTYManager` provides both execution and inspection by default
  - remote executors should provide `PTYInspector` explicitly if they want verifier support
- `pty_stream_contains` can resume from `shell_stream.next_offset` when the action result includes that field (unless an explicit verifier `offset` overrides it)
- remote executor stream inspection should be implemented in your platform/module layer

## Pattern 4: Restart and Recovery

For durable deployments, use `pkg/harness/postgres` bootstrap and run workers via `pkg/harness/worker`.

Worker helper semantics:
- claim runnable/recoverable
- renew lease heartbeat
- run or recover claimed session
- release lease
- optionally keep the outer polling loop in the helper through `RunLoop(...)`
- optionally label the helper with `worker.Options{Name: ...}` for platform logs/metrics
- optionally observe each loop iteration through `worker.LoopOptions{Observe: ...}`
- optionally apply deterministic idle/error backoff in `RunLoop(...)` without adding fleet-specific logic into the kernel

On service restart:
- start workers again
- helper will claim available runnable/recoverable sessions
- recovery remains lease-governed and transport-neutral

## Pattern 5: Accepted-First API Wrapper

Use an async platform API style:

1. API accepts request and returns `accepted` with platform `run_id`
2. background worker drives kernel execution
3. client reads progress/status through your projection endpoints

Projection path:
- read kernel facts and cycles from runtime APIs
- optionally use `pkg/harness/replay` to build ordered cycle/event views
- present product-specific JSON/UI models outside kernel packages

## Replay and Debug Projection

Prefer this read chain:
- `ListExecutionCycles(session_id)`
- `GetExecutionCycle(session_id, cycle_id)` when needed
- `ListAuditEvents(session_id)`
- `pkg/harness/replay` projection helpers for stable ordering and grouping

Do not query internal tables directly from embedding code.

## Boundary Checklist

Belongs in kernel usage:
- state machine, approvals, lease coordination, recovery, runtime facts

Belongs in platform layer:
- user/tenant/org ownership
- auth and access control
- approval UI and notification routing
- queue/fleet topology
- billing/quota/reporting
- transport protocol envelopes

If you need these platform concepts, add them around the kernel, not into it.
