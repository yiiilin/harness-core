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
- `docs/EMBEDDER_VNEXT.md`
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
- `CreatePlanFromProgram`
- `CreatePlanFromPlanner`

Execution:
- `RunStep`
- `RunClaimedStep`
- `RunProgram`
- `RunSession`
- `RunClaimedSession`
- `RecoverSession`
- `RecoverClaimedSession`
- `AbortSession`

Approval and coordination:
- `RespondApproval`
- `ResumePendingApproval`
- `ResumeClaimedApproval`
- `GetBlockedRuntime`
- `GetBlockedRuntimeByApproval`
- `ListBlockedRuntimes`
- `ClaimRunnableSession`
- `ClaimRecoverableSession`
- `RenewSessionLease`
- `ReleaseSessionLease`
- `MarkSessionInFlight`
- `MarkClaimedSessionInFlight`
- `MarkSessionInterrupted`
- `MarkClaimedSessionInterrupted`

Capability matching:
- `ResolveCapability`
- `MatchCapability`

Durable execution facts and reads:
- `ListAttempts`
- `ListActions`
- `ListVerifications`
- `ListAggregateResults`
- `ListArtifacts`
- `ListRuntimeHandles`
- `GetInteractiveRuntime`
- `ListInteractiveRuntimes`
- `ListCapabilitySnapshots`
- `ListContextSummaries`
- `ListAuditEvents`
- `ListExecutionCycles`
- `GetExecutionCycle`
- `GetBlockedRuntimeProjection`
- `GetBlockedRuntimeProjectionByApproval`
- `ListBlockedRuntimeProjections`

Runtime handle control:
- `UpdateRuntimeHandle`
- `UpdateInteractiveRuntime`
- `UpdateClaimedRuntimeHandle`
- `UpdateClaimedInteractiveRuntime`
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
  - interactive runtime projection
  - blocked runtime projection
  - generic blocked-runtime contracts
  - execution target contracts
  - attachment / artifact input contracts
  - output / artifact / attachment reference contracts
  - preplanned execution-program / tool-graph contracts
  - target-slice and richer blocked-runtime projection contracts
  - output / artifact / attachment reference contracts
- runtime control types:
  - `StepRunOutput`
  - `SessionRunOutput`
  - `AbortRequest`
  - `AbortOutput`
  - `InteractiveRuntimeUpdate`
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

## Capability Match And Reason Codes

Embedders that need a stable capability-support decision should prefer:

- `MatchCapability(...)`

This public surface preserves `ResolveCapability(...)` for low-level resolution while adding stable unsupported reason codes such as:

- `CAPABILITY_NOT_FOUND`
- `CAPABILITY_DISABLED`
- `CAPABILITY_VERSION_NOT_FOUND`
- `CAPABILITY_VIEW_NOT_FOUND`
- `CAPABILITY_VIEW_DRIFT`
- `MULTI_TARGET_FANOUT_UNSUPPORTED`
- `PREPLANNED_TOOL_GRAPH_UNSUPPORTED`
- `INTERACTIVE_REOPEN_UNSUPPORTED`
- `ARTIFACT_INPUT_UNSUPPORTED`

Current support requirements are request-level hints, not proof that the runtime already implements those broader execution features.
For the explicit current support matrix, see `docs/EMBEDDER_VNEXT.md`.

## Blocked Runtime Surface

The current public blocked-runtime read surface is:

- `GetBlockedRuntime(sessionID)`
- `GetBlockedRuntimeByApproval(approvalID)`
- `ListBlockedRuntimes()`
- `GetBlockedRuntimeProjection(sessionID)`
- `GetBlockedRuntimeProjectionByApproval(approvalID)`
- `ListBlockedRuntimeProjections()`

Current scope:

- approval-backed blocked runtimes only
- durable lookup after restart by `session_id` or `approval_id`
- waiting step identity comes from the approval record
- cycle identity comes from the latest blocked attempt for that approval when present, otherwise from blocked-step metadata when present
- `ListBlockedRuntimes()` is ordered by `requested_at` ascending, with `blocked_runtime_id` as the tie-break
- richer blocked-runtime projections currently derive:
  - `ExecutionBlockedRuntimeWait` from the blocked step/action locus
  - `ExecutionTargetSlice` from the blocked cycle when target-scoped facts already exist
  - `ExecutionInteractiveRuntime` from runtime handles linked to the blocked cycle
- `GetBlockedRuntimeByApproval(approvalID)` returns:
  - `approval.ErrApprovalNotFound` when the approval id does not exist
  - `execution.ErrBlockedRuntimeNotFound` when the approval exists but is not the session's current pending blocked runtime

This does **not** mean the kernel already has a generic blocked-runtime engine for arbitrary external conditions.
That broader model remains planned vNext work.

