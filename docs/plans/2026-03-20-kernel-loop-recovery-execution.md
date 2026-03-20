# Kernel Loop Recovery Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** close the biggest remaining kernel gap by adding a session-level execution driver and a recovery driver that can continue persisted work without host-side orchestration.

**Architecture:** add exported runtime entrypoints that operate on persisted session/task/plan state instead of requiring the host to manually select steps. The new driver should reuse existing planner, approval, and `RunStep()` semantics, and recovery should route through the same step-selection logic so durable state remains the single source of truth.

**Tech Stack:** Go, `pkg/harness/runtime`, existing in-memory/Postgres stores, Go test

---

## Execution Rule

Progress must be tracked in this file only:

- Start every pending task as unchecked
- Mark each task with `[x]` immediately after implementation and verification complete
- If a task reveals a prerequisite gap, add a new unchecked task directly below it before continuing
- If any task remains unchecked, the project is not complete

### Task 1: Add regression coverage for session driver and recovery behavior

**Files:**
- Modify: `pkg/harness/runtime/planner_integration_test.go`
- Modify: `pkg/harness/runtime/recovery_test.go`

- [x] Add a failing test that proves the runtime can drive a planner-derived multi-step session to completion without the host manually calling `RunStep()` for each step.
- [x] Add a failing test that proves the driver pauses cleanly on pending approval and resumes to completion after approval is granted.
- [x] Add a failing test that proves an interrupted session with a persisted plan can be recovered and completed through a dedicated recovery entrypoint.
- [x] Run focused runtime tests to confirm the new coverage fails for the expected missing behavior.

### Task 2: Implement session-level execution driver

**Files:**
- Modify: `pkg/harness/runtime/service.go`
- Modify: `pkg/harness/runtime/planning.go`
- Create: `pkg/harness/runtime/session_driver.go`

- [x] Add internal helpers that load the latest plan and select the next executable step from durable state.
- [x] Add an exported `RunSession(...)` runtime entrypoint that creates a plan when needed, executes steps in order, stops on approval waits, and returns when the session reaches a terminal or blocked state.
- [x] Keep the driver transport-neutral and reuse existing `CreatePlanFromPlanner()`, `RunStep()`, and persisted state transitions instead of duplicating step execution logic.
- [x] Run focused runtime tests to verify the driver now passes its new coverage.

### Task 3: Implement recovery driver

**Files:**
- Modify: `pkg/harness/runtime/recovery.go`
- Modify: `pkg/harness/runtime/service.go`
- Modify: `pkg/harness/runtime/session_driver.go`

- [x] Add an exported `RecoverSession(...)` runtime entrypoint that resumes approved pending-approval sessions and continues interrupted/in-flight sessions through the same session driver.
- [x] Ensure recovery normalizes `ExecutionState` / `Phase` before continuing so persisted state and resumed execution stay coherent.
- [x] Keep recovery behavior bounded to kernel-owned semantics only; do not introduce transport-specific retry logic.
- [x] Run focused runtime tests to verify recovery now passes its new coverage.

### Task 4: Full verification and close-out

**Files:**
- Modify: `docs/plans/2026-03-20-kernel-loop-recovery-execution.md`

- [x] Run `go test ./pkg/harness/runtime -count=1`.
- [x] Run `go test ./... -count=1`.
- [x] Mark all completed tasks in this file and leave no unchecked items.
