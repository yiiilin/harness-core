# Kernel Claim Closure Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** finish the remaining pure-kernel claim / lease gaps so concurrent multi-session platforms can rely on the kernel for execution ownership, approval resume ownership, and recovery ownership.

**Architecture:** keep the kernel identity-neutral and transport-neutral by extending the existing session lease contract instead of adding worker or user concepts. Execution, approval resume, and recovery state transitions should all honor the same active-lease rule: direct APIs remain valid only when no active lease exists; claimed variants require the matching lease id.

**Tech Stack:** Go, `pkg/harness/*`, `internal/postgres`, docs, Go test

---

## Execution Rule

Progress must be tracked in this file only:

- Start every pending task as unchecked
- Mark each task with `[x]` immediately after implementation and verification complete
- If a task reveals a prerequisite gap, add a new unchecked task directly below it before continuing
- If any task remains unchecked, the project is not complete

## Boundary Guardrails

The kernel owns:

- session claim / lease semantics
- approval resume correctness
- recovery correctness
- transport-neutral execution ownership rules

The kernel does not own:

- worker fleet identity
- queue topology
- transport caller metadata
- user / tenant / auth / ownership concepts

### Task 1: Make execution entrypoints lease-aware

**Files:**
- Modify: `pkg/harness/runtime/service.go`
- Modify: `pkg/harness/runtime/runner.go`
- Modify: `pkg/harness/runtime/session_driver.go`
- Modify: `pkg/harness/harness.go`
- Modify: `docs/API.md`
- Modify: `docs/RUNTIME.md`
- Test: `pkg/harness/runtime/*coordination*_test.go`
- Test: `pkg/harness/runtime/*recovery*_test.go`

- [x] Add claimed execution entrypoints for step/session execution without introducing worker identity into kernel types.
- [x] Ensure direct `RunStep` / `RunSession` fail with `session.ErrSessionLeaseNotHeld` when an active lease exists for another holder.
- [x] Keep unclaimed execution working for single-runtime embeddings where no active lease exists.

### Task 2: Close the approval resume claim gap

**Files:**
- Modify: `pkg/harness/runtime/approval_flow.go`
- Modify: `pkg/harness/runtime/session_driver.go`
- Modify: `pkg/harness/session/store.go`
- Modify: `pkg/harness/harness.go`
- Modify: `docs/API.md`
- Modify: `docs/RUNTIME.md`
- Test: `pkg/harness/runtime/*approval*_test.go`
- Test: `pkg/harness/runtime/*coordination*_test.go`

- [x] Add a claimed approval-resume entrypoint and make direct resume respect the active-lease rule.
- [x] Allow approval-approved idle sessions to re-enter runnable claim flow without making still-pending approvals claimable.
- [x] Add regression coverage proving a claimed session can resume approved work while stale/unclaimed callers cannot bypass ownership.

### Task 3: Make recovery state mutations honor the same lease contract

**Files:**
- Modify: `pkg/harness/runtime/recovery.go`
- Modify: `pkg/harness/runtime/runner.go`
- Modify: `pkg/harness/harness.go`
- Modify: `docs/RUNTIME.md`
- Modify: `docs/STATUS.md`
- Test: `pkg/harness/runtime/*recovery*_test.go`
- Test: `pkg/harness/runtime/*conflict*_test.go`

- [x] Add claimed recovery-state mutation entrypoints or equivalent internal enforcement so stale holders cannot write `in_flight` / `interrupted` after reclaim.
- [x] Ensure the runtime’s own execution path uses the lease-aware mutation path consistently.
- [x] Keep recovery semantics transport-neutral and identity-neutral.

### Task 4: Full verification and boundary review

**Files:**
- Modify: `docs/plans/2026-03-20-kernel-claim-closure-execution.md`

- [x] Re-read every completed task and confirm it introduces no user/tenant/auth/transport/worker-fleet concepts into `pkg/harness/*`.
- [x] Run focused red/green verification for each new claim-aware behavior before marking its task done.
- [x] Run `go test ./pkg/harness/... -count=1`.
- [x] Run `go test ./... -count=1`.
- [x] Mark all completed tasks in this file and leave no unchecked items.
