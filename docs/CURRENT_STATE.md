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
- `docs/KERNEL_SCOPE.md`

## Executive Summary

`harness-core` is now best described as a **pre-1.0 execution kernel for harness engineering that is already suitable for embedding into an existing platform**.

It is no longer just a minimal runtime skeleton:

- the core runtime loop is complete enough to support real execution
- durable Postgres-backed embedding is public
- replay/debug reads are public
- reusable worker orchestration is public
- shell PTY execution is extensible enough for external platforms

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
- public durable Postgres bootstrap
- approval / recovery / claimed execution closure
- execution-fact persistence model
- replay/debug read model
- runtime-handle persistence and lifecycle control
- shell PTY execution extensibility
- explicit stability guidance for embedders

## Current Pure-Kernel / Embedder-Facing Gaps

The project no longer has a large kernel-boundary problem.

The remaining gaps are now narrower and mostly about making the embedder surface cleaner and more replaceable.

### 1. Worker helper should depend on a narrow interface

`pkg/harness/worker` currently depends directly on `*runtime.Service`.

That works, but it is a little too concrete for a public helper package.

The next improvement should be:

- define a narrow worker runtime interface
- let the helper depend on that interface instead of the concrete service

This keeps the helper more reusable and more testable without expanding kernel scope.

### 2. Worker helper still stops at `RunOnce()`

The current helper is useful, but platforms still need to write their own outer loop:

- polling
- backoff
- jitter
- shutdown behavior
- worker naming / observability wrapping

The next useful public addition is a small `RunLoop(...)` helper that remains transport-neutral and fleet-neutral.

### 3. Remote PTY execution is replaceable, but PTY inspection is still local-manager-centric

This is the biggest remaining shell-module gap.

Today:

- PTY execution can be delegated through `PTYBackend`
- PTY verification still assumes local `PTYManager` access

That is correct for now, but incomplete for remote PTY platforms.

The next improvement should stay in `modules/shell`, not in the kernel:

- introduce a public PTY inspection / observer abstraction
- allow PTY verifiers to bind to that abstraction

### 4. Adapter-facing protocol guidance can still be stronger

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

### Priority 1: strengthen `pkg/harness/worker`

Do this next:

- narrow the dependency from `*runtime.Service` to a worker-specific interface
- add a small `RunLoop(...)` helper
- keep it transport-neutral and free of fleet/product concepts

This produces the highest embedder value for the lowest scope risk.

### Priority 2: add a PTY inspection abstraction in `modules/shell`

Do this after worker helper cleanup:

- separate PTY execution backend from PTY inspection backend
- allow remote PTY implementations to participate in `pty_*` verification through a public module-level interface

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
