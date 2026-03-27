# EMBEDDER_VNEXT_REALITY_CHECK.md

## Purpose

Record the most concrete current-code assessment of the embedder-vNext execution-model work.

Use this document when maintainers or embedders need a precise answer to:

1. what is already real in code today
2. what is only partially implemented
3. what remains missing from the kernel
4. what should stay out of the kernel entirely

This document is intentionally stricter than roadmap or adaptation plans.
It is a reality check, not a wishlist.

It is also scoped to the current kernel architecture.
Read every vNext item here as an assessment of extensions to the existing
`session + plan + step` runtime, not as evidence that `harness-core` already
ships a first-class durable workflow graph runtime.

Use this together with:

- `docs/API.md`
- `docs/EMBEDDER_VNEXT.md`
- `docs/EMBEDDING.md`
- `docs/CURRENT_STATE.md`
- `docs/KERNEL_SCOPE.md`
- `docs/V1_RELEASE_CHECKLIST.md`

## Reading Rule

Do not treat a public type or public method as proof that the full runtime behavior exists.

For this repository, execution-model maturity must be evaluated in four separate layers:

1. public data model
2. public service API
3. runtime execution semantics
4. release and module-consumption hygiene

A slice is only fully implemented when all relevant layers are actually wired.

## Satisfies Or Basically Satisfies Today

### Public multi-target abstractions

The kernel now has public target contracts:

- `Target`
- `TargetRef`
- `TargetSelection`
- `TargetSelectionMode`
- `TargetFailureStrategy`

This is enough to call the model layer real.

### Public preplanned tool-graph model

The kernel now has public program contracts:

- `Program`
- `ProgramNode`
- `ProgramInputBinding`
- `VerificationScope`

This is enough to call the program/model layer real.

### Public program-execution entry points

The runtime now exposes:

- `CreatePlanFromProgram(...)`
- `RunProgram(...)`

That means preplanned execution is no longer only a document concept.

### Step-to-step dataflow

The runtime now resolves later-step bindings from earlier-step results for:

- structured output
- text output
- bytes output
- artifact refs

Important precision:

- attachment-input contracts are public and usable as input bindings
- the default materializer now supports temp-file materialization for inline text / bytes attachments and artifact-ref payloads
- custom non-empty materialization modes are now delegated to `runtime.AttachmentMaterializer`, and its returned value is passed through to the tool action
- transport-specific cleanup/lifecycle policy still belongs to the configured materializer

### Target-scoped aggregate / replay / projection slices

The kernel now has public and runtime-backed shapes for:

- aggregate summaries
- target slices
- blocked-runtime projections

Replay now derives target slices from persisted target-scoped facts.

### Generic blocked-runtime lifecycle and read model

The kernel now has public blocked-runtime types plus public runtime APIs for both:

- approval-backed current blocked-runtime reads:
  - `GetBlockedRuntime(...)`
  - `GetBlockedRuntimeByApproval(...)`
  - `GetBlockedRuntimeProjection(...)`
  - `GetBlockedRuntimeProjectionByApproval(...)`
- generic blocked-runtime lifecycle and durable lookup:
  - `CreateBlockedRuntime(...)`
  - `RespondBlockedRuntime(...)`
  - `ResumeBlockedRuntime(...)`
  - `AbortBlockedRuntime(...)`
  - `GetBlockedRuntimeByID(...)`
  - `GetBlockedRuntimeRecord(...)`
  - `ListBlockedRuntimeRecords(...)`
- current blocked-runtime listing and projection:
  - `ListBlockedRuntimes()`
  - `ListBlockedRuntimeProjections()`

This is enough to call the blocked-runtime lifecycle and projection surface real.

### Interactive runtime state model

The kernel now has public interactive runtime types and public state/projection APIs:

- `InteractiveRuntime`
- `InteractiveObservation`
- `InteractiveCapabilities`
- `InteractiveController`
- `StartInteractive(...)`
- `ReopenInteractive(...)`
- `ViewInteractive(...)`
- `WriteInteractive(...)`
- `CloseInteractive(...)`
- `GetInteractiveRuntime(...)`
- `ListInteractiveRuntimes(...)`
- `UpdateInteractiveRuntime(...)`
- `UpdateClaimedInteractiveRuntime(...)`

This is enough to call the interactive control plane and state model real.

### Capability matcher and unsupported reason codes

The kernel now has public capability matching with stable reason codes, including:

- `MULTI_TARGET_FANOUT_UNSUPPORTED`
- `PREPLANNED_TOOL_GRAPH_UNSUPPORTED`
- `INTERACTIVE_REOPEN_UNSUPPORTED`
- `ARTIFACT_INPUT_UNSUPPORTED`

This part is real and embedder-usable.

### Attachment / artifact-first input contracts

The kernel now has a public `AttachmentInput` contract with:

- `text`
- `bytes`
- `artifact_ref`

This is real as a public input model.

### Resolver-backed target discovery hook

The runtime now exposes a transport-neutral `TargetResolver` hook.

That makes `TargetSelectionFanoutAll` real for embedders that can supply
concrete targets without teaching the kernel product-specific discovery policy.

## Still Only Partial Today

### Multi-target execution now has a real concurrent fan-out scheduler for native program execution

Current native program fan-out still compiles one logical node into multiple target-scoped steps,
but the runtime no longer executes those sibling steps only through a purely serial step loop.

When a compiled fan-out group carries stable aggregate metadata plus `max_concurrency > 1`, the session driver now runs a scheduler-owned fan-out round that:

- executes ready sibling target steps concurrently
- actually consumes `TargetSelection.MaxConcurrency`
- preserves durable per-target attempts/actions/verifications/artifacts/runtime handles
- preserves current retry / partial-failure / aggregate-verification semantics
- falls back to the serial step path when a round cannot safely run concurrently, such as approval-gated execution

This means the native program fan-out path is now materially real as a concurrent scheduler.

What is still **not** implemented is a broader generic multi-target step model outside the current program/fan-out path.

### Blocked runtime is now generic, but still intentionally session-scoped

The kernel now supports transport-neutral generic blocked-runtime persistence and session-level blocked/unblocked transitions.

That does **not** mean the kernel now owns product semantics such as:

- approval TTL policy
- operator workflow state
- multi-step continuation blobs
- platform-specific resume orchestration

Those remain outside the kernel by design.

### Interactive control plane is transport-neutral in core, not backend-specific

The kernel now exposes a real transport-neutral interactive controller surface for:

- start
- reopen
- view
- write
- close

What still stays outside the kernel is backend-specific behavior such as:

- PTY attach or resize
- stream transport protocol details
- remote backend connection policy

### Attachment materialization is now explicit, while lifecycle policy remains materializer-owned

`AttachmentInput.Materialize` exists as a public contract.

The runtime now provides a real transport-neutral attachment materialization hook for native program execution:

- the default materializer supports temp-file materialization for:
  - inline text attachments
  - inline bytes attachments
  - artifact-ref payloads
- custom non-empty materialization modes are delegated to `runtime.AttachmentMaterializer`
- the materializer return value is injected into the action args without the kernel forcing it to be only a filesystem path

The kernel still does **not** own transport-specific cleanup or lifecycle policy for those materialized values.
That policy remains with the configured materializer by design.

## Still Missing From The Kernel

These are the most important pure-kernel or embedder-surface gaps that remain after the current vNext checklist work:

### 1. Release/module-consumption hygiene for companion modules

This is not a runtime semantics gap, but it is still a real embedder problem.

Current remaining issue:

- root-level or downstream `go mod tidy` can still fail if companion-module tags are not published consistently with the versions referenced by nested modules

That must be treated as a release and module-publishing gap until proven clean.

## Explicit Non-Kernel Areas

The following should still remain outside `harness-core`, even if an embedder needs them:

- `tenant_id`, `user_id`, `org_id`
- auth and gateway/session identity
- approval UI and operator workflow
- run inbox, search, and product projections
- queue topology and worker-fleet orchestration
- billing, quota, or provider-routing policy
- product-specific continuation blobs unless a future generic opaque store is added deliberately

These are platform-layer responsibilities, not kernel gaps.

## Recommended Next Priorities

If maintainers want to close the most meaningful remaining gaps without expanding kernel scope incorrectly, the best next priorities are:

1. upgrade multi-target execution from sequential step expansion to a true concurrent scheduler
2. keep hardening companion-module consumption and publishing hygiene so `@dev` and released companion versions stay externally resolvable
3. clean up companion-module release tags so `go mod tidy` succeeds for downstream users without local exclusions or workarounds

## Bottom Line

The current repository is no longer missing the execution-model *concepts*.

The remaining work is now about closing a few critical semantic gaps:

- true concurrent multi-target scheduling
- generalized attachment materialization
- clean companion-module release hygiene

That is a much narrower and healthier position than the earlier skeleton-stage runtime.
