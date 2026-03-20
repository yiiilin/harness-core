# Postgres Migrations, Durable Workers, And HTTP Adapter Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** harden platform-facing integration beyond the bare kernel by adding a versioned public Postgres migration contract, a durable multi-worker reference example, and a second minimal reference adapter without expanding kernel scope.

**Architecture:** keep the kernel unchanged unless a transport-neutral execution contract is missing. Postgres migration/versioning belongs in the public `pkg/harness/postgres` bootstrap layer plus `internal/postgres` storage internals. The durable worker sample belongs in `examples/*`. The second transport stays in `adapters/*` and must consume the same public runtime surfaces rather than inventing adapter-specific runtime semantics.

**Tech Stack:** Go 1.24, `pkg/harness/postgres`, `internal/postgres`, `examples/*`, `adapters/*`, Go tests, Markdown docs.

---

## Execution Rule

Progress must be tracked in this file only:

- Start every pending task as unchecked
- Mark each task with `[x]` immediately after implementation and verification complete
- If a task reveals a prerequisite gap, add a new unchecked task directly below it before continuing
- If any task remains unchecked, the project is not complete

## Scope Guardrails

This work is in scope:
- public Postgres migration/version metadata and bootstrap behavior
- a durable multi-worker reference example using claim/lease against Postgres
- a second minimal reference adapter that stays outside the kernel
- docs that steer platforms toward public integration surfaces

This work is out of scope:
- user / tenant / auth / RBAC / queue / scheduler concepts in `pkg/harness/*`
- product dashboards, approval UIs, or search/reporting projections
- changing kernel runtime semantics unless a transport-neutral correctness gap is discovered
- adapter-specific business policy or provider routing logic

### Task 1: Add a public migration/versioning contract for Postgres bootstrap

**Files:**
- Modify: `pkg/harness/postgres/*`
- Modify: `internal/postgres/*`
- Modify: `internal/postgresruntime/runtime.go`
- Modify: `internal/postgrestest/postgrestest.go`
- Test: `pkg/harness/postgres/*_test.go`
- Test: `internal/postgres/*_test.go`

- [x] Add failing tests for public migration application, schema version introspection, and idempotent re-apply behavior.
- [x] Replace the current monolithic schema-only apply path with a versioned migration path backed by embedded migration files.
- [x] Expose a minimal public versioning contract on `pkg/harness/postgres` such as apply-migrations plus current/latest version lookup.
- [x] Keep `ApplySchema(...)` as a compatibility wrapper only if needed; the canonical path should become migration-driven.
- [x] Re-run the focused Postgres bootstrap and storage tests until they pass.

### Task 2: Add a durable multi-worker reference example

**Files:**
- Create: `examples/postgres-workers/*`
- Modify: existing shared example docs as needed
- Test: `examples/postgres-workers/*_test.go`

- [x] Add failing tests for a Postgres-backed example where multiple workers contend for runnable work and only one wins each claim.
- [x] Implement a platform-neutral example that boots the runtime through `pkg/harness/postgres`, seeds work, and runs multiple worker loops against the same DB.
- [x] Show lease renewal, release, and recovery-safe behavior without introducing queue or tenant concepts.
- [x] Re-run the example tests until they pass.

### Task 3: Add a second minimal reference adapter

**Files:**
- Create: `adapters/http/*`
- Create/Modify: `internal/protocol/*` only if strictly shared and transport-neutral
- Test: `adapters/http/*_test.go`
- Modify: docs that enumerate reference adapters

- [x] Add failing tests for a minimal HTTP adapter that exposes a small public runtime control surface.
- [x] Implement the adapter as thin transport glue over existing runtime methods; do not duplicate runtime state machines in the adapter.
- [x] Keep the surface intentionally narrow and reference-grade: health/info plus a minimal lifecycle/run path is enough.
- [x] Re-run the adapter tests until they pass.

### Task 4: Update integration docs and close out

**Files:**
- Modify: `README.md`
- Modify: `docs/API.md`
- Modify: `docs/API.zh-CN.md`
- Modify: `docs/STATUS.md`
- Modify: `docs/PERSISTENCE.md`
- Modify: `docs/plans/2026-03-20-postgres-migrations-workers-http-execution.md`

- [x] Document the migration/versioning contract as part of the public Postgres bootstrap path.
- [x] Document the durable worker example and second adapter as reference integration surfaces outside the kernel.
- [x] Re-run focused verification for the touched packages/examples/adapters.
- [x] Run `go test ./... -count=1`.
- [x] Mark completed tasks `[x]` only after fresh verification and leave no unchecked items.
