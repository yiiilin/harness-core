# Kernel Boundary And Pure-Core Gaps Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** lock down what `harness-core` is responsible for, prevent platform concerns from leaking into the kernel, and close the remaining kernel-only correctness gaps.

**Architecture:** treat `pkg/harness/runtime` plus the domain contracts under `pkg/harness/*` as a transport-neutral execution kernel. New work belongs in core only if it changes global runtime semantics such as execution correctness, recovery correctness, replay stability, or governed execution; transport, identity, tenancy, UI, and product policy concerns stay in adapters/modules/platform layers.

**Tech Stack:** Go, `pkg/harness/runtime`, `pkg/harness/session`, `pkg/harness/approval`, `pkg/harness/execution`, `pkg/harness/capability`, `pkg/harness/observability`, `internal/postgres`, Go test, docs

---

## Execution Rule

Progress must be tracked in this file only:

- Start every pending task as unchecked
- Mark each task with `[x]` immediately after implementation and verification complete
- If a task reveals a prerequisite gap, add a new unchecked task directly below it before continuing
- If any task remains unchecked, the project is not complete

## Kernel Boundary Guardrails

The kernel owns:

- session / plan / step lifecycle semantics
- runtime loop semantics: policy -> approval -> execute -> verify -> recover
- durable execution facts and replay/debug stability
- transport-neutral control-plane primitives required for correct execution
- typed extension points that alter global runtime behavior

The kernel does not own:

- user / tenant / org / ownership / RBAC / ACL
- auth handshakes, bearer tokens, OIDC, API gateway behavior
- WebSocket / HTTP / SSE / gRPC envelope semantics
- dashboards, approval UIs, search pages, reporting, analytics
- product-specific provider routing, prompt strategy, business policy
- deployment topology, worker fleet wiring, queueing infrastructure
- module-specific capability implementations such as browser/windows/pty UX

Kernel admission test for new concepts:

- If the concept is not transport-neutral, it does not belong in core.
- If the concept is not identity-neutral, it does not belong in core.
- If the concept does not change runtime correctness or global runtime semantics, it does not belong in core.
- If the concept can live in `modules/*`, `adapters/*`, or an embedding application without forking the runtime loop, it should stay out of core.

### Task 1: Publish a durable kernel-scope charter

**Files:**
- Create: `docs/KERNEL_SCOPE.md`
- Modify: `docs/ARCHITECTURE.md`
- Modify: `docs/EXTENSIBILITY.md`
- Modify: `docs/RUNTIME.md`
- Modify: `docs/API.md`

- [x] Add a dedicated `docs/KERNEL_SCOPE.md` document that defines kernel-owned, module-owned, adapter-owned, and platform-owned concerns with explicit allow/deny examples.
- [x] Add an explicit kernel admission test so future contributors can reject `tenant`, `user`, `auth`, `UI`, `transport`, `billing`, and provider-policy concepts before they enter core review.
- [x] Cross-link the new scope document from architecture/runtime/extensibility/API docs so the boundary is visible anywhere new hooks are proposed.
- [x] Add a docs verification pass that confirms every concept newly blessed as kernel scope is transport-neutral and identity-neutral.

### Task 2: Add optimistic concurrency and coordination-safe persistence semantics

**Files:**
- Modify: `pkg/harness/session/state.go`
- Modify: `pkg/harness/session/store.go`
- Modify: `pkg/harness/approval/store.go`
- Modify: `internal/postgres/schema.sql`
- Modify: `internal/postgres/sessionrepo/repo.go`
- Modify: `internal/postgres/approvalrepo/repo.go`
- Modify: `pkg/harness/runtime/recovery.go`
- Test: `pkg/harness/runtime/*_test.go`
- Test: `internal/postgres/sessionrepo/*_test.go`
- Test: `internal/postgres/approvalrepo/*_test.go`

- [x] Add kernel-level version / revision fields and any minimal lease metadata required to prevent blind overwrite of mutable coordination records.
- [x] Change mutable session and approval store update paths from blind writes to compare-and-swap style updates that fail cleanly on stale state.
- [x] Ensure runtime recovery, approval response, and resume flows surface concurrency conflicts as runtime errors instead of silently clobbering state.
- [x] Add in-memory and Postgres regression coverage proving concurrent resume/respond/recover attempts cannot both commit as winners.

### Task 3: Add transport-neutral claim / lease primitives for runnable work

**Files:**
- Modify: `pkg/harness/session/state.go`
- Modify: `pkg/harness/session/store.go`
- Modify: `pkg/harness/runtime/service.go`
- Modify: `pkg/harness/runtime/recovery.go`
- Create: `pkg/harness/runtime/coordination.go`
- Modify: `internal/postgres/schema.sql`
- Modify: `internal/postgres/sessionrepo/repo.go`
- Test: `pkg/harness/runtime/*coordination*_test.go`
- Test: `internal/postgres/sessionrepo/*_test.go`

- [x] Add a minimal kernel coordination API for claiming runnable or recoverable sessions without introducing queueing, tenancy, or worker-fleet concepts.
- [x] Add lease renewal and release semantics so a caller can continue or abandon a claim without relying on transport-specific state.
- [x] Ensure selection excludes terminal sessions and approval-blocked sessions while still allowing interrupted sessions to be reclaimed safely.
- [x] Add concurrency tests proving two workers cannot claim the same runnable session at the same time.

### Task 4: Add abort / cancel control-plane semantics to the kernel

