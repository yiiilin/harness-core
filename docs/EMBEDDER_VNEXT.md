# EMBEDDER_VNEXT.md

## Goal

Describe the approved embedder-facing vNext direction without overstating what the kernel already implements today.

Use this document when an embedder asks:

- what `harness-core` supports today
- which new execution-model terms are accepted
- which requests are valid kernel-next work
- which requests still belong outside the kernel

This document is intentionally explicit about what is **not implemented yet**.

## Accepted Terminology

These terms are accepted for public kernel/embedder discussion:

- `execution target`: an embedder-supplied executable target; transport-neutral and product-neutral
- `target-scoped action`: one action execution tied to one execution target inside a broader logical execution
- `blocked runtime`: a runtime paused on an external condition such as approval, confirmation, or another external readiness signal

These terms do **not** imply product semantics such as:

- platform run IDs
- target discovery strategy
- approval TTL policy
- user / tenant / org ownership
- UI workflow state

## Support Matrix

| Area | Status today | Notes |
| --- | --- | --- |
| Single-target step execution | Supported | Current kernel baseline: `StepSpec -> action -> optional verify`. |
| Durable approval pause/resume | Supported | Approval-backed blocked state is durable and restart-safe. |
| Approval-backed blocked-runtime lookup/projection | Supported | Public reads exist for `session_id` / `approval_id` lookup, blocked-runtime listing, and blocked-runtime projection. |
| Execution target public contracts | Partial | Typed execution-target contracts are public and the runtime now consumes explicit declared targets plus resolver-backed `fanout_all` targets for native program fan-out. Broader multi-target step semantics remain unimplemented. |
| Attachment / artifact input public contracts | Partial | Typed attachment-input contracts are public and the runtime now consumes inline text/bytes, artifact-ref temp-file materialization, and custom `AttachmentMaterializer` passthrough for native program execution. Transport-specific cleanup/lifecycle policy remains outside the kernel. |
| Output / artifact / attachment reference contracts | Partial | Typed refs are public and the runtime now resolves structured/text/bytes output refs, artifact refs, and attachment materialization through `runtime.AttachmentMaterializer` for native program execution. Broader generalized dataflow remains unimplemented. |
| Preplanned execution-program / tool-graph contracts | Supported | Typed program/node/input-binding contracts are public and the runtime now exposes minimal single-target program execution entry points. |
| Target-slice / richer blocked-runtime projection contracts | Partial | Typed projection contracts are public; target slices are runtime-backed, and blocked-runtime projections are now populated for both approval-backed and generic current blocked runtimes. Broader multi-target scheduler semantics remain unimplemented. |
| Generic blocked-runtime contract types | Supported | Typed durable blocked-runtime record/condition contracts are public and now back a generic blocked-runtime lifecycle API. |
| Durable interactive runtime handles | Supported | Runtime handles, typed interactive projections, and a transport-neutral interactive controller surface for start/reopen/view/write/close are public now. Backend-specific attach/resize behavior remains embedder or module concern. |
| PTY backend replacement | Supported in companion module | `modules/shell` exposes `PTYBackend` and `PTYInspector`; this is companion-module surface, not kernel core. |
| Replay/debug execution cycles | Supported | `pkg/harness/replay` projects execution cycles plus audit events. |
| Capability unsupported reason codes | Supported | Public capability matching can return stable unsupported reason codes. |
| Native multi-target fan-out scheduler | Supported for native program execution | Explicit program fan-out across declared targets and resolver-backed `fanout_all` targets now runs through scheduler-owned fan-out rounds that consume `TargetSelection.MaxConcurrency`, preserve per-target retries / `on_partial_failure=continue`, and keep aggregate result summaries durable. Broader generic multi-target step execution outside the program path remains unimplemented. |
| Native preplanned non-shell tool graph | Partial | `CreatePlanFromProgram(...)`, `RunProgram(...)`, `ListAggregateResults(...)`, and `execution.AttachProgram(...)` execute dependency-ordered programs with explicit or resolver-backed target fan-out, per-target retry/continue semantics, structured/artifact dataflow, and temp-file attachment materialization. Broader graph semantics remain unimplemented. |
| Stable step-to-step output / artifact refs | Partial | Structured/text/bytes output refs, artifact refs, and temp-file attachment materialization now resolve natively for program execution. Generalized attachment/dataflow semantics outside that path remain unimplemented. |
| First-class blocked runtime beyond approval-shaped pause | Supported | The runtime now exposes generic blocked-runtime create/respond/resume/abort APIs plus durable record lookup by `blocked_runtime_id`. Approval-backed policy pause/resume remains a dedicated path. |
| Unified verification across graph / fan-out / interactive | Partial | `ExecutionProgramNode.VerifyScope` now supports step/target/aggregate verification across native program execution. Interactive-specific verification scope remains unimplemented. |
| Kernel-native attachment / artifact input model | Planned vNext | Not implemented yet. |
| Target-sliced replay / projection views | Partial | Replay now groups target-scoped facts into target slices, derives interactive runtimes from runtime handles, and includes blocked-runtime projections. True concurrent multi-target scheduling remains unimplemented. |

