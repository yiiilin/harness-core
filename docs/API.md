# API.md

## Purpose

This document defines the embedder-facing public API for `harness-core`.

Primary import path:

```go
import "github.com/yiiilin/harness-core/pkg/harness"
```

Scope rule:
- kernel API only
- no transport/auth/user/tenant/product API in kernel types

Repository module layout:
- root kernel module: `github.com/yiiilin/harness-core`
- companion composition module: `github.com/yiiilin/harness-core/pkg/harness/builtins`
- companion capability-pack module: `github.com/yiiilin/harness-core/modules`
- companion adapter module: `github.com/yiiilin/harness-core/adapters`
- companion CLI module: `github.com/yiiilin/harness-core/cmd/harness-core`
- local in-repo development uses the committed `go.work`

See:
- `docs/KERNEL_SCOPE.md`
- `docs/VERSIONING.md`
- `docs/EMBEDDING.md`
- `docs/ADAPTERS.md`
- `docs/RELEASING.md`

## Recommended Public Surface

### Primary facade

- `pkg/harness`
  - constructors:
    - `harness.New(opts)`
    - `harness.NewDefault()`
  - constructor default:
    - runtime creation installs a local in-memory `Runner` over the configured stores unless you explicitly replace or clear it
    - clearing `Service.Runner` opts into direct-store best-effort event semantics and should be treated as an explicit local-mode choice

### Stable root helper packages

- `pkg/harness/postgres`
  - `Config`
  - `EnsureSchema(...)`
  - `OpenDB(...)`
  - `OpenDBWithConfig(...)`
  - `EmbeddedMigrations()`
  - `ApplyMigrations(...)`
  - `ApplySchema(...)`
  - `ListMigrationStatus(...)`
  - `PendingMigrations(...)`
  - `HasSchemaDrift(...)`
  - `SchemaVersion(...)`
  - `LatestSchemaVersion()`
  - `BuildOptions(...)`
  - `OpenService(...)`
  - `OpenServiceWithConfig(...)`

- `pkg/harness/worker`
  - `worker.New(worker.Options{Runtime: rt, ...})`
  - `(*worker.Worker).RunOnce(ctx)`
  - `(*worker.Worker).RunLoop(ctx, worker.LoopOptions{...})`
  - runtime dependency is a narrow worker-facing interface, not a required concrete `*runtime.Service`
  - additive helper ergonomics:
    - optional `Options.Name` for embedder logs/metrics labels
    - optional loop observation via `LoopOptions.Observe`
    - deterministic idle/error polling backoff via `LoopOptions{IdleWait, MaxIdleWait, IdleBackoffFactor, ErrorWait, MaxErrorWait, ErrorBackoffFactor}`
  - result flags:
    - `NoWork`
    - `ApprovalPending`

- `pkg/harness/replay`
  - `replay.NewReader(source)`
  - `(*replay.Reader).SessionProjection(sessionID)`
  - `(*replay.Reader).ExecutionCycleProjection(sessionID, cycleID)`
  - convenience helpers:
    - `LoadSessionProjection(...)`
    - `LoadCycleProjection(...)`

### Public companion composition module

- `pkg/harness/builtins`
  - `builtins.New()`
  - `builtins.Register(&opts)`
  - same import path as before, but now shipped from a separate `go.mod`
  - convenience composition for local/default capability packs, not part of the bare-kernel stability promise

### Public companion modules

- `modules/*`
- `adapters/*`
- `cmd/harness-core`

### Runtime control plane

Lifecycle:
- `CreateSession`
- `CreateTask`
- `AttachTaskToSession`
- `CreatePlan`
- `CreatePlanFromPlanner`

Execution:
- `RunStep`
- `RunClaimedStep`
- `RunSession`
- `RunClaimedSession`
- `RecoverSession`
- `RecoverClaimedSession`
- `AbortSession`

Approval and coordination:
- `RespondApproval`
- `ResumePendingApproval`
- `ResumeClaimedApproval`
- `ClaimRunnableSession`
- `ClaimRecoverableSession`
- `RenewSessionLease`
- `ReleaseSessionLease`
- `MarkSessionInFlight`
- `MarkClaimedSessionInFlight`
- `MarkSessionInterrupted`
- `MarkClaimedSessionInterrupted`

Durable execution facts and reads:
- `ListAttempts`
- `ListActions`
- `ListVerifications`
- `ListArtifacts`
- `ListRuntimeHandles`
- `ListCapabilitySnapshots`
- `ListContextSummaries`
- `ListAuditEvents`
- `ListExecutionCycles`
- `GetExecutionCycle`

Runtime handle control:
- `UpdateRuntimeHandle`
- `UpdateClaimedRuntimeHandle`
- `CloseRuntimeHandle`
- `CloseClaimedRuntimeHandle`
- `InvalidateRuntimeHandle`
- `InvalidateClaimedRuntimeHandle`

Context maintenance:
- `CompactSessionContext`

Read consistency rule:
- public getters/listers resolve against the same effective repository set that runtime writes use
- when a custom `Runner` overrides only some repositories, reads fall back only for the repositories the runner does not override

### Re-exported facade types