**Files:**
- Modify: `pkg/harness/session/state.go`
- Modify: `pkg/harness/runtime/service.go`
- Modify: `pkg/harness/runtime/loop.go`
- Modify: `pkg/harness/runtime/recovery.go`
- Modify: `pkg/harness/runtime/runner.go`
- Modify: `pkg/harness/audit/event.go`
- Test: `pkg/harness/runtime/*abort*_test.go`

- [x] Add a transport-neutral runtime API for aborting a session with a structured reason while keeping user-facing wording and UX out of core.
- [x] Define how abort interacts with pending approvals, in-flight execution, recoverable sessions, and already-terminal sessions.
- [x] Ensure abort transitions, task/session terminal updates, and audit events occur through the same persistence boundary as other runtime state changes.
- [x] Add regression coverage proving aborted sessions cannot be resumed, re-run, or recovered accidentally.

### Task 5: Turn runtime handles into a managed kernel control surface

**Files:**
- Modify: `pkg/harness/execution/types.go`
- Modify: `pkg/harness/execution/stores.go`
- Modify: `pkg/harness/runtime/service.go`
- Modify: `pkg/harness/runtime/runner.go`
- Modify: `internal/postgres/schema.sql`
- Modify: `internal/postgres/executionrepo/repo.go`
- Test: `pkg/harness/runtime/runtime_handle_test.go`
- Test: `internal/postgres/executionrepo/*_test.go`

- [x] Extend `RuntimeHandle` so it can represent lifecycle state rather than only a passive opaque record.
- [x] Add minimal kernel APIs for updating, closing, or invalidating runtime handles without encoding PTY-shell or browser-specific behavior in core.
- [x] Ensure abort/recovery paths can reconcile dangling handles deterministically.
- [x] Add regression coverage proving handle lifecycle changes survive persistence boundaries and restart/recovery flows.

### Task 6: Freeze capability visibility at the plan/session boundary

**Files:**
- Modify: `pkg/harness/capability/types.go`
- Modify: `pkg/harness/runtime/planning.go`
- Modify: `pkg/harness/runtime/session_driver.go`
- Modify: `pkg/harness/runtime/options.go`
- Modify: `internal/postgres/schema.sql`
- Modify: `internal/postgres/capabilityrepo/*`
- Test: `pkg/harness/runtime/*capability*_test.go`

- [x] Add a kernel concept for freezing the capability set visible to planning/execution so replay and recovery do not depend on a drifting live registry.
- [x] Keep existing per-action capability snapshots, but relate them to the frozen plan/session-level capability view.
- [x] Ensure replanning creates an explicit new frozen capability view rather than silently reusing mutable live registry state.
- [x] Add coverage proving replay/recovery semantics stay stable even when the live tool registry changes after planning.

### Task 7: Promote context compaction into a full runtime lifecycle concern

**Files:**
- Modify: `pkg/harness/runtime/planning.go`
- Modify: `pkg/harness/runtime/session_driver.go`
- Modify: `pkg/harness/runtime/options.go`
- Modify: `pkg/harness/runtime/context_types.go`
- Test: `pkg/harness/runtime/context_budget_test.go`
- Test: `pkg/harness/runtime/*session*_test.go`

- [x] Move compaction triggers beyond planner assembly so long-running sessions can compact durable context under explicit runtime control.
- [x] Define when summaries are created, superseded, or reused across plan, execute, and recover phases.
- [x] Keep the abstraction generic; do not add vector DB, retrieval product logic, or tenant memory concepts to core.
- [x] Add tests proving compaction and summary persistence work for long-running sessions outside the planner-only path.

### Task 8: Tighten kernel observability contracts without leaking vendor concerns

**Files:**
- Modify: `pkg/harness/observability/metrics.go`
- Modify: `pkg/harness/runtime/options.go`
- Modify: `pkg/harness/runtime/interfaces.go`
- Modify: `pkg/harness/audit/event.go`
- Modify: `docs/PROTOCOL.md`
- Test: `pkg/harness/runtime/*event*_test.go`
- Test: `pkg/harness/runtime/*metrics*_test.go`

- [x] Keep audit events as the canonical runtime event envelope, but define any remaining required correlation fields and invariants as explicit kernel contract.
- [x] Add transport-neutral exporter hooks for richer metrics and tracing labels without coupling core to Prometheus, OpenTelemetry, or any other vendor surface.
- [x] Ensure observability contracts remain compatible with adapter replay/streaming but do not encode adapter message formats into core types.
- [x] Add tests/docs proving the kernel observability surface is complete enough for replay/debugging without requiring platform projection logic.

### Task 9: Stabilize the public kernel API surface around actual runtime entrypoints

**Files:**
- Modify: `pkg/harness/harness.go`
- Modify: `docs/API.md`
- Modify: `docs/RUNTIME.md`
- Test: `pkg/harness/harness_test.go`

- [x] Reconcile the top-level public facade and docs with the actual kernel entrypoints already present in `runtime.Service`, especially session-level execution and recovery.
- [x] Re-export or document any kernel-first output types needed so consumers can embed the runtime without importing adapter-only concepts.
- [x] Keep the public facade small; do not add transport, auth, or tenant abstractions to make the API feel “complete.”
- [x] Add facade-level tests/docs that establish the intended stable embedding path for a kernel consumer.

### Task 10: Full verification and scope check

**Files:**
- Modify: `docs/plans/2026-03-20-kernel-boundary-core-gaps-execution.md`

- [x] Re-read every completed task and confirm it introduces no user/tenant/auth/transport/UI/product-provider concepts into `pkg/harness/*`.
- [x] Run the relevant focused test commands for each completed kernel task before marking it done.
- [x] Run `go test ./pkg/harness/... -count=1`.
- [x] Run `go test ./... -count=1`.
- [x] Mark all completed tasks in this file and leave no unchecked items.