## Execution Target Contracts

The public facade now re-exports typed execution-target contracts:

- `ExecutionTarget`
- `ExecutionTargetRef`
- `ExecutionTargetSelection`
- `ExecutionTargetSelectionMode`
- `ExecutionTargetFailureStrategy`

Current scope:

- these define how embedders and runtime slices describe target selection and partial-failure policy
- the runtime now consumes explicit declared targets through native program fan-out
- `TargetSelection.OnPartialFailure=continue` is now honored for explicit fan-out groups
- `TargetSelectionFanoutAll` remains unsupported because target discovery stays outside the kernel

This is still a partial runtime slice, not the full generalized multi-target step engine.

## Attachment / Artifact Input Contracts

The public facade now re-exports typed attachment-input contracts:

- `ExecutionAttachmentInput`
- `ExecutionAttachmentInputKind`
- `ExecutionAttachmentMaterialization`

Current scope:

- these are public model-layer contracts only
- they describe inline text/bytes inputs, artifact-backed inputs, and materialization hints
- the current runtime does **not** yet consume these contracts as a kernel-native attachment execution path

The corresponding runtime engine remains planned vNext work.

## Output / Artifact / Attachment Reference Contracts

The public facade now re-exports typed reference contracts for stable cross-step wiring:

- `ExecutionArtifactRef`
- `ExecutionAttachmentRef`
- `ExecutionOutputRef`
- `ExecutionOutputRefKind`

Current scope:

- they let embedders and runtime slices refer to prior structured output, text output, bytes output, artifacts, and attachment inputs using stable typed references
- the runtime now resolves structured/text/bytes `OutputRef` values and artifact refs for native preplanned program execution
- attachment-backed bindings remain a partial path; inline text/bytes inputs work, while broader attachment materialization semantics remain later work

This is a partial runtime slice, not the full generalized dataflow engine.

## Preplanned Execution-Program / Tool-Graph Contracts

The public facade now re-exports typed preplanned execution-program contracts:

- `ExecutionProgram`
- `ExecutionProgramNode`
- `ExecutionProgramInputBinding`
- `ExecutionProgramInputBindingKind`
- `ExecutionVerificationScope`

Current scope:

- they define a transport-neutral graph/program value shape for non-shell preplanned execution
- nodes compose generic `action.Spec`, optional `verify.Spec`, optional `on_fail`, optional target selection, dependency edges, and stable input bindings
- the runtime now exposes:
  - `CreatePlanFromProgram(sessionID, changeReason, program)`
  - `RunProgram(ctx, sessionID, program)`
  - `ListAggregateResults(sessionID)` for the current aggregate view over explicit fan-out groups
  - `execution.AttachProgram(step, program)` plus `CreatePlan(...)` for embedding a program into an otherwise normal plan
- current runtime execution is intentionally minimal:
  - explicit target fan-out from `ExecutionProgramNode.Targeting.Targets` is supported
  - dependency-ordered execution through the existing plan/session loop
  - literal, output-ref, and artifact-ref input bindings are supported for native program execution
  - explicit fan-out can now use:
    - per-target retries through `ExecutionProgramNode.OnFail`
    - partial-failure continuation through `ExecutionTargetSelection.OnPartialFailure=continue`
    - aggregate results through `RunProgram(...).Aggregates` and `ListAggregateResults(...)`
    - verification scopes through `ExecutionProgramNode.VerifyScope`
      - `step` for ordinary single-step verification
      - `target` for per-target fan-out verification
      - `aggregate` for explicit fan-out summary verification when the group resolves
- broader attachment materialization semantics, `TargetSelectionFanoutAll`, and richer aggregate replay/projection remain planned later slices

This is a partial native runtime slice, not the full vNext tool-graph engine.

## Aggregate Result Contracts

The public facade now re-exports typed aggregate result contracts for explicit fan-out groups:

- `ExecutionAggregateScope`
- `ExecutionAggregateStatus`
- `ExecutionAggregateTargetResult`
- `ExecutionAggregateResult`

Current scope:

- these contracts currently describe explicit target-fanout aggregates compiled from `ExecutionProgramNode.Targeting.Targets`
- `RunProgram(...)` returns aggregate summaries on `SessionRunOutput.Aggregates`
- `ListAggregateResults(sessionID)` derives the current aggregate view from the latest durable plan state
- `ExecutionProgramNode.VerifyScope=aggregate` evaluates `verify.Spec` against the aggregate summary for explicit fan-out groups
- aggregate status currently distinguishes:
  - `pending`
  - `completed`
  - `partial_failed`
  - `failed`
- richer replay/projection over aggregate groups remains a later slice

## Target-Slice And Richer Blocked-Runtime Projection Contracts