## Public Capability Match Surface

The first vNext-safe public addition is capability matching with stable unsupported reason codes.

Current reason-code set:

- `CAPABILITY_NOT_FOUND`
- `CAPABILITY_DISABLED`
- `CAPABILITY_VERSION_NOT_FOUND`
- `CAPABILITY_VIEW_NOT_FOUND`
- `CAPABILITY_VIEW_DRIFT`
- `MULTI_TARGET_FANOUT_UNSUPPORTED`
- `PREPLANNED_TOOL_GRAPH_UNSUPPORTED`
- `INTERACTIVE_REOPEN_UNSUPPORTED`
- `ARTIFACT_INPUT_UNSUPPORTED`

Current matching semantics:

- `ResolveCapability(...)` remains the low-level resolution API
- `MatchCapability(...)` adds a public supported/unsupported result with stable reason codes
- feature-level reasons are requirement-driven and metadata-driven
- current modules do not automatically claim support for vNext features unless they advertise the corresponding support metadata explicitly

Current blocked-runtime semantics:

- approval-backed blocked runtimes remain restart-safe by `session_id` and `approval_id`
- generic blocked runtimes are durable by `blocked_runtime_id`
- current blocked-runtime listing is stably ordered by `requested_at` ascending with `blocked_runtime_id` tie-break
- public generic lifecycle APIs now exist:
  - `CreateBlockedRuntime(...)`
  - `RespondBlockedRuntime(...)`
  - `ResumeBlockedRuntime(...)`
  - `AbortBlockedRuntime(...)`
  - `GetBlockedRuntimeRecord(...)`
  - `ListBlockedRuntimeRecords(...)`
- richer current blocked-runtime projection reads are now available through:
  - `GetBlockedRuntimeProjection(...)`
  - `GetBlockedRuntimeProjectionByApproval(...)`
  - `ListBlockedRuntimeProjections()`
  - `pkg/harness/replay.SessionProjection.BlockedRuntimes`
- generic blocked runtimes hold the session in a non-runnable blocked state until resume or abort clears it
- approval-backed policy pause/resume remains its own dedicated governance path

Current output/artifact/attachment reference semantics:

- `ExecutionArtifactRef` and `ExecutionAttachmentRef` are stable typed references for persisted artifacts and typed inputs
- `ExecutionOutputRef` can point at prior structured/text/bytes output plus artifact/attachment-backed values
- the runtime now resolves structured/text/bytes output refs and artifact refs for native program execution
- `runtime.AttachmentMaterializer` now owns native attachment materialization for program execution
- the default materializer supports `AttachmentInput.Materialize=temp_file` for inline text/bytes and artifact-ref payloads
- custom non-empty `AttachmentInput.Materialize` values are passed through to the configured materializer, and its returned value is injected into the tool action args
- transport-specific cleanup/lifecycle policy remains materializer-owned rather than kernel-owned

Current preplanned program/tool-graph semantics:

- `ExecutionProgram` is a transport-neutral container for future preplanned execution graphs
- `ExecutionProgramNode` carries a generic tool `action`, optional `verify`, optional `verify_scope`, optional `on_fail`, optional target selection, dependency edges, and stable input bindings
- `ExecutionProgramInputBinding` can carry literal values, `OutputRef` references, or `AttachmentInput` values
- the runtime now exposes `CreatePlanFromProgram(...)`, `RunProgram(...)`, and `execution.AttachProgram(step, program)` for plan-embedded compilation
- current native execution is intentionally minimal:
  - explicit target fan-out from declared `Targeting.Targets`
  - resolver-backed `TargetSelectionFanoutAll` target discovery through `runtime.TargetResolver`
  - dependency-ordered execution through the existing plan/session loop, with scheduler-owned concurrent fan-out rounds for ready sibling target steps
  - literal bindings plus structured/text/bytes output refs, artifact refs, default temp-file attachment materialization, and custom materializer passthrough
  - per-target retries through `ProgramNode.OnFail`
  - partial-failure continuation through `TargetSelection.OnPartialFailure=continue`
  - actual runtime consumption of `TargetSelection.MaxConcurrency` for native fan-out groups
  - aggregate result summaries through `SessionRunOutput.Aggregates` and `ListAggregateResults(...)`
  - verification scopes through `ProgramNode.VerifyScope`
    - `step` for ordinary single-step execution
    - `target` for per-target fan-out verification
    - `aggregate` for explicit fan-out summary verification when the group resolves
