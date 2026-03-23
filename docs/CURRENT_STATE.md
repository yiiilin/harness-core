# CURRENT_STATE.md

## Purpose

This document records the current assessment of `harness-core` after the latest kernel-boundary, embedder-surface, and PTY extensibility work.

It answers four questions:

1. What is the project today?
2. What is already solid?
3. What is still missing from the kernel or embedder-facing surface?
4. What should explicitly stay out of the kernel?

Use this together with:

- `docs/STATUS.md`
- `docs/API.md`
- `docs/EMBEDDING.md`
- `docs/VERSIONING.md`
- `docs/CHANGE_POLICY.md`
- `docs/KERNEL_SCOPE.md`
- `docs/V1_RELEASE_CHECKLIST.md`

## Executive Summary

`harness-core` is now best described as a **pre-1.0 execution kernel for harness engineering that is already suitable for embedding into an existing platform**.

It is no longer just a minimal runtime skeleton:

- the core runtime loop is complete enough to support real execution
- durable Postgres-backed embedding is public
- replay/debug reads are public
- reusable worker orchestration is public
- shell PTY execution is extensible enough for external platforms
- service reads now stay aligned with runner-backed committed state
- runtime budgets start from durable first-runtime activity rather than raw enqueue time
- control-plane mutations are visible in the canonical audit stream

It is still intentionally **not** a complete multi-user product platform.

That is not a maturity gap. It is part of the design boundary.

## What The Project Is

Today, `harness-core` is responsible for:

- task / session / plan / step lifecycle
- governed runtime execution
- policy / approval / resume / recovery flow
- claim / lease / heartbeat / reclaim coordination
- execution facts and runtime-handle persistence
- capability resolution and snapshot persistence
- context assembly / compaction hooks and summaries
- audit envelopes and observability hooks
- durable runtime bootstrap for Postgres
- public embedder helpers around worker loops and replay/debug reads

This is the correct kernel scope.

## What The Project Is Not

The following should remain outside the kernel:

- user / tenant / org ownership
- auth / gateway / session identity
- approval UI, operator workflow, and notifications
- session search / inbox / product projections
- billing, quota, and provider-routing policy
- worker fleet topology and deployment orchestration
- business-specific transport envelopes

If a platform needs these concepts, it should build them around the kernel rather than push them into `pkg/harness/*`.

## Current Maturity Assessment

### Kernel runtime semantics: strong

The kernel already has the important mechanics:

- governed execution loop
- approval gating and resumption
- retry / recover transitions
- lease-aware claimed execution
- durable execution facts
- runtime-handle lifecycle
- replay-friendly audit/correlation metadata

This is enough to call the kernel itself real, not aspirational.

### Durable embedding: strong

`pkg/harness/postgres` is now the correct public path for durable bootstrap.

External platforms no longer need to reach into `internal/postgresruntime` just to get:

- persisted lifecycle state
- persisted approvals
- persisted execution facts
- migrations and schema checks

That is a major maturity milestone.

### Embedder-facing surface: medium-high

The project now has a credible public embedding story:

- `pkg/harness`
- `pkg/harness/postgres`
- `pkg/harness/worker`
- `pkg/harness/replay`
- `docs/EMBEDDING.md`
- `examples/platform-embedding`

This is enough for serious consumers to embed the kernel without carrying the old patch burden in the most important areas.

One important semantic detail is now explicit:

- `runtime.New(...)` installs a local in-memory unit-of-work runner by default over the configured stores
- embedders that want direct-store best-effort behavior must opt into it explicitly by clearing `Service.Runner`

### Extensibility: medium-high

The design direction is now correct:

- kernel-level contracts live in `pkg/harness/*`
- capability variation lives in `modules/*`
- transport exposure lives in `adapters/*`

The shell module is the clearest proof point because PTY execution can now be replaced through `PTYBackend` without forcing a local `PTYManager`.

### Module ecosystem: medium

The module pattern is now clear, but the ecosystem is still small.

This is not a defect in the kernel. It is simply the next area for expansion.

### Adapter ecosystem: medium

The shipped adapters are useful reference implementations, but they are still reference-grade rather than the main stability surface.

