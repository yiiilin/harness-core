# STATUS.md

## Current maturity snapshot

`harness-core` is a **pre-1.0 execution kernel for harness engineering**.

For a fuller maintainers' assessment of current strengths, remaining pure-kernel gaps, and next priorities, see `docs/CURRENT_STATE.md`.

It already has:
- durable `task / session / plan / step` lifecycle contracts
- a governed runtime loop: `plan -> policy -> approval -> execute -> verify -> recover`
- optimistic concurrency for mutable session and approval records
- claim / lease primitives for runnable and recoverable sessions
- abort / cancel semantics
- first-class execution facts: attempts, actions, verifications, artifacts, and runtime handles
- runtime handle lifecycle control
- plan-level capability freeze plus per-action capability snapshots
- context compaction hooks plus durable context summaries
- audit event envelopes with correlation ids
- vendor-neutral metrics and trace exporter hooks
- Postgres-backed repositories and transaction runner wiring
- a public `pkg/harness/postgres` durable bootstrap path
- a public `pkg/harness/worker` helper for claim/renew/run-or-recover/release loops
- a public `pkg/harness/replay` helper for execution-cycle/audit replay projections
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

The focused pure-kernel follow-up plan in `docs/plans/2026-03-20-kernel-purity-followup-execution.md` is now implemented and verified.

That means the current kernel baseline already includes:
- transport-neutral runtime metadata surfaces
- a bare-kernel constructor path separate from builtins composition
- first-class planning / replanning records
- lifecycle-wide observability hooks
- explicit lease heartbeat / expiry / reclaim semantics for runnable and recoverable work
- claim-aware execution, approval resume, and recovery control-plane entrypoints

Remaining work is mainly future expansion, not a known core-boundary defect:
- new capability modules
- new adapters
- stronger product-layer projections outside the kernel

For Postgres-backed embedding, platforms no longer need `internal/postgresruntime`.
The recommended public path is `pkg/harness/postgres`; the WebSocket adapter remains a reference transport layer.
The same applies to migration inspection: use `pkg/harness/postgres`, while `cmd/harness-core migrate ...` is only an ops convenience wrapper.

## Not kernel gaps

The following are intentionally outside `harness-core`:
- user / tenant / org ownership
- auth handshakes and gateway behavior
- session visibility and search projections
- approval UI and operator dashboards
- provider routing, billing, quota, and business policy
- worker fleet orchestration and deployment topology

Those belong in adapters, modules, or an embedding platform.

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

## Current execution plan

Repository-wide roadmap: `docs/ROADMAP.md`

Pure-kernel follow-up plan:
- `docs/plans/2026-03-20-kernel-purity-followup-execution.md`
- `docs/plans/2026-03-20-kernel-claim-closure-execution.md`