- interactive verification scopes and broader multi-target policy semantics remain planned vNext work

Current target-slice / blocked-runtime projection semantics:

- `ExecutionTargetSlice` is the public value shape for future target-scoped execution grouping
- `ExecutionBlockedRuntimeProjection` and `ExecutionBlockedRuntimeWait` are the public value shapes for richer blocked-runtime views
- `pkg/harness/replay.SessionProjection` and `pkg/harness/replay.ExecutionCycleProjection` populate target slices when execution facts carry stable target metadata
- `pkg/harness/replay.ExecutionCycleProjection` now also derives `InteractiveRuntimes` from persisted runtime handles
- blocked-runtime projection fields are now populated through the public projection reads and replay helper for both approval-backed and generic current blocked runtimes

Current interactive runtime semantics:

- `ExecutionInteractiveRuntime` is the public projection shape over a persisted runtime handle plus stable interactive metadata
- the runtime now exposes:
  - `StartInteractive(...)`
  - `ReopenInteractive(...)`
  - `ViewInteractive(...)`
  - `WriteInteractive(...)`
  - `CloseInteractive(...)`
  - `GetInteractiveRuntime(...)`
  - `ListInteractiveRuntimes(...)`
  - `UpdateInteractiveRuntime(...)`
  - `UpdateClaimedInteractiveRuntime(...)`
- `runtime.InteractiveController` is the transport-neutral kernel hook behind those operations
- this lets embedders own backend-specific interactive behavior while the kernel persists durable runtime-handle state, last-known observation, and replay facts
- PTY attach/resize or other transport-specific stream behaviors still remain module/embedder concerns

Current native fan-out semantics:

- `CreatePlanFromProgram(...)` and `RunProgram(...)` support explicit target fan-out from `ExecutionProgramNode.Targeting.Targets`
- fan-out is compiled into stable target-scoped plan steps plus aggregate metadata that lets the session driver recover one concurrent fan-out round at runtime
- target-scoped facts are persisted through stable target metadata on attempts/actions/verifications/artifacts/runtime handles
- explicit fan-out groups also persist stable aggregate metadata so the runtime can derive aggregate result summaries durably
- explicit fan-out groups now also persist stable `aggregate_max_concurrency` metadata so runtime execution can consume `TargetSelection.MaxConcurrency`
- `RunProgram(...)` returns aggregate summaries on `SessionRunOutput.Aggregates`
- `ListAggregateResults(sessionID)` exposes the current aggregate view from the latest durable plan
- `TargetSelection.OnPartialFailure=continue` now means:
  - each target step still respects retry budgets independently
  - exhausted failed targets no longer block the rest of the explicit fan-out group
  - the logical fan-out group still fails when every target exhausts as failed
- `TargetSelection.MaxConcurrency` now means:
  - the runtime executes ready sibling target steps in one scheduler-owned fan-out round
  - fan-out rounds cap concurrent target execution at the declared limit
  - approval-gated steps still fall back to the serial path so approval governance remains on the existing kernel chain
- `TargetSelectionFanoutAll` now resolves through the embedder-supplied `runtime.TargetResolver`
- the kernel still does not own target discovery strategy itself; the embedder provides that policy through the resolver hook
- aggregate verification is now supported for explicit program fan-out groups through `ProgramNode.VerifyScope=aggregate`

Current generic blocked-runtime contract semantics:

- `ExecutionBlockedRuntimeRecord` is the public durable record shape for non-approval generic blocked-runtime persistence
- `ExecutionBlockedRuntimeSubject` identifies the waiting step/action/target locus without assuming approval-specific fields
- `ExecutionBlockedRuntimeCondition` and `ExecutionBlockedRuntimeConditionKind` describe the external condition in a transport-neutral way
- current runtime APIs now persist and return these generic records, while current blocked-runtime reads project them into the richer `BlockedRuntime` view

## Kernel Boundary For These Requests

### Valid kernel-next work

- execution-target contracts
- target-scoped execution facts
- transport-neutral blocked-runtime records beyond the current approval-backed subset
- stable output / artifact references
- unified verification scopes
- replay/projection models over those runtime facts

### Not kernel work

- `tenant_id`, `user_id`, `org_id`
- auth and gateway session semantics
- approval UI or operator workflow
- queue topology and worker deployment
- product-specific planner or tool-loop state
- business run IDs or reporting models

## Delivery Shape

The approved delivery shape is incremental:

1. Wave 1: docs, support matrix, capability reason codes, publish hygiene
2. Wave 2: public model layer for targets / blocked-runtime / refs
3. Wave 3: runtime engine for fan-out / tool graph / dataflow / verification scopes
4. Wave 4: stronger durable interactive lifecycle, public interactive controller hooks, and richer projections

The current repository state only claims Wave 1 completion once the corresponding code, tests, and docs land.
