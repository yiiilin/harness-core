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
- adapter-owned config surfaces such as `adapters/websocket.Config` when you choose to reuse a repository-shipped transport

## Effective Repository Consistency

When you supply `runtime.Options.Runner`, the runtime treats the runner repository set as the committed source of truth for execution-time mutations.

Constructor default:
- `runtime.New(...)` installs an in-memory unit-of-work runner over the configured stores unless you explicitly replace or clear it
- if you intentionally want direct-store best-effort behavior, clear `rt.Runner` after construction and treat event emission failures as advisory rather than transactional

Embedder rule:
- use public runtime read APIs such as `GetSession`, `ListAttempts`, `ListAuditEvents`, and replay helpers
- do not assume the bare service stores are authoritative if your runner overrides some repositories
- any repository the runner does not override falls back to the service store

## Pattern 0: Durable Bootstrap Config

Use `pkg/harness/postgres.Config` for embedder-facing durable runtime bootstrap:

```go
cfg := postgres.Config{
	DSN:             dsn,
	Schema:          "agent_kernel",
	MaxOpenConns:    8,
	MaxIdleConns:    4,
	ConnMaxLifetime: 30 * time.Minute,
	ApplyMigrations: true,
}
rt, db, err := postgres.OpenServiceWithConfig(ctx, cfg, opts)
```

Use `EnsureSchema(...)` when schema provisioning must happen explicitly.

Important boundary:
- this config is public embedding surface
- the CLI env loader in `internal/config` is only reference-layer wiring
- adapter config should stay transport-only and should not absorb DSN/schema/storage settings

For broader execution-model requests such as execution targets, blocked runtimes beyond approval-only pause, and native tool-graph/dataflow semantics, see `docs/EMBEDDER_VNEXT.md`.
That document distinguishes what is supported today from what remains planned vNext work.

## Pattern 1: LLM-Backed Planner

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

## Pattern 2: External Run ID

Kernel sessions use `session_id`.
If your platform already has `run_id`, keep a mapping table in your platform store:

- `platform_run_id -> session_id`
- optional reverse index `session_id -> platform_run_id`

Do not add `run_id` into kernel domain types.
Use wrapper APIs in your service boundary:
- accept `run_id`
- resolve `session_id`
- call kernel APIs

## Pattern 3: External Approval UI

Recommended flow:

1. execute through `RunSession`, `RunStep`, or worker helper
2. when `PendingApprovalID` is present, persist/emit approval task to your UI pipeline
3. UI/operator decides and calls your platform API
4. platform API maps to:
   - `RespondApproval(...)`
   - `ResumePendingApproval(...)` or claimed/session driver flow

Kernel owns approval state machine correctness.
Your platform owns human workflow, notification, and UI experience.

Current blocked-runtime read surface for this approval-backed model:

- `GetBlockedRuntime(sessionID)`
- `GetBlockedRuntimeByApproval(approvalID)`
- `ListBlockedRuntimes()`

Use these when your platform needs a restart-safe read model for currently blocked approval work without scraping session + approval + attempt records manually.
Current ordering/lookup contract:

- `ListBlockedRuntimes()` is sorted by `requested_at` ascending, tie-break `blocked_runtime_id`
- unknown approval ids still return `approval.ErrApprovalNotFound`
- known approval ids that are no longer the session's current pending blocked runtime return `execution.ErrBlockedRuntimeNotFound`

## Pattern 4: Remote PTY Executor

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

## Pattern 5: Restart and Recovery

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

Budget rule:
- `LoopBudgets.MaxTotalRuntimeMS` is measured from durable `session.runtime_started_at`
- that anchor is set on the first real runtime activity, such as planner execution, direct step execution, or claimed in-flight execution
- queued sessions therefore do not burn runtime budget before the runtime actually starts work

## Pattern 6: Accepted-First API Wrapper

Use an async platform API style:

1. API accepts request and returns `accepted` with platform `run_id`
2. background worker drives kernel execution
3. client reads progress/status through your projection endpoints

Projection path:
- read kernel facts and cycles from runtime APIs
- optionally use `pkg/harness/replay` to build ordered cycle/event views
- use blocked-runtime projection reads and interactive-runtime reads when you need a transport-neutral current-state view
- present product-specific JSON/UI models outside kernel packages

If you reuse a repository-shipped adapter:
- pass adapter-owned config types directly, for example `websocket.Config`
- keep env loading, durable bootstrap choice, and product-specific wiring in your platform layer or CLI wrapper

## Pattern 7: Preplanned Program Execution

For a transport-neutral preplanned tool graph, use the public program contracts plus the new runtime entry points:

