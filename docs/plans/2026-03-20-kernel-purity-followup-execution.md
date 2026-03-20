# Kernel Purity And Follow-up Core Gaps Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** finish turning `harness-core` into a small, transport-neutral execution kernel for multi-session agent platforms by removing the last adapter/module leaks from core surfaces and closing the remaining pure-kernel gaps.

**Architecture:** treat `pkg/harness/runtime` plus the domain contracts under `pkg/harness/*` as the bare kernel. `modules/*` remain capability packs, `adapters/*` remain transport bindings, and user/tenant/auth/ownership/governance stay in the embedding platform.

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

- session lifecycle semantics
- runtime correctness and recovery correctness
- durable execution and planning facts
- replay/debug stability
- transport-neutral control-plane primitives

The kernel does not own:

- user / tenant / org / ownership / RBAC / ACL
- auth/session-login state or gateway behavior
- WebSocket / HTTP / SSE / gRPC protocol envelopes
- dashboards, approval UI, search, analytics, billing, quota
- provider routing, deployment topology, worker fleet policy

Every task below must preserve that boundary.

### Task 1: Strip adapter/auth vocabulary from kernel metadata surfaces

**Files:**
- Modify: `pkg/harness/runtime/service.go`
- Modify: `pkg/harness/harness.go`
- Modify: `docs/API.md`
- Modify: `docs/RUNTIME.md`
- Test: `pkg/harness/*_test.go`
- Test: `adapters/websocket/*_test.go`

- [x] Remove transport-specific and auth-specific fields from exported runtime info and related kernel-facing metadata.
- [x] Ensure no `pkg/harness/*` public type reports adapter-defined or shared-token-specific modes.
- [x] Update docs/tests so the public kernel surface is described only in kernel terms.

### Task 2: Separate bare-kernel construction from builtins composition

**Files:**
- Modify: `pkg/harness/harness.go`
- Modify: `pkg/harness/runtime/defaults.go`
- Create: `pkg/harness/*builtins*`
- Modify: `docs/API.md`
- Modify: `docs/PACKAGE_BOUNDARIES.md`
- Test: `pkg/harness/*_test.go`

- [x] Stop treating built-in module registration as part of the bare-kernel path.
- [x] Move default `shell` / `filesystem` / `http` wiring behind a composition helper that is mechanically separate from the core runtime package.
- [x] Keep a convenience bundle for local embedding and examples without widening kernel scope.
- [x] Add tests/docs that distinguish bare-kernel constructors from builtins bundle constructors.

### Task 3: Add first-class planning / replanning records

**Files:**
- Create: `pkg/harness/*planning*`
- Modify: `pkg/harness/runtime/planning.go`
- Modify: `pkg/harness/runtime/service.go`
- Modify: `pkg/harness/runtime/options.go`
- Modify: `pkg/harness/audit/event.go`
- Modify: `internal/postgres/schema.sql`
- Modify: `internal/postgres/*planning*`
- Test: `pkg/harness/runtime/*planning*_test.go`
- Test: `internal/postgres/*planning*_test.go`

- [x] Record each planning and replanning cycle as a durable first-class fact with ids, status, reason, and correlation links.
- [x] Link planning records to the generated plan revision, capability view, and relevant context summary when present.
- [x] Expose list/get APIs sufficient for replay and debugging without introducing UI/platform projection concepts.
- [x] Add in-memory and Postgres coverage proving planning facts survive restarts and replans.

### Task 4: Expand observability across the full runtime lifecycle

**Files:**
- Modify: `pkg/harness/observability/metrics.go`
- Modify: `pkg/harness/runtime/observability.go`
- Modify: `pkg/harness/audit/event.go`
- Modify: `docs/PROTOCOL.md`
- Test: `pkg/harness/runtime/*observability*_test.go`

- [x] Add vendor-neutral metric samples and trace spans for planning, approval, recovery, abort, and lease lifecycle in addition to step execution.
- [x] Keep names, labels, and attributes transport-neutral and correlation-complete.
- [x] Preserve audit events as the canonical replay envelope while making the exporter surface broad enough for production embeddings.

### Task 5: Remove unsupported parent-session surface until semantics exist

**Files:**
- Modify: `pkg/harness/session/state.go`
- Modify: `pkg/harness/session/store.go`
- Modify: `internal/postgres/schema.sql`
- Modify: `internal/postgres/sessionrepo/*`
- Modify: `docs/ARCHITECTURE.md`
- Modify: `docs/STATUS.md`
- Test: `pkg/harness/session/*_test.go`
- Test: `internal/postgres/sessionrepo/*_test.go`

- [x] Remove `ParentSessionID` from the stable kernel surface unless a concrete transport-neutral runtime semantic is implemented in the same change.
- [x] Update persistence, tests, and docs so no half-formed sub-session concept remains in core.
- [x] Confirm ownership/orchestration semantics still stay outside the kernel boundary.

### Task 6: Tighten lease heartbeat and reclaim semantics

**Files:**
- Modify: `pkg/harness/session/store.go`
- Modify: `pkg/harness/runtime/coordination.go`
- Modify: `pkg/harness/runtime/recovery.go`
- Modify: `docs/RUNTIME.md`
- Test: `pkg/harness/runtime/*coordination*_test.go`
- Test: `internal/postgres/sessionrepo/*_test.go`

- [x] Define heartbeat, lease-renew, expiry, and reclaim semantics explicitly in the kernel contract.
- [x] Ensure reclaim and recovery use deterministic stale-holder rules and surface clean conflicts under race.
- [x] Add regression coverage for renew/reclaim races, expired holders, and interrupted-session recovery.
- [x] Keep queue topology and worker-fleet behavior out of core.

### Task 7: Full verification and boundary review

**Files:**
- Modify: `docs/plans/2026-03-20-kernel-purity-followup-execution.md`

- [x] Re-read every completed task and confirm it introduces no user/tenant/auth/transport/UI/provider-routing concepts into `pkg/harness/*`.
- [x] Run the relevant focused test commands for each completed kernel task before marking it done.
- [x] Run `go test ./pkg/harness/... -count=1`.
- [x] Run `go test ./... -count=1`.
- [x] Mark all completed tasks in this file and leave no unchecked items.
