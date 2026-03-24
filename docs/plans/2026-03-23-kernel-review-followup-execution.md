# Kernel Review Follow-Up Execution Plan

> Progress must be tracked in this file only. Every task starts unchecked and is marked `[x]` only after code, tests, and docs are complete.

**Goal:** close the remaining kernel logic gaps from the latest review without expanding scope into platform concerns.

**Non-goals:** tenant/user/org identity, auth, UI workflows, transport-specific behavior, provider routing, billing, or fleet orchestration.

## Execution Rule

Progress must be tracked in this file only:

- Start every pending task as unchecked
- Mark each task with `[x]` immediately after implementation and verification complete
- If a task reveals a prerequisite gap, add a new unchecked task directly below it before continuing
- If any task remains unchecked, the project is not complete

## Task 1: Eliminate no-runner split commits in approval request/response paths

**Goal:** no-runner `RunStep()` ask flows and `RespondApproval()` flows must not return an error after partially committing approval/session/task/plan state.

**Files:**
- Modify: `pkg/harness/runtime/runner.go`
- Modify: `pkg/harness/runtime/approval_flow.go`
- Test: `pkg/harness/runtime/*approval*_test.go`
- Test: `pkg/harness/runtime/*persistence*_test.go`

- [x] Add regression coverage proving no-runner ask/reject/approve failures do not strand partially updated approval/session/task/plan state.
- [x] Implement compensation or reordered persistence so no-runner approval control paths stay logically atomic enough for callers.

## Task 2: Eliminate no-runner split commits in abort paths

**Goal:** no-runner `AbortSession()` must not leave approval/plan/task/runtime-handle state terminalized if the final session write fails.

**Files:**
- Modify: `pkg/harness/runtime/abort.go`
- Test: `pkg/harness/runtime/abort_test.go`

- [x] Add regression coverage proving no-runner abort failures do not leave partially aborted state behind.
- [x] Implement compensation or persistence ordering so no-runner abort stays terminally consistent on failure.

## Task 3: Make runtime `WithEventSink()` preserve local audit durability semantics

**Goal:** replacing the event sink after `New(...)` must preserve the same audit rebinding/fanout guarantees as `Options` and Postgres bootstrap wiring.

**Files:**
- Modify: `pkg/harness/runtime/service.go`
- Modify: `pkg/harness/runtime/runner.go`
- Test: `pkg/harness/runtime/eventsink_integration_test.go`

- [x] Add regression coverage proving `WithEventSink()` does not silently sever local audit persistence in no-runner and partial-runner paths.
- [x] Rebind/fanout runtime event sinks on `WithEventSink()` using the same local-audit semantics as construction-time wiring.

## Task 4: Final verification and checklist closeout

**Files:**
- Modify: `docs/plans/2026-03-23-kernel-review-followup-execution.md`

- [x] Run focused tests for each task before marking it complete.
- [x] Run `go test ./pkg/harness/runtime ./pkg/harness/worker ./release -count=1`.
- [x] Run `go test ./... -count=1`.
- [x] Mark every completed item in this file and leave no unchecked items.