```go
created, err := rt.CreatePlanFromProgram(sessionID, "external-program", program)
out, err := rt.RunProgram(ctx, sessionID, program)
aggregates, err := rt.ListAggregateResults(sessionID)

step := execution.AttachProgram(plan.StepSpec{
    StepID: "step_program",
    Title:  "external program",
}, program)
created, err := rt.CreatePlan(sessionID, "external-program", []plan.StepSpec{step})
```

Current semantics are intentionally narrow:

- dependency-ordered execution through the existing durable plan/session loop
- literal bindings plus structured/text/bytes output refs and artifact refs
- explicit target fan-out from `ExecutionProgramNode.Targeting.Targets` is supported
- execution still reuses the existing durable plan/session loop, so approvals, retries, audits, and execution facts remain consistent with normal step execution
- runtime injects the current target under `ExecutionTargetArgKey` and persists stable target metadata for replay/debug grouping
- explicit fan-out can now additionally use:
  - `ExecutionProgramNode.OnFail` for per-target retry policy
  - `ExecutionTargetSelection.OnPartialFailure=continue` to tolerate partial target failure while still failing when every target exhausts as failed
  - `RunProgram(...).Aggregates` or `ListAggregateResults(sessionID)` for the current aggregate result view
  - `ExecutionProgramNode.VerifyScope`
    - `target` for per-target fan-out verification
    - `aggregate` for explicit fan-out summary verification when the group resolves
- target discovery (`fanout_all`), broader attachment bindings/materialization, and richer aggregate replay/projection are not implemented yet

## Replay and Debug Projection

Prefer this read chain:
- `ListExecutionCycles(session_id)`
- `GetExecutionCycle(session_id, cycle_id)` when needed
- `ListAuditEvents(session_id)`
- `GetArtifact(id)` / `ReadArtifact(id, request)` when a step exposed `ActionResult.RawHandle`
  - `ReadArtifact(raw_handle.ref, ArtifactReadRequest{})` returns the default raw payload window without needing an internal schema path
  - `ReadArtifact(raw_handle.ref, ArtifactReadRequest{Path: ...})` still supports targeted byte/line rereads when the embedder wants a specific field
- `GetBlockedRuntimeProjection(...)` / `ListBlockedRuntimeProjections()` when you need current approval-backed blocked views
- `GetInteractiveRuntime(...)` / `ListInteractiveRuntimes(...)` when you need typed interactive current-state projection
- `pkg/harness/replay` projection helpers for stable ordering and grouping

The audit stream now includes both step-execution and control-plane events.
Important control-plane examples:
- `session.task_attached`
- `lease.claimed` / `lease.renewed` / `lease.released`
- `recovery.state_changed`
- `runtime_handle.updated` / `runtime_handle.closed` / `runtime_handle.invalidated`

Do not query internal tables directly from embedding code.

## Current VNext Boundary

Before building against newer embedder terminology, check `docs/EMBEDDER_VNEXT.md`.

Short version:

- `execution target`, `target-scoped action`, and `blocked runtime` are accepted terms
- typed execution-target contracts are public model-layer surface now
- typed attachment / artifact input contracts are public model-layer surface now
- typed output / artifact / attachment reference contracts are public model-layer surface now
- structured/text/bytes output refs and artifact refs are runtime-backed today for native program execution
- typed preplanned execution-program / tool-graph contracts are public model-layer surface now
- typed target-slice / richer blocked-runtime projection contracts are public model-layer surface now
- typed interactive runtime projection contracts are public model-layer surface now
- typed generic blocked-runtime durable contracts are public model-layer surface now
- single-target governed step execution is supported today
- minimal native preplanned program execution is supported today through `CreatePlanFromProgram(...)` and `RunProgram(...)`
- explicit multi-target program fan-out is partially supported today through `ExecutionProgramNode.Targeting.Targets`
- approval-backed blocked runtime is supported today
- approval-backed blocked-runtime lookup/projection is supported today through:
  - `GetBlockedRuntime(...)`
  - `GetBlockedRuntimeByApproval(...)`
  - `ListBlockedRuntimes()`
  - `GetBlockedRuntimeProjection(...)`
  - `GetBlockedRuntimeProjectionByApproval(...)`
  - `ListBlockedRuntimeProjections()`
- typed interactive runtime projection/update is supported today through `GetInteractiveRuntime(...)`, `ListInteractiveRuntimes(...)`, and `UpdateInteractiveRuntime(...)`
- target discovery-based fan-out, broader attachment/dataflow semantics, and richer blocked-runtime models are not implemented yet

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
- opaque continuation blobs for platform-specific loop state, unless the kernel explicitly adds a future generic store for them

If you need these platform concepts, add them around the kernel, not into it.
