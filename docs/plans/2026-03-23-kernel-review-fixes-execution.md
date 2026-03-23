# Kernel Review Fixes Execution Plan

> Progress must be tracked in this file only. Every task starts unchecked and is marked `[x]` only after code, tests, and docs are complete.

**Goal:** close the current kernel logic gaps found in code review without expanding scope into platform concerns.

**Non-goals:** tenant/user/org identity, auth, UI workflows, transport-specific behavior, provider routing, billing, or fleet orchestration.

## Execution Rule

Progress must be tracked in this file only:

- Start every pending task as unchecked
- Mark each task with `[x]` immediately after implementation and verification complete
- If a task reveals a prerequisite gap, add a new unchecked task directly below it before continuing
- If any task remains unchecked, the project is not complete

## Task 1: Make worker lease release resilient to canceled run contexts

**Goal:** ensure `worker.RunOnce()` still releases an already-claimed lease even when the run context is canceled.

**Files:**
- Modify: `pkg/harness/worker/worker.go`
- Test: `pkg/harness/worker/worker_test.go`

- [x] Release the lease with a cleanup context that is not already canceled by the run path.
- [x] Add regression coverage proving a canceled run context still results in lease release rather than a leaked lease/error fan-out.

## Task 2: Stop swallowing plan/task persistence errors in no-runner execution paths

**Goal:** keep no-runner runtime semantics honest by surfacing real plan/task persistence failures instead of returning success with split state.

**Files:**
- Modify: `pkg/harness/runtime/runner.go`
- Modify: `pkg/harness/runtime/approval_flow.go`
- Test: `pkg/harness/runtime/*_test.go`

- [x] Surface `plan.Store` and `task.Store` update failures on no-runner ask/deny/complete/reject paths.
- [x] Add regression coverage proving those failures are returned and do not silently report success.

## Task 3: Keep runtime budget anchoring inside the same durable state transition

**Goal:** prevent `runtime_started_at` from being persisted in a separate transaction before the session actually enters `in_flight`.

**Files:**
- Modify: `pkg/harness/runtime/recovery.go`
- Modify: `pkg/harness/runtime/runtime_budget_anchor.go`
- Test: `pkg/harness/runtime/*budget*_test.go`
- Test: `pkg/harness/runtime/*recovery*_test.go`

- [x] Fold the runtime budget anchor write into the same mutation boundary used by `markSessionInFlight`.
- [x] Add regression coverage proving failed `in_flight` mutation does not leave a burned runtime budget anchor behind.

## Task 4: Make no-runner task attachment fail atomically enough to avoid split session/task state

**Goal:** stop `AttachTaskToSession()` from persisting the session side first and then failing after the task side write, leaving split state in explicit no-runner mode.

**Files:**
- Modify: `pkg/harness/runtime/lifecycle.go`
- Test: `pkg/harness/runtime/service_lifecycle_test.go`

- [x] Make no-runner attach either complete both writes or compensate the first write before returning an error.
- [x] Add regression coverage proving failed task-store update does not leave the session attached while the task remains stale.

## Task 5: Stop worker execution promptly after lease renewal failure

**Goal:** if the worker loses its lease during renewal, the current claimed run must stop rather than continuing to execute after ownership has been lost.

**Files:**
- Modify: `pkg/harness/worker/worker.go`
- Test: `pkg/harness/worker/worker_test.go`

- [x] Add regression coverage proving a renewal failure cancels the in-flight run and returns the lease-loss error.
- [x] Ensure renewal failure actively interrupts the claimed run instead of only being reported after the run returns on its own.

## Task 6: Bound worker lease-release cleanup after caller cancellation

**Goal:** keep the post-run lease release resilient to caller cancellation without allowing cleanup to block forever.

**Files:**
- Modify: `pkg/harness/worker/worker.go`
- Test: `pkg/harness/worker/worker_test.go`

- [x] Add regression coverage proving cleanup release observes a bounded timeout rather than hanging unbounded.
- [x] Replace unbounded `context.WithoutCancel(...)` cleanup with a bounded cleanup context.

## Task 7: Make abort reconcile pending approvals and blocked attempts

**Goal:** aborting a session must not leave approval or execution history suspended in pending/blocked states.

**Files:**
- Modify: `pkg/harness/runtime/abort.go`
- Test: `pkg/harness/runtime/abort_test.go`

- [x] Add regression coverage proving abort resolves a pending approval into a terminal approval record.
- [x] Add regression coverage proving abort finalizes the blocked attempt tied to that approval.
- [x] Reconcile approval and blocked-attempt state during abort so read models and replay stay terminally consistent.

## Task 8: Final verification and checklist closeout

**Files:**
- Modify: `docs/plans/2026-03-23-kernel-review-fixes-execution.md`

- [x] Run focused tests for each task before marking it complete.
- [x] Run `go test ./pkg/harness/worker ./pkg/harness/runtime ./release -count=1`.
- [x] Run `go test ./... -count=1`.
- [x] Mark every completed item in this file and leave no unchecked items.

## Task 9: Make abort preempt claimed execution ownership

**Goal:** an external abort must revoke the active lease so an already-claimed worker cannot later overwrite the aborted session state.

