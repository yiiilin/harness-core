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
| Approval-backed blocked-runtime lookup/projection | Supported | Public reads exist for `session_id` / `approval_id` lookup and blocked-runtime listing. This is the current subset, not a generic blocking engine. |
| Execution target public contracts | Partial | Typed execution-target contracts are public and the runtime now consumes explicit declared targets for minimal program fan-out. Generalized target discovery and broader multi-target step semantics remain unimplemented. |
| Attachment / artifact input public contracts | Supported as model layer | Typed attachment-input contracts are public, but the runtime does not yet consume them as kernel-native input semantics. |
| Output / artifact / attachment reference contracts | Partial | Typed refs are public and the runtime now resolves structured/text/bytes output refs plus artifact refs for native program execution. Broader attachment materialization and generalized dataflow remain unimplemented. |
| Preplanned execution-program / tool-graph contracts | Supported | Typed program/node/input-binding contracts are public and the runtime now exposes minimal single-target program execution entry points. |
| Target-slice / richer blocked-runtime projection contracts | Partial | Typed projection contracts are public; target slices are runtime-backed, and approval-backed blocked-runtime projections are now populated through public projection reads plus replay. Generic blocked-runtime progression remains unimplemented. |
| Generic blocked-runtime contract types | Supported as model layer | Typed durable blocked-runtime record/condition contracts are public, but runtime APIs still expose only the approval-backed subset. |
| Durable interactive runtime handles | Partial | Runtime handles, typed interactive projections, and interactive-state updates are public now. Actual reopen/view/write/close backends remain embedder or module concerns. |
| PTY backend replacement | Supported in companion module | `modules/shell` exposes `PTYBackend` and `PTYInspector`; this is companion-module surface, not kernel core. |
| Replay/debug execution cycles | Supported | `pkg/harness/replay` projects execution cycles plus audit events. |
| Capability unsupported reason codes | Supported | Public capability matching can return stable unsupported reason codes. |
| Native multi-target fan-out scheduler | Partial | Explicit program fan-out across declared targets is supported through native program execution. Explicit `on_partial_failure=continue`, per-target retries, and aggregate result summaries are now supported for this path. `fanout_all` discovery, concurrency controls, and broader multi-target step execution remain unimplemented. |
| Native preplanned non-shell tool graph | Partial | `CreatePlanFromProgram(...)`, `RunProgram(...)`, `ListAggregateResults(...)`, and `execution.AttachProgram(...)` execute dependency-ordered programs with explicit target fan-out, per-target retry/continue semantics, structured/artifact dataflow, and aggregate result derivation. Broader attachment bindings and generalized graph semantics remain unimplemented. |
| Stable step-to-step output / artifact refs | Partial | Structured/text/bytes output refs and artifact refs now resolve natively for program execution. Generalized attachment/dataflow semantics outside that path remain unimplemented. |
| First-class blocked runtime beyond approval-shaped pause | Planned vNext | Not implemented yet. |
| Unified verification across graph / fan-out / interactive | Partial | `ExecutionProgramNode.VerifyScope` now supports step/target/aggregate verification across native program execution. Interactive-specific verification scope remains unimplemented. |
| Kernel-native attachment / artifact input model | Planned vNext | Not implemented yet. |
| Target-sliced replay / projection views | Partial | Replay now groups target-scoped facts into target slices, derives interactive runtimes from runtime handles, and includes approval-backed blocked-runtime projections. Generic blocked-runtime progression remains unimplemented. |

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

Current blocked-runtime subset semantics:

- lookup is restart-safe by `session_id` and `approval_id`
- listing is stably ordered by `requested_at` ascending with `blocked_runtime_id` tie-break
- richer approval-backed projection reads are now available through:
  - `GetBlockedRuntimeProjection(...)`
  - `GetBlockedRuntimeProjectionByApproval(...)`
  - `ListBlockedRuntimeProjections()`
  - `pkg/harness/replay.SessionProjection.BlockedRuntimes`
- this is a projection over approval-backed blocked state, not a new generic blocked-runtime state machine

Current output/artifact/attachment reference semantics:

- `ExecutionArtifactRef` and `ExecutionAttachmentRef` are stable typed references for persisted artifacts and typed inputs
- `ExecutionOutputRef` can point at prior structured/text/bytes output plus artifact/attachment-backed values
- the runtime now resolves structured/text/bytes output refs and artifact refs for native program execution
- broader attachment-backed materialization semantics remain planned vNext work