The public facade now re-exports typed projection contracts:

- `ExecutionTargetSlice`
- `ExecutionBlockedRuntimeProjection`
- `ExecutionBlockedRuntimeWait`
- `ExecutionBlockedRuntimeWaitScope`

Current scope:

- these contracts define the public value shape for target-scoped replay/projection and richer blocked-runtime views
- target slices are now populated when execution facts carry stable target metadata, for example from explicit program fan-out execution
- approval-backed blocked-runtime projections are now runtime-backed through:
  - `GetBlockedRuntimeProjection(...)`
  - `GetBlockedRuntimeProjectionByApproval(...)`
  - `ListBlockedRuntimeProjections()`
  - `pkg/harness/replay.SessionProjection.BlockedRuntimes`
- richer blocked-runtime progression remains limited to the current approval-backed subset; there is still no generic blocked-runtime engine for arbitrary external conditions

This is a mixed state:
- target-slice population is partially runtime-backed today
- approval-backed blocked-runtime projection is runtime-backed today
- broader generic blocked-runtime progression remains planned vNext work

## Interactive Runtime Projection Contracts

The public facade now re-exports typed interactive runtime contracts:

- `ExecutionInteractiveCapabilities`
- `ExecutionInteractiveSnapshot`
- `ExecutionInteractiveObservation`
- `ExecutionInteractiveOperation`
- `ExecutionInteractiveOperationKind`
- `ExecutionInteractiveRuntime`

The runtime also exposes:

- `GetInteractiveRuntime(handleID)`
- `ListInteractiveRuntimes(sessionID)`
- `UpdateInteractiveRuntime(handleID, update)`
- `UpdateClaimedInteractiveRuntime(handleID, leaseID, update)`

Current scope:

- interactive runtime projection is derived from persisted runtime handles plus stable interactive metadata keys
- the kernel now exposes a typed transport-neutral way to persist last-known interactive observation and reopen/view/write/close projection state without encoding PTY-specific UX into core
- `pkg/harness/replay.ExecutionCycleProjection` now exposes `InteractiveRuntimes`
- runtime-handle lifecycle still remains authoritative for active/closed/invalidated status
- actual interactive I/O backends such as PTY read/write/attach remain companion-module or embedder concerns

## Target-Scoped Execution Facts

Current explicit target-scoped fact semantics:

- explicit program fan-out now injects a stable target argument under `ExecutionTargetArgKey`
- target-derived attempts/actions/verifications/artifacts/runtime handles carry stable target metadata keys:
  - `ExecutionTargetMetadataKeyID`
  - `ExecutionTargetMetadataKeyKind`
  - `ExecutionTargetMetadataKeyName`
  - `ExecutionTargetMetadataKeyIndex`
  - `ExecutionTargetMetadataKeyCount`
- `pkg/harness/replay` uses those facts to populate `TargetSlices`
- explicit fan-out steps also carry stable aggregate metadata for durable partial-failure / aggregate-result derivation:
  - `ExecutionAggregateMetadataKeyID`
  - `ExecutionAggregateMetadataKeyScope`
  - `ExecutionAggregateMetadataKeyStrategy`
  - `ExecutionAggregateMetadataKeyExpected`
  - `ExecutionAggregateMetadataKeyTitle`

Current limits:

- target scoping is driven by explicit program fan-out, not by a generalized multi-target step engine yet
- `TargetSelectionFanoutAll` remains unsupported because target discovery stays outside the kernel
- aggregate verification is currently limited to explicit program fan-out groups and is attached to the resolving target-step verification record

## Generic Blocked-Runtime Contract Types

The public facade now re-exports generic blocked-runtime contract types:

- `ExecutionBlockedRuntimeRecord`
- `ExecutionBlockedRuntimeSubject`
- `ExecutionBlockedRuntimeCondition`
- `ExecutionBlockedRuntimeConditionKind`

Current scope:

- these are public model-layer contracts only
- they define a transport-neutral durable record shape beyond today's approval-backed blocked-runtime read subset
- current runtime read APIs still return the approval-shaped `BlockedRuntime` projection and do not yet persist or return the generic record form

The corresponding runtime engine and durable API surface remain planned vNext work.

## Output / Artifact / Attachment Reference Contracts

The public facade now re-exports typed reference contracts:

- `ExecutionArtifactRef`
- `ExecutionAttachmentRef`
- `ExecutionOutputRef`
- `ExecutionOutputRefKind`

Current scope:

- they define stable cross-step references to structured output, text output, bytes output, persisted artifacts, and attachment inputs
- they are the value shape used by the native program-binding resolver for structured/text/bytes output refs and artifact refs
- broader attachment input materialization and richer replay/projection over these refs remain later work

This is a partial runtime slice, not the full generalized dataflow engine.

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
