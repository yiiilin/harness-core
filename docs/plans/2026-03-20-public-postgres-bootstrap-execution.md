# Public Postgres Bootstrap Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** publish a stable, public Postgres-backed durable bootstrap path under `pkg/harness/*` so embedding platforms can start a precise persisted runtime without importing `internal/*` or copying wiring code.

**Architecture:** expose a small public package dedicated to Postgres runtime bootstrap. It should wrap the existing repository wiring, schema application, and transactional runtime options assembly while keeping kernel scope intact. `internal/postgresruntime` may remain as a compatibility shim, but the recommended path must move to a public package and example/docs.

**Tech Stack:** Go 1.24, `pkg/harness/postgres`, `pkg/harness/runtime`, `internal/postgres/*` repositories, Go tests, `examples/*`, Markdown docs.

---

## Execution Rule

Progress must be tracked in this file only:

- Start every pending task as unchecked
- Mark each task with `[x]` immediately after implementation and verification complete
- If a task reveals a prerequisite gap, add a new unchecked task directly below it before continuing
- If any task remains unchecked, the project is not complete

## Scope Guardrails

This work is in scope:
- a public Postgres bootstrap package under `pkg/harness/*`
- public schema/bootstrap helpers needed by embedding platforms
- example embedding code showing the public durable path
- documentation that points platforms at the public API instead of `internal/*`

This work is out of scope:
- adding tenant/user/auth/platform concepts to the kernel
- making `adapters/websocket` the primary integration surface
- encoding transport-specific bootstrapping into the public runtime API
- changing runtime semantics unrelated to durable bootstrap

### Task 1: Add a dedicated public bootstrap package

**Files:**
- Create: `pkg/harness/postgres/*`
- Modify: `internal/postgresruntime/runtime.go`
- Modify: `cmd/harness-core/main.go`
- Test: `pkg/harness/postgres/*_test.go`

- [x] Create a new public package under `pkg/harness/postgres` that exposes durable bootstrap helpers for opening a DB, applying schema, building runtime options, and opening a runtime service.
- [x] Keep the public API independent from `internal/*` types in its signatures where possible.
- [x] Repoint the CLI wiring to the new public package.
- [x] Leave `internal/postgresruntime` as a thin compatibility layer or wrapper instead of the canonical implementation.

### Task 2: Lock public bootstrap behavior with tests

**Files:**
- Create/Modify: `pkg/harness/postgres/*_test.go`
- Modify: `internal/postgrestest/postgrestest.go`
- Modify: existing Postgres integration tests that import `internal/postgresruntime`

- [x] Add focused tests proving the public package can open/apply/build/open with the same behavior as the current internal bootstrap.
- [x] Update repo-local Postgres test helpers and integration tests to consume the public bootstrap path where appropriate.
- [x] Run the targeted tests and verify the new public surface is exercised directly.

### Task 3: Add a public durable embedding example

**Files:**
- Create: `examples/postgres-embedded/*`
- Test: `examples/postgres-embedded/*_test.go`

- [x] Add an example that uses `pkg/harness/postgres` plus `pkg/harness/builtins` to start a durable runtime and execute a minimal path.
- [x] Keep the example platform-neutral: no WebSocket server, no UI, no tenant/user concerns.
- [x] Add example verification so the public bootstrap path is covered by tests.

### Task 4: Make the new integration path explicit in docs

**Files:**
- Modify: `README.md`
- Modify: `docs/API.md`
- Modify: `docs/API.zh-CN.md`
- Modify: `docs/PACKAGE_BOUNDARIES.md`
- Modify: `docs/STATUS.md`
- Modify: `internal/postgres/README.md`
- Modify: `docs/plans/2026-03-20-public-postgres-bootstrap-execution.md`

- [x] Document `pkg/harness/postgres` as the recommended durable Postgres integration path for embedding platforms.
- [x] Clarify that `internal/*` and the reference WebSocket adapter are not the required public integration surface.
- [x] Update package-boundary and status docs so the public/bootstrap split is obvious to maintainers and downstream consumers.

### Task 5: Verification and closeout

**Files:**
- Modify: `docs/plans/2026-03-20-public-postgres-bootstrap-execution.md`

- [x] Run focused verification for the new public package and example.
- [x] Run `go test ./... -count=1`.
- [x] Mark completed tasks `[x]` only after fresh verification.
- [x] Re-read this plan file and ensure no unchecked item remains.
