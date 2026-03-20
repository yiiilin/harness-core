# Claimed Lease Renewal Coordination Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** make claimed execution and claimed recovery survive lease heartbeat renewals during in-flight work without weakening normal session CAS conflict semantics.

**Architecture:** keep the fix inside `pkg/harness/runtime`. The runtime should add a lease-aware session-update retry path that is only active for claimed execution paths (`leaseID != ""`). Ordinary non-claimed session updates must continue surfacing version conflicts instead of silently retrying. Examples should then demonstrate background renewal safely.

**Tech Stack:** Go 1.24, `pkg/harness/runtime`, `examples/postgres-workers`, Go tests, Markdown docs.

---

## Execution Rule

Progress must be tracked in this file only:

- Start every pending task as unchecked
- Mark each task with `[x]` immediately after implementation and verification complete
- If a task reveals a prerequisite gap, add a new unchecked task directly below it before continuing
- If any task remains unchecked, the project is not complete

### Task 1: Lock the claimed-renewal conflict with tests

**Files:**
- Modify: `pkg/harness/runtime/claim_execution_test.go`

- [x] Add a failing test that runs a claimed session, renews its lease while the tool is still executing, and asserts the run completes successfully.
- [x] Keep the test deterministic by controlling when the tool starts and when it is released.

### Task 2: Add a lease-aware runtime session update helper

**Files:**
- Create: `pkg/harness/runtime/session_update.go`
- Modify: `pkg/harness/runtime/runner.go`
- Modify: `pkg/harness/runtime/recovery.go`

- [x] Add a helper that retries session persistence only for claimed paths and only when the conflict is on the session version.
- [x] Merge refreshed lease heartbeat fields into the desired session state so renewals are preserved instead of being clobbered by stale state writes.
- [x] Keep unclaimed paths on the original CAS semantics so ordinary concurrency conflicts still surface.

### Task 3: Re-enable background renewal in the durable worker example

**Files:**
- Modify: `examples/postgres-workers/main.go`
- Modify: `examples/postgres-workers/README.md`

- [x] Switch the Postgres worker example back to a background renewal loop during execution.
- [x] Keep the example transport-neutral and limited to claim/renew/run/release behavior.

### Task 4: Verification and closeout

**Files:**
- Modify: `docs/plans/2026-03-20-claimed-lease-renewal-execution.md`

- [x] Run focused verification for claimed renewal and the worker example.
- [x] Run `go test ./... -count=1`.
- [x] Mark completed tasks `[x]` only after fresh verification.
- [x] Leave no unchecked items.