`pkg/harness` re-exports stable kernel domain and control types, including:
- task/session/plan/action/verify types
- permission decision/action types
- tool definition/risk types
- audit event type
- execution fact types:
  - attempt/action/verification/artifact/runtime handle/execution cycle
- runtime control types:
  - `StepRunOutput`
  - `SessionRunOutput`
  - `AbortRequest`
  - `AbortOutput`
  - `RuntimeHandleUpdate`
  - `RuntimeHandleCloseRequest`
  - `RuntimeHandleInvalidateRequest`
  - `CompactionTrigger`
  - `CompactionPolicy`
  - `LoopBudgets`
- worker helper types:
  - `WorkerLoopIteration`
- runtime interfaces:
  - planner
  - context assembler
  - event sink
  - metrics exporter
  - trace exporter

## Shell Module Embedder Notes

`modules/shell` is a capability module, not kernel surface, but embedders frequently rely on it.
Current extension semantics:

- `RegisterWithOptions(..., shellmodule.Options{PTYBackend: ...})` supports external PTY executors
- `RegisterWithOptions(..., shellmodule.Options{PTYInspector: ...})` supports external PTY inspection/verifier wiring
- `PTYManager` remains the default local PTY execution and inspection path
- PTY-specific verifiers are conditional:
  - `pty_handle_active`
  - `pty_stream_contains`
  - `pty_exit_code`
  - these are registered only when PTY inspection is available, either through `PTYManager` or explicit `PTYInspector`
  - `pty_handle_active` uses the verifier call context for inspection
  - `pty_stream_contains` can resolve PTY handles from `shell_stream`, `runtime_handle`, or `runtime_handles`
  - when `shell_stream.next_offset` is present, `pty_stream_contains` starts from that offset by default

Implication:
- remote PTY backend wiring does not automatically imply local PTY stream inspection/verifier support

## Stability Classification

For detailed policy, see `docs/VERSIONING.md`.

Most stable embedding path:
- `pkg/harness`
- `pkg/harness/postgres`
- `pkg/harness/worker`
- `pkg/harness/replay`

Public and supported but evolving faster pre-1.0:
- `pkg/harness/runtime`
- `pkg/harness/task`
- `pkg/harness/session`
- `pkg/harness/plan`
- `pkg/harness/action`
- `pkg/harness/verify`
- `pkg/harness/tool`
- `pkg/harness/permission`
- `pkg/harness/audit`
- `pkg/harness/persistence`
- `pkg/harness/observability`
- `pkg/harness/executor/*`

Public companion modules, independently versioned and intentionally faster-moving:
- `pkg/harness/builtins`
- `modules/*`
- `adapters/*`
- `cmd/harness-core`

No compatibility promise:
- `internal/*`
- `examples/*`
- `docs/plans/*`

## Runtime Consistency Notes

- When `runtime.Options.Runner` is configured, runtime writes execute against the runner repository set.
- Public runtime getters/listers and internal helper reads resolve through that same effective repository set, falling back to service stores only for repositories the runner does not override.
- Embedders should treat service methods as the supported read surface rather than mixing direct reads from stale or partially overridden stores.

## Runtime Budget Semantics

- `LoopBudgets.MaxTotalRuntimeMS` is enforced from durable `session.runtime_started_at`.
- `runtime_started_at` is set on the first real runtime activity, not on raw session creation.
- Planner-driven sessions, direct step execution, and claimed in-flight execution therefore share the same durable total-runtime clock semantics across restarts.

## Audit Surface Notes

`ListAuditEvents(sessionID)` is the canonical audit read surface for both execution and control-plane mutations.

In addition to step events, embedders should expect control-plane events such as:
- `session.task_attached`
- `lease.claimed` / `lease.renewed` / `lease.released`
- `recovery.state_changed`
- `runtime_handle.updated` / `runtime_handle.closed` / `runtime_handle.invalidated`

## Minimal Embedding Path

```go
import (
	"context"
	"time"

	"github.com/yiiilin/harness-core/pkg/harness/builtins"
	hpostgres "github.com/yiiilin/harness-core/pkg/harness/postgres"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/worker"
)

var opts hruntime.Options
builtins.Register(&opts)

rt, db, err := hpostgres.OpenServiceWithConfig(context.Background(), hpostgres.Config{
	DSN:             dsn,
	Schema:          "agent_kernel",
	MaxOpenConns:    8,
	MaxIdleConns:    4,
	ConnMaxLifetime: 30 * time.Minute,
	ApplyMigrations: true,
}, opts)
if err != nil {
	panic(err)
}
defer db.Close()

helper, err := worker.New(worker.Options{
	Runtime:  rt,
	LeaseTTL: time.Minute,
})
if err != nil {
	panic(err)
}
_, _ = helper.RunOnce(context.Background())
```

For platform integration patterns (external run id, external approval UI, remote PTY, restart recovery, accepted-first API wrapper), see `docs/EMBEDDING.md`.
For transport-binding rules and event/error mapping guidance, see `docs/ADAPTERS.md`.

Important boundary:
- `pkg/harness/postgres.Config` is the embedder-facing durable bootstrap config surface
- `internal/config` remains a reference CLI env loader, not a public embedder API
