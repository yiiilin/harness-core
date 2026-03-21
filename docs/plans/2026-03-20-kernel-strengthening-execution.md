# Kernel Strengthening Execution Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** strengthen the pure kernel around execution recovery semantics, replay/debug stability, and transport-neutral operational contracts without introducing platform-layer concepts.

**Architecture:** keep all work inside `pkg/harness/*` unless a durable storage or reference-surface update is required. The kernel continues to own execution correctness, recovery correctness, replay/debug stability, and transport-neutral runtime semantics. No user/tenant/auth/UI/worker-fleet concepts may enter kernel types.

**Tech Stack:** Go, `pkg/harness/*`, `internal/postgres/*` when storage changes are required, docs, Go tests.

---

## Execution Rule

Progress must be tracked in this file only:

- Start every pending task as unchecked
- Mark each task with `[x]` immediately after implementation and verification complete
- If a task reveals a prerequisite gap, add a new unchecked task directly below it before continuing
- If any task remains unchecked, the project is not complete

## Boundary Guardrails

The kernel owns:
- execution loop correctness
- retry / reinspect / replan / backoff semantics
- replay-stable execution facts
- runtime-handle lifecycle safety
- capability freeze / replay stability
- transport-neutral error semantics
- deterministic, testable time-sensitive runtime behavior

The kernel does not own:
- user / tenant / org identity
- auth / gateway / UI workflows
- queue topology or worker-fleet metadata
- transport request/response envelopes
- provider-routing or product policy

### Task 1: Complete `OnFail` runtime semantics

**Files:**
- Modify: `pkg/harness/runtime/budgets.go`
- Modify: `pkg/harness/runtime/runner.go`
- Modify: `pkg/harness/runtime/session_driver.go`
- Modify: `pkg/harness/runtime/errors.go`
- Modify: `docs/RUNTIME.md`
- Modify: `docs/PROTOCOL.md`
- Test: `pkg/harness/runtime/*_test.go`

- [x] Add failing tests for `reinspect` behavior, persisted `backoff_ms`, and session-driver handling when retry backoff is active.
- [x] Implement transport-neutral runtime semantics for `retry`, `reinspect`, `replan`, and `abort`, including durable retry-not-before metadata.
- [x] Re-run focused runtime tests until they pass.

### Task 2: Strengthen execution cycle records

**Files:**
- Modify: `pkg/harness/execution/*`
- Modify: `pkg/harness/runtime/*`
- Modify: `internal/postgres/executionrepo/*`
- Modify: `internal/postgres/schema*`
- Modify: `docs/RUNTIME.md`
- Modify: `docs/PROTOCOL.md`
- Test: `pkg/harness/runtime/*_test.go`
- Test: `internal/postgres/executionrepo/*_test.go`

- [x] Add failing tests that prove one logical execution cycle can be reasoned about coherently across approval, action, verification, and recovery.
- [x] Introduce richer attempt/cycle metadata without adding transport or worker identity concepts.
- [x] Re-run focused execution-record tests until they pass.

### Task 3: Tighten runtime-handle governance

**Files:**
- Modify: `pkg/harness/runtime/runtime_handles.go`
- Modify: `pkg/harness/execution/*`
- Modify: `docs/RUNTIME.md`
- Test: `pkg/harness/runtime/runtime_handle*_test.go`

- [x] Add failing tests for stale or invalid runtime-handle mutations under competing execution/recovery conditions.
- [x] Enforce stronger kernel-side invariants for active/closed/invalidated runtime handles.
- [x] Re-run focused runtime-handle tests until they pass.

### Task 4: Strengthen capability freeze replay guarantees

**Files:**
- Modify: `pkg/harness/capability/*`
- Modify: `pkg/harness/runtime/capability_freeze.go`
- Modify: `docs/RUNTIME.md`
- Modify: `docs/PROTOCOL.md`
- Test: `pkg/harness/runtime/capability*_test.go`

- [x] Add failing tests for stronger capability-view replay validation and drift detection.
- [x] Introduce replay-stable capability-view identity/validation that remains transport-neutral.
- [x] Re-run focused capability-freeze tests until they pass.

### Task 5: Add transport-neutral runtime error taxonomy

**Files:**
- Modify: `pkg/harness/runtime/*`
- Modify: `pkg/harness/session/*`
- Modify: `pkg/harness/approval/*`
- Modify: `pkg/harness/execution/*`
- Modify: `docs/API.md`
- Modify: `docs/PROTOCOL.md`
- Test: `pkg/harness/runtime/*_test.go`

- [x] Add failing tests for retryable/conflict/not-found/budget/lease/runtime-handle error classification.
- [x] Expose a small typed error taxonomy that adapters can map without inventing kernel semantics.
- [x] Re-run focused error-contract tests until they pass.

### Task 6: Introduce a replaceable kernel clock

**Files:**
- Modify: `pkg/harness/runtime/*`
- Modify: `pkg/harness/session/*`
- Modify: `pkg/harness/postgres/*` only if required by tests/docs
- Modify: `docs/RUNTIME.md`
- Test: `pkg/harness/runtime/*_test.go`

- [x] Add failing tests that freeze/advance time for budget, backoff, lease, and runtime-handle lifecycle behaviors.
- [x] Introduce a small replaceable clock contract and route time-sensitive kernel behavior through it.
- [x] Re-run focused time-sensitive runtime tests until they pass.

### Task 7: Verification and boundary review

**Files:**
- Modify: `docs/plans/2026-03-20-kernel-strengthening-execution.md`

- [x] Re-read every completed task and confirm no user/tenant/auth/UI/transport/worker-fleet concepts entered `pkg/harness/*`.
- [x] Run `go test ./pkg/harness/... -count=1`.
- [x] Run `go test ./... -count=1`.
- [x] Mark all completed tasks in this file and leave no unchecked items.
