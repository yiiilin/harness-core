# Kernel Review Round 3 Execution Plan

> Progress for this round must be tracked in this file only. Every item starts unchecked and is marked `[x]` only after implementation and fresh verification are complete.

**Goal:** close the remaining kernel logic gaps from the latest review without expanding scope into platform-layer concerns.

**Non-goals:** tenant/user identity, auth, UI-specific lifecycle, product-side run metadata, adapter transport policy, or platform orchestration.

## Execution Rule

Progress must be tracked in this file only:

- Start every task as unchecked
- Mark a task `[x]` only after code and tests are complete
- If a task reveals a prerequisite gap, insert it directly below the task before continuing
- If any task remains unchecked, this round is not complete

## Task 1: Remove no-runner post-commit error splitting from ordinary step execution

**Goal:** once no-runner `RunStep()` has already committed the durable session/plan/task outcome for a denied or executed step, later auxiliary persistence failures must not surface as an operation failure that invites duplicate execution.

**Files:**
- Modify: `pkg/harness/runtime/runner.go`
- Test: `pkg/harness/runtime/no_runner_persistence_errors_test.go`

- [x] Add regression coverage proving no-runner `RunStep()` stays successful when post-commit execution-fact or approval-finalization persistence fails.
- [x] Align no-runner ordinary step execution with explicit post-commit best-effort semantics instead of returning split-state errors.

## Task 2: Preserve audit durability for fanout sinks that do not already carry an audit child

**Goal:** `WithEventSink(...)`, constructor defaults, and Postgres bootstrap must still retain local audit persistence when the caller provides a `FanoutEventSink` whose children are all non-audit sinks.

**Files:**
- Modify: `pkg/harness/runtime/eventsink.go`
- Modify: `pkg/harness/runtime/service.go`
- Modify: `pkg/harness/runtime/options.go`
- Modify: `pkg/harness/postgres/runtime.go`
- Test: `pkg/harness/runtime/eventsink_integration_test.go`
- Test: `pkg/harness/postgres/runtime_test.go`

- [x] Add regression coverage proving fanout sinks without an audit child still preserve local audit durability.
- [x] Fix event-sink rebinding so fanout replacement and durable bootstrap always append or preserve an effective audit sink.

## Task 3: Bind pinned execution state and step writeback to a concrete plan revision

**Goal:** step execution, approval resolution, recovery, abort, and rollback must update the originating plan revision instead of blindly mutating the latest plan for the session.

**Files:**
- Modify: `pkg/harness/runtime/session_driver.go`
- Modify: `pkg/harness/runtime/runner.go`
- Modify: `pkg/harness/runtime/approval_flow.go`
- Modify: `pkg/harness/runtime/abort.go`
- Modify: `pkg/harness/runtime/rollback.go`
- Modify: `pkg/harness/runtime/service_reads.go`
- Modify: `pkg/harness/runtime/*session*.go` as needed
- Modify: `pkg/harness/plan/spec.go`
- Test: `pkg/harness/runtime/*approval*_test.go`
- Test: `pkg/harness/runtime/*recovery*_test.go`

- [x] Add regression coverage proving approval/resume/recovery paths keep writing to the originating plan revision even after a newer plan revision exists.
- [x] Introduce a stable plan identity binding for pinned steps and use it across step updates, rollback, abort, and recovery selection.

## Task 4: Final verification and checklist closeout

**Files:**
- Modify: `docs/plans/2026-03-23-kernel-review-round3-execution.md`

- [x] Run focused tests for each task before marking it complete.
- [x] Run `go test ./pkg/harness/runtime ./pkg/harness/postgres ./pkg/harness/worker ./release -count=1`.
- [x] Run `go test ./... -count=1`.
- [x] Mark every completed item in this file and leave no unchecked items.
