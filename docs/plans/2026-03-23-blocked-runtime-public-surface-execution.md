# Blocked Runtime Public Surface Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a public blocked-runtime projection and query surface for the current approval-backed pause/resume model without pretending the broader vNext blocked-runtime engine already exists.

**Architecture:** Keep persistence and runtime state transitions unchanged. Derive blocked-runtime projections from existing session, approval, attempt, execution-cycle, and runtime-handle facts. Scope this slice to the current approval-backed blocked state only, with durable lookup-after-restart by `session_id` and `approval_id`. Do not introduce execution-target scheduling, generic external-condition engines, or new resume/reopen control-plane methods in this change.

**Tech Stack:** Go 1.24, `pkg/harness/execution`, `pkg/harness/runtime`, `pkg/harness`, existing in-memory and runner-backed tests, Markdown docs.

---

## Execution Rule

Progress must be tracked in this file only:

- Start every pending task as unchecked
- Mark each task with `[x]` immediately after implementation and verification complete
- If a task reveals a prerequisite gap, add a new unchecked task directly below it before continuing
- If any task remains unchecked, the project is not complete

## Scope Guardrails

- in scope: approval-backed blocked-runtime projection types, `Get/List` query APIs, durable lookup by approval/session id, facade re-exports, docs
- out of scope: native multi-target fan-out, generic blocked-runtime store, tool-graph runtime, target-scoped action execution, interactive-specific blocked-runtime engines
- blocked-runtime support in this slice must be documented as the current approval-backed subset, not the full future vNext model
- do not add product workflow concepts such as approval TTL, operator assignment, tenant ownership, or UI states
- `ListBlockedRuntimes()` must define a stable ordering contract
- projection source-of-truth must be explicit:
  - current blocked runtime is discovered from `session.pending_approval_id`
  - approval record must still be `pending`
  - waiting step comes from the approval record step
  - cycle identity comes from the latest blocked attempt for that approval when present, otherwise from blocked-step metadata when present
- not-blocked lookups must return a dedicated `ErrBlockedRuntimeNotFound`

### Task 1: Lock Approval-Backed Blocked Runtime Semantics With Failing Tests

**Files:**
- Create: `pkg/harness/runtime/blocked_runtime_test.go`
- Modify: `release/release_test.go`

- [x] Add failing tests proving a session paused on approval can be projected as a blocked runtime through public service APIs.
- [x] Add failing tests proving blocked-runtime reads expose durable lookup ids, waiting step identity, cycle identity, and current approval metadata.
- [x] Add failing tests proving non-blocked sessions do not appear in the blocked-runtime read surface.
- [x] Add failing tests proving `ListBlockedRuntimes()` uses a stable public ordering.
- [x] Add release-surface compile coverage for the new public methods and re-exported types.

### Task 2: Implement Public Blocked Runtime Projection Types And Service Reads

**Files:**
- Create: `pkg/harness/execution/blocked_runtime.go`
- Modify: `pkg/harness/runtime/service.go`
- Modify: `pkg/harness/runtime/service_reads.go`
- Modify: `pkg/harness/harness.go`

- [x] Add public blocked-runtime projection types and error values under `pkg/harness/execution`.
- [x] Implement runner-aware `GetBlockedRuntime(sessionID)`, `GetBlockedRuntimeByApproval(approvalID)`, and `ListBlockedRuntimes()` service methods for durable lookup only.
- [x] Build projections only from current approval-backed blocked state and existing execution facts.
- [x] Apply the explicit projection/source-of-truth rules and return `ErrBlockedRuntimeNotFound` when the runtime is not currently blocked.
- [x] Keep JSON/public-field naming explicit so supported vs absent data is not ambiguous.

### Task 3: Document The Current Blocked Runtime Boundary

**Files:**
- Modify: `docs/API.md`
- Modify: `docs/EMBEDDING.md`
- Modify: `docs/EMBEDDER_VNEXT.md`

- [x] Document the new blocked-runtime read surface as the current approval-backed subset.
- [x] State explicitly that generic blocked-runtime engines and non-approval blocking conditions remain planned vNext work.
- [x] Link the new surface from the existing embedder docs without over-claiming broader support.

### Task 4: Verification And Closeout

**Files:**
- Modify: `docs/plans/2026-03-23-blocked-runtime-public-surface-execution.md`

- [x] Run focused verification for the new blocked-runtime tests and release compile coverage.
- [x] Run `go test ./release ./pkg/harness/execution ./pkg/harness/runtime -count=1`.
- [x] Mark completed tasks `[x]` only after fresh verification.
- [x] Re-read this plan file and ensure no unchecked item remains if the slice is complete.