Current preplanned program/tool-graph semantics:

- `ExecutionProgram` is a transport-neutral container for future preplanned execution graphs
- `ExecutionProgramNode` carries a generic tool `action`, optional `verify`, optional `verify_scope`, optional `on_fail`, optional target selection, dependency edges, and stable input bindings
- `ExecutionProgramInputBinding` can carry literal values, `OutputRef` references, or `AttachmentInput` values
- the runtime now exposes `CreatePlanFromProgram(...)`, `RunProgram(...)`, and `execution.AttachProgram(step, program)` for plan-embedded compilation
- current native execution is intentionally minimal:
  - explicit target fan-out from declared `Targeting.Targets`
  - dependency-ordered execution through the existing plan/session loop
  - literal bindings plus structured/text/bytes output refs and artifact refs
  - per-target retries through `ProgramNode.OnFail`
  - partial-failure continuation through `TargetSelection.OnPartialFailure=continue`
  - aggregate result summaries through `SessionRunOutput.Aggregates` and `ListAggregateResults(...)`
  - verification scopes through `ProgramNode.VerifyScope`
    - `step` for ordinary single-step execution
    - `target` for per-target fan-out verification
    - `aggregate` for explicit fan-out summary verification when the group resolves
- broader attachment bindings, interactive verification scopes, and broader multi-target policy semantics remain planned vNext work

Current target-slice / blocked-runtime projection semantics:

- `ExecutionTargetSlice` is the public value shape for future target-scoped execution grouping
- `ExecutionBlockedRuntimeProjection` and `ExecutionBlockedRuntimeWait` are the public value shapes for richer blocked-runtime views
- `pkg/harness/replay.SessionProjection` and `pkg/harness/replay.ExecutionCycleProjection` populate target slices when execution facts carry stable target metadata
- `pkg/harness/replay.ExecutionCycleProjection` now also derives `InteractiveRuntimes` from persisted runtime handles
- approval-backed blocked-runtime projection fields are now populated through the public projection reads and replay helper
- broader generic blocked-runtime progression facts still do not exist

Current interactive runtime semantics:

- `ExecutionInteractiveRuntime` is the public projection shape over a persisted runtime handle plus stable interactive metadata
- the runtime now exposes:
  - `GetInteractiveRuntime(...)`
  - `ListInteractiveRuntimes(...)`
  - `UpdateInteractiveRuntime(...)`
  - `UpdateClaimedInteractiveRuntime(...)`
- this lets embedders persist reopen/view/write/close projection state in a transport-neutral way
- actual interactive control APIs remain outside the kernel and stay in companion modules or embedding layers

Current native fan-out semantics:

- `CreatePlanFromProgram(...)` and `RunProgram(...)` support explicit target fan-out from `ExecutionProgramNode.Targeting.Targets`
- fan-out is currently compiled into ordered target-scoped plan steps and reuses the existing durable plan/session loop
- target-scoped facts are persisted through stable target metadata on attempts/actions/verifications/artifacts/runtime handles
- explicit fan-out groups also persist stable aggregate metadata so the runtime can derive aggregate result summaries durably
- `RunProgram(...)` returns aggregate summaries on `SessionRunOutput.Aggregates`
- `ListAggregateResults(sessionID)` exposes the current aggregate view from the latest durable plan
- `TargetSelection.OnPartialFailure=continue` now means:
  - each target step still respects retry budgets independently
  - exhausted failed targets no longer block the rest of the explicit fan-out group
  - the logical fan-out group still fails when every target exhausts as failed
- `TargetSelectionFanoutAll` remains unsupported because target discovery strategy stays outside the kernel
- aggregate verification is now supported for explicit program fan-out groups through `ProgramNode.VerifyScope=aggregate`

Current generic blocked-runtime contract semantics:

- `ExecutionBlockedRuntimeRecord` is the public durable record shape for future non-approval-only blocked-runtime persistence
- `ExecutionBlockedRuntimeSubject` identifies the waiting step/action/target locus without assuming approval-specific fields
- `ExecutionBlockedRuntimeCondition` and `ExecutionBlockedRuntimeConditionKind` describe the external condition in a transport-neutral way
- current runtime APIs still expose only the approval-backed `BlockedRuntime` subset; these generic types are not yet populated by runtime writes

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
4. Wave 4: stronger durable interactive lifecycle and richer projections

The current repository state only claims Wave 1 completion once the corresponding code, tests, and docs land.
