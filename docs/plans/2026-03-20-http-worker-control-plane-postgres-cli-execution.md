# HTTP Worker Control Plane And Postgres CLI Execution Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** finish the remaining platform-facing integration work by exposing worker claim/lease/run flows through the HTTP reference adapter and by turning the public Postgres bootstrap surface into an inspectable, operable migration contract with a small CLI entrypoint.

**Architecture:** keep runtime semantics in `pkg/harness/runtime`; the HTTP adapter remains thin transport glue over existing service methods. Migration metadata and inspection stay in `pkg/harness/postgres`. The CLI belongs in `cmd/harness-core` and must consume the public Postgres package instead of `internal/*`.

**Tech Stack:** Go 1.24, `adapters/http`, `pkg/harness/postgres`, `cmd/harness-core`, Go tests, Markdown docs.

---

## Execution Rule

Progress must be tracked in this file only:

- Start every pending task as unchecked
- Mark each task with `[x]` immediately after implementation and verification complete
- If a task reveals a prerequisite gap, add a new unchecked task directly below it before continuing
- If any task remains unchecked, the project is not complete

## Scope Guardrails

This work is in scope:
- HTTP reference routes for worker claim, lease renewal/release, and claimed execution flows
- public Postgres migration listing, pending status, and drift/version inspection
- a minimal CLI surface for migration status, apply, and version inspection
- docs that keep these capabilities outside the kernel while making them easy for platforms to consume

This work is out of scope:
- adding queue, scheduler, tenant, user, auth, or deployment concepts into the kernel
- growing the HTTP adapter into a product API or long-lived compatibility contract
- introducing adapter-specific runtime state machines
- embedding Nidus-specific bootstrap logic into public packages

### Task 1: Expose worker control-plane routes in the HTTP adapter

**Files:**
- Modify: `adapters/http/server.go`
- Modify: `adapters/http/server_test.go`
- Modify: `adapters/http/README.md`

- [x] Add failing tests for claiming runnable/recoverable sessions, renewing/releasing leases, and running/recovering/resuming claimed sessions over HTTP.
- [x] Implement the HTTP routes as thin wrappers over existing runtime service methods with simple JSON request/response types.
- [x] Re-run the focused HTTP adapter tests until they pass.

### Task 2: Expose public Postgres migration inspection helpers

**Files:**
- Modify: `pkg/harness/postgres/*`
- Test: `pkg/harness/postgres/*_test.go`

- [x] Add failing tests for listing embedded migration status, pending migrations, and schema drift/currentness.
- [x] Implement public migration inspection helpers without leaking `internal/postgres` details into downstream callers.
- [x] Re-run the focused Postgres package tests until they pass.

### Task 3: Add a minimal migration CLI surface

**Files:**
- Modify: `cmd/harness-core/main.go`
- Test: `cmd/harness-core/*_test.go`

- [x] Add failing tests for `harness-core migrate status`, `harness-core migrate up`, and `harness-core migrate version`.
- [x] Implement the CLI so it uses the public Postgres package and preserves the current default server startup path.
- [x] Re-run the focused CLI tests until they pass.

### Task 4: Update docs and verify end to end

**Files:**
- Modify: `README.md`
- Modify: `docs/API.md`
- Modify: `docs/API.zh-CN.md`
- Modify: `docs/PERSISTENCE.md`
- Modify: `docs/STATUS.md`
- Modify: `docs/plans/2026-03-20-http-worker-control-plane-postgres-cli-execution.md`

- [x] Document the new HTTP worker control-plane reference routes and the public migration inspection/CLI surface.
- [x] Run focused verification for `adapters/http`, `pkg/harness/postgres`, and `cmd/harness-core`.
- [x] Run `go test ./... -count=1`.
- [x] Mark completed tasks `[x]` only after fresh verification and leave no unchecked items.