**Files:**
- Modify: `pkg/harness/runtime/abort.go`
- Test: `pkg/harness/runtime/abort_test.go`

- [x] Add regression coverage proving abort during claimed execution causes the in-flight claimed run to fail with lease-not-held and preserves the session's aborted terminal state.
- [x] Clear active lease ownership as part of abort persistence so claimed follow-up writes no longer succeed.

## Task 10: Reconcile active plan steps on abort outside approval-pending flows

**Goal:** aborting an in-flight or recoverable session must also mark the current plan step terminal instead of leaving plan state pending/running behind a terminal session.

**Files:**
- Modify: `pkg/harness/runtime/abort.go`
- Test: `pkg/harness/runtime/abort_test.go`

- [x] Add regression coverage proving abort marks an in-flight plan step failed with a finished timestamp.
- [x] Update the latest plan step for active non-approval abort paths.

## Task 11: Terminalize already-approved pending approvals on abort

**Goal:** if approval has already been granted but the session is aborted before resume, the approval must stop being reusable/resumable.

**Files:**
- Modify: `pkg/harness/runtime/abort.go`
- Test: `pkg/harness/runtime/abort_test.go`

- [x] Add regression coverage proving abort converts an approved pending approval into a non-approved terminal record and finalizes its blocked attempt.
- [x] Stop leaving `reply_always` approvals in reusable `approved` state after the owning session is aborted.

## Task 12: Final verification and checklist closeout

**Files:**
- Modify: `docs/plans/2026-03-23-kernel-review-fixes-execution.md`

- [x] Run focused tests for each task before marking it complete.
- [x] Run `go test ./pkg/harness/worker ./pkg/harness/runtime ./release -count=1`.
- [x] Run `go test ./... -count=1`.
- [x] Mark every completed item in this file and leave no unchecked items.

## Task 13: Keep policy-denied steps out of recovery/in-flight persistence

**Goal:** a step rejected by policy before tool execution must not first persist `ExecutionInFlight` or burn runtime budget anchor state.

**Files:**
- Modify: `pkg/harness/runtime/runner.go`
- Modify: `pkg/harness/runtime/recovery.go`
- Test: `pkg/harness/runtime/*deny*_test.go`
- Test: `pkg/harness/runtime/*budget*_test.go`

- [x] Add regression coverage proving a denied step does not leave the session recoverable or marked in-flight.
- [x] Move in-flight persistence so deny-only paths fail/complete without first entering recovery ownership state.

## Task 14: Stop swallowing authoritative session reload failures during step execution

**Goal:** once the runtime transitions a session into in-flight execution, it must fail fast if the authoritative session reload fails.

**Files:**
- Modify: `pkg/harness/runtime/runner.go`
- Test: `pkg/harness/runtime/*_test.go`

- [x] Add regression coverage proving a failed post-in-flight session reload aborts step execution instead of continuing on stale or zero-value state.
- [x] Return the session reload error instead of discarding it after `markSessionInFlight()`.

## Task 15: Keep normalized recoverable sessions discoverable across restarts

**Goal:** if recovery normalization succeeds but execution does not continue, the session must still be claimable and recoverable after a restart.

**Files:**
- Modify: `pkg/harness/runtime/recovery.go`
- Modify: `pkg/harness/session/store.go`
- Test: `pkg/harness/runtime/recovery_test.go`
- Test: `pkg/harness/runtime/coordination_test.go`

- [x] Add regression coverage proving a normalized `PhaseRecover` session remains visible to recoverable workers after restart or reopen.
- [x] Align recoverable listing and claim semantics with the normalized recovery state instead of dropping those sessions from discovery.

## Task 16: Make no-runner planner persistence semantics explicit and non-splitting

**Goal:** no-runner plan creation must not return an error after the plan record is already committed unless dependent persistence is compensated or explicitly best-effort.

**Files:**
- Modify: `pkg/harness/runtime/capability_freeze.go`
- Test: `pkg/harness/runtime/*planning*_test.go`

- [x] Add regression coverage around no-runner plan creation when capability snapshot or planning-record persistence fails.
- [x] Remove the “plan committed but API failed” split-state path from no-runner planner-backed plan creation.

## Task 17: Preserve local audit durability when external sinks fail in no-runner mode

**Goal:** best-effort no-runner event emission should still record local audit events even when an embedder-provided `EventSink` returns an error.

**Files:**
- Modify: `pkg/harness/runtime/eventsink.go`
- Modify: `pkg/harness/runtime/options.go`
- Test: `pkg/harness/runtime/eventsink_integration_test.go`

- [x] Add regression coverage proving local audit storage still receives events when a fanout peer sink fails in no-runner mode.
- [x] Change fanout/no-runner event delivery so local audit persistence is not short-circuited by external sink failures.

## Task 18: Final verification and checklist closeout

**Files:**
- Modify: `docs/plans/2026-03-23-kernel-review-fixes-execution.md`

- [x] Run focused tests for each new task before marking it complete.
- [x] Run `go test ./pkg/harness/runtime ./pkg/harness/worker ./release -count=1`.
- [x] Run `go test ./... -count=1`.
- [x] Mark every completed item in this file and leave no unchecked items.
