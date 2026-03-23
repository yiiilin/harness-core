# Kernel Hardening Checklist Execution Plan

> Progress must be tracked in this file only. Every task starts unchecked and is marked `[x]` only after code, tests, and docs are complete.

**Goal:** close the remaining pure-kernel correctness and hardening gaps without expanding kernel scope into platform concerns.

**Non-goals:** tenant/user/org identity, auth, UI workflows, transport-specific product behavior, provider routing, billing, or fleet orchestration.

## Execution Rules

- Start every task as unchecked.
- Mark a task `[x]` only after implementation and verification complete.
- If a task reveals a prerequisite gap, add it directly below the current task before continuing.
- If any task remains unchecked, the checklist is not complete.

## Task 1: Close runner-aware read path gaps

**Goal:** make committed runtime state readable through the same effective repository set that write paths already use.

**Files:**
- Modify: `pkg/harness/runtime/service.go`
- Modify: `pkg/harness/runtime/planning.go`
- Modify: `pkg/harness/runtime/approval_flow.go`
- Modify: `pkg/harness/runtime/recovery.go`
- Modify: `pkg/harness/runtime/capability_freeze.go`
- Modify: `pkg/harness/runtime/execution_cycle_reads.go`
- Modify: `pkg/harness/runtime/runner.go`
- Test: `pkg/harness/runtime/*_test.go`

- [x] Add runner-aware read helpers for public getters/listers and internal helper reads.
- [x] Switch planner, approval, recovery, capability-freeze, and replay/execution-cycle read paths to the runner-aware read helpers.
- [x] Add mixed-store regression coverage proving runtime reads reflect runner-committed state rather than stale service stores.

## Task 2: Make total runtime budget anchoring explicit and durable

**Goal:** prevent queued-but-not-yet-running sessions from burning `MaxTotalRuntimeMS` before real runtime activity begins.

**Files:**
- Modify: `pkg/harness/session/state.go`
- Modify: `pkg/harness/session/store.go`
- Modify: `pkg/harness/runtime/budgets.go`
- Modify: `pkg/harness/runtime/planning.go`
- Modify: `pkg/harness/runtime/runner.go`
- Modify: `pkg/harness/runtime/recovery.go`
- Modify: `internal/postgres/schema.sql`
- Modify: `internal/postgres/sessionrepo/repo.go`
- Create: `internal/postgres/migrations/0005_session_runtime_started_at.sql`
- Test: `pkg/harness/runtime/*budget*_test.go`
- Test: `internal/postgres/sessionrepo/*_test.go`

- [x] Add a minimal durable session field for the runtime budget anchor.
- [x] Initialize the anchor on first real runtime activity and enforce `MaxTotalRuntimeMS` against that anchor.
- [x] Add regression coverage for queue-before-run, direct step execution, planning-first execution, and durable restart behavior.

## Task 3: Fix worker renewal cancellation semantics

**Goal:** make lease renewal cancellation actually cancel blocked renew calls so worker shutdown is bounded.

**Files:**
- Modify: `pkg/harness/worker/worker.go`
- Test: `pkg/harness/worker/worker_test.go`

- [x] Use the renewal-specific context when calling `RenewSessionLease(...)`.
- [x] Add regression coverage proving a blocked renew call is canceled when `RunOnce()` finishes or the worker context ends.

## Task 4: Fill control-plane audit gaps

**Goal:** ensure non-step control-plane mutations are visible in the canonical audit stream and replay/debug surfaces.

**Files:**
- Modify: `pkg/harness/audit/event.go`
- Modify: `pkg/harness/runtime/lifecycle.go`
- Modify: `pkg/harness/runtime/coordination.go`
- Modify: `pkg/harness/runtime/recovery.go`
- Modify: `pkg/harness/runtime/runtime_handles.go`
- Modify: `docs/EVENTS.md`
- Modify: `docs/PROTOCOL.md`
- Test: `pkg/harness/runtime/*event*_test.go`
- Test: `pkg/harness/runtime/*handle*_test.go`

- [x] Add canonical audit event types for session-task attach, lease claim/renew/release, recovery state mutations, and runtime-handle lifecycle control.
- [x] Emit those events through the same transaction boundary as the state mutation when a runner exists, and best-effort otherwise.
- [x] Add regression coverage proving the new control-plane events are queryable through `ListAuditEvents(...)` and replay helpers.

## Task 5: Align status/docs with the actual kernel state

**Goal:** keep repo guidance honest after the hardening work lands.

**Files:**
- Modify: `docs/STATUS.md`
- Modify: `docs/CURRENT_STATE.md`
- Modify: `docs/V1_RELEASE_CHECKLIST.md`
- Modify: `docs/EMBEDDING.md`
- Modify: `docs/API.md`

- [x] Update status/current-state docs so they no longer claim there are no known core-boundary defects while checklist items are still open.
- [x] Document the supported runner/store consistency model for embedders.
- [x] Reflect the new control-plane audit coverage and runtime-budget semantics in public docs.

## Task 6: Final verification and checklist closeout

**Files:**
- Modify: `docs/plans/2026-03-23-kernel-hardening-checklist-execution.md`

- [x] Run focused tests for each task before marking it complete.
- [x] Run `go test ./pkg/harness/... -count=1`.
- [x] Run `go test ./... -count=1`.
- [x] Mark every completed item in this file and leave no unchecked items.