That is acceptable as long as the public docs keep making this distinction explicit.

## What Is Solid Enough Today

These areas are now in good shape and should be treated as current strengths:

- kernel state-machine scope and boundaries
- runner/store consistency across read and write paths
- public durable Postgres bootstrap
- approval / recovery / claimed execution closure
- durable runtime-budget anchoring via `runtime_started_at`
- execution-fact persistence model
- replay/debug read model
- control-plane audit coverage for attach / lease / recovery / runtime-handle control
- runtime-handle persistence and lifecycle control
- shell PTY execution extensibility
- explicit stability guidance for embedders
- dedicated release-gate tests for Tier 1 compatibility and durable restart/upgrade paths

## Current Pure-Kernel / Embedder-Facing Gaps

The project no longer has a large kernel-boundary problem.

The remaining gaps are now narrower and mostly about making the embedder surface cleaner and more replaceable.
There is no longer an active tracked correctness checklist in this document for runner/read consistency, runtime-budget anchoring, worker renew cancellation, or control-plane audit visibility; those are now part of the current baseline.

### 1. Worker helper outer-loop ergonomics are intentionally still minimal

`pkg/harness/worker` now depends on a narrow worker-facing runtime interface rather than a concrete `*runtime.Service`.

The core correctness path is in place, including bounded renewal cancellation on `RunOnce()` shutdown.
What remains is optional outer-loop ergonomics around:

- polling
- backoff
- jitter
- shutdown behavior
- worker naming / observability wrapping

The helper now includes a minimal `RunLoop(...)`, but this area is still a good place for careful incremental improvement as long as it remains transport-neutral and fleet-neutral.

Recent improvement:

- the helper now supports additive worker naming, per-iteration observation hooks, and deterministic idle/error backoff without introducing fleet-level concepts

### 2. Remote PTY inspection is improved, but still young

This is the biggest remaining shell-module gap.

Today:

- PTY execution can be delegated through `PTYBackend`
- PTY verification can be delegated through `PTYInspector`

That closes the hard local-manager dependency, but the abstraction is still young and should be exercised further by remote PTY embedders before it can be considered fully proven.

### 3. Adapter-facing protocol guidance can still be stronger

The code now has better kernel/public boundaries than before, but protocol-facing surfaces still deserve tighter documentation around:

- event-stream expectations
- versioning and compatibility
- mapping kernel events/errors into transport contracts

This is not a core-runtime problem. It is an adapter/documentation maturity problem.

## What Should Not Be Mistaken For Kernel Gaps

The following may still be missing from a full platform, but they are not reasons to expand the kernel:

- tenant-aware access models
- user identities and actor models
- UI inboxes and approval consoles
- search and operations dashboards
- business reporting
- provider directories
- organization-level policy UX

These are platform responsibilities.

## Recommended Next Priorities

### Priority 1: keep strengthening `pkg/harness/worker`

Do this next:

- refine `RunLoop(...)` carefully if embedders need more control
- keep it transport-neutral and free of fleet/product concepts
- do not let it grow into fleet orchestration

This produces the highest embedder value for the lowest scope risk.

### Priority 2: harden the PTY inspection abstraction in `modules/shell`

Do this after worker helper cleanup:

- validate the new `PTYInspector` path with more remote-style integrations
- add only the minimum extra surface needed for real remote PTY implementations
- keep PTY inspection in the module layer, not the kernel

This solves the most important remaining shell embedder gap without polluting the kernel.

### Priority 3: tighten adapter-facing guidance

Continue clarifying:

- which adapter surfaces are stable enough to depend on
- how to version protocol changes
- what event/error mapping shape adapters should follow

This is mostly docs plus small public helper cleanup.

### Priority 4: expand ecosystem, not kernel scope

After the above, the best growth path is:

- more modules
- more reference adapters
- more examples

The kernel itself should grow slowly from this point onward.

## Practical Conclusion

The project is now at a good inflection point:

- small enough to remain coherent
- complete enough to embed seriously
- explicit enough about what belongs outside the kernel

The most important discipline now is not adding more concepts into the core.

The next wins come from making the existing kernel easier to embed, easier to replace around the edges, and easier to compose into larger platforms.
