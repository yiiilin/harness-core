# STATUS.md

## Current maturity snapshot

`harness-core` is a **pre-1.0 execution kernel for harness engineering**.

For the current workflow-runtime wave, maintainers are explicitly continuing on
the existing `session + plan + step` execution architecture. This wave may
extend the current `Program`, approval, blocked-runtime, interactive-handle,
replay, and recovery surfaces, but it does not yet adopt a first-class durable
workflow graph runtime.

For a fuller maintainers' assessment of current strengths, remaining pure-kernel gaps, and next priorities, see `docs/CURRENT_STATE.md`.
For a stricter code-level embedder-vNext implementation matrix, see `docs/EMBEDDER_VNEXT_REALITY_CHECK.md`.
For explicit `v1.0.0` release gates, see `docs/V1_RELEASE_CHECKLIST.md`.
For post-`v1` compatibility rules on the stable path, see `docs/CHANGE_POLICY.md`.

It already has:
- durable `task / session / plan / step` lifecycle contracts
- a governed runtime loop: `plan -> policy -> approval -> execute -> verify -> recover`
- runner-aware reads that resolve through the same effective repository set as runtime writes
- optimistic concurrency for mutable session and approval records
- claim / lease primitives for runnable and recoverable sessions
- total runtime budgeting anchored at first real runtime activity, not raw session creation time
- abort / cancel semantics
- first-class execution facts: attempts, actions, verifications, artifacts, and runtime handles
- runtime handle lifecycle control
- plan-level capability freeze plus per-action capability snapshots
- context compaction hooks plus durable context summaries
- audit event envelopes with correlation ids
- control-plane audit coverage for task attach, lease mutations, recovery state changes, and runtime-handle control
- vendor-neutral metrics and trace exporter hooks
- Postgres-backed repositories and transaction runner wiring
- a public `pkg/harness/postgres` durable bootstrap path
- a public schema-aware `pkg/harness/postgres.Config` bootstrap surface
- a public `pkg/harness/worker` helper for claim/renew/run-or-recover/release loops
- a public `pkg/harness/replay` helper for execution-cycle/audit replay projections
- a public `runtime.TargetResolver` hook for resolver-backed `fanout_all` execution
- scheduler-owned concurrent fan-out rounds that consume `TargetSelection.MaxConcurrency` for native program execution
- a public `runtime.AttachmentMaterializer` hook plus default temp-file materialization
- a public generic blocked-runtime lifecycle with durable blocked-runtime records
- a dedicated `./release` test package for Tier 1 compatibility and durable upgrade/restart gates
- public migration status / pending / drift helpers on `pkg/harness/postgres`
- a public `pkg/harness` embedding facade
- reference capability modules including PTY-backed shell execution
- a minimal platform reference example under `examples/platform-reference`
- a reference WebSocket adapter
- a minimal HTTP reference adapter
- HTTP worker control-plane reference routes for claim / lease / claimed execution
- a durable Postgres multi-worker example

It is not yet a complete product platform.

## Current pure-kernel gap status

The latest hardening pass is tracked in `docs/plans/2026-03-23-kernel-hardening-checklist-execution.md`.

That pass closed the remaining correctness-oriented kernel gaps that were still active at the service/runtime boundary:
- runner-aware reads now follow the same effective repository set as runner-backed writes
- `MaxTotalRuntimeMS` now starts from durable `runtime_started_at`, so queued sessions do not burn runtime budget before planning/execution begins
- worker lease renewal cancellation is bounded when `RunOnce()` finishes
- attach / lease / recovery / runtime-handle control-plane mutations now emit canonical audit events

This does not mean `v1` is ready today.
The remaining blockers are now primarily release-discipline items from `docs/V1_RELEASE_CHECKLIST.md`, not an open pure-kernel correctness checklist.
The active consolidated follow-up checklist now lives in `docs/plans/2026-03-24-master-kernel-gap-checklist.md`.
At the execution-model layer, the generic blocked-runtime lifecycle, transport-neutral interactive control plane, native concurrent program fan-out scheduling, and transport-neutral attachment materialization hook are now part of the baseline; the largest remaining follow-up item is companion-module release hygiene.

For Postgres-backed embedding, platforms no longer need `internal/postgresruntime`.
The recommended public path is `pkg/harness/postgres`, especially `OpenServiceWithConfig(...)` plus `postgres.Config`; the WebSocket adapter remains a reference transport layer.
The same applies to migration inspection: use `pkg/harness/postgres`, while `cmd/harness-core migrate ...` is only an ops convenience wrapper.
`internal/config` remains CLI-private reference wiring, not the embedder API.

## Not kernel gaps

The following are intentionally outside `harness-core`:
- user / tenant / org ownership
- auth handshakes and gateway behavior
- session visibility and search projections
- approval UI and operator dashboards
- provider routing, billing, quota, and business policy
- worker fleet orchestration and deployment topology

Those belong in adapters, modules, or an embedding platform.
The same currently applies to opaque continuation blobs for platform-specific loop resume state.
Interactive backend implementations such as PTY attach/resize or other transport-specific stream behavior remain outside the kernel; the kernel now owns the transport-neutral interactive controller contract plus runtime-handle state and replay facts.

## Best use today

Use it for:
- embedding a small execution kernel inside a larger agent system
- experimenting with governed tool execution and recovery semantics
- building capability modules against stable-enough runtime contracts
- studying a minimal claim/lease worker loop in `examples/platform-reference`
- refining replay, audit, and observability patterns through `pkg/harness/replay`

Do not assume it is a complete multi-user product platform by itself.

## Embedder-facing status

For existing-platform integration, the recommended path is now documented as:
- kernel facade and control-plane APIs in `docs/API.md`
- durable bootstrap in `pkg/harness/postgres`
- reusable worker loop in `pkg/harness/worker`
- replay/debug projection in `pkg/harness/replay`
- integration patterns in `docs/EMBEDDING.md`
- stability tiers in `docs/VERSIONING.md`
- post-`v1` compatibility policy in `docs/CHANGE_POLICY.md`

Consistency rule for embedders:
- `runtime.New(...)` installs an in-memory unit-of-work runner over the configured stores by default
- when `runtime.Options.Runner` is present, runtime writes execute against the runner repository set
- public getters/listers and internal runtime read helpers resolve through that same effective repository set, falling back to service stores only for repositories the runner does not override
- embedders should prefer service read APIs over mixing direct store reads from partially overridden repositories
- explicitly clearing `Service.Runner` opts into direct-store, best-effort event semantics and is mainly useful for tightly scoped local embeddings or tests

## Current execution plan

Repository-wide roadmap: `docs/ROADMAP.md`

Pure-kernel follow-up plan:
- `docs/plans/2026-03-20-kernel-purity-followup-execution.md`
- `docs/plans/2026-03-20-kernel-claim-closure-execution.md`
- `docs/plans/2026-03-23-kernel-hardening-checklist-execution.md`
