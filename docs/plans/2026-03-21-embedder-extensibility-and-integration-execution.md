# Embedder Extensibility And Integration Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Improve the public embedding surface around shell PTY extensibility, reusable worker helpers, replay-oriented projections, and integration/stability documentation without expanding kernel scope.

**Architecture:** Keep kernel state-machine semantics in `pkg/harness/runtime` unchanged unless a transport-neutral contract is genuinely missing. PTY extensibility stays in `modules/shell`; reusable worker and replay helpers live under `pkg/harness/*` as public helper packages; integration guidance and stability classification live in docs and examples. The resulting surface should let external platforms embed the kernel with fewer local patches, especially for remote PTY execution, external approval UX, and durable worker orchestration.

**Tech Stack:** Go 1.24, `modules/shell`, `pkg/harness`, `pkg/harness/runtime`, `pkg/harness/postgres`, new public helper packages under `pkg/harness/*`, existing examples, Go tests, Markdown docs.

---

## Execution Rule

Progress must be tracked in this file only:

- Start every pending task as unchecked
- Mark each task with `[x]` immediately after implementation and verification complete
- If a task reveals a prerequisite gap, add a new unchecked task directly below it before continuing
- If any task remains unchecked, the project is not complete

## Scope Guardrails

This plan intentionally stays kernel-first but not kernel-expanding:

- in scope: public shell-module PTY backend extensibility, conditional PTY verifier wiring, reusable worker helpers, replay/debug helper surfaces, integration docs, stability notes, and reference examples
- out of scope: user/tenant/org concepts, business dashboards, approval UI implementation, adapter-specific product protocols, worker fleet deployment topology, and provider-specific business logic
- module-specific runtime control remains outside kernel state semantics
- helper packages may wrap existing kernel APIs, but must not invent parallel execution semantics
- product search/reporting projections remain outside `harness-core`; only replay/debug-friendly public helpers are in scope

## Current State Notes

- `modules/shell` already exposes `Backend` and `PTYManager` customization, but PTY execution still routes through a local-manager assumption in `RegisterWithOptions(...)`.
- PTY-specific verifiers are currently tied to the local `PTYManager` path.
- Claimed worker loops currently exist only in `examples/platform-reference` and `examples/postgres-workers`.
- `pkg/harness/runtime` already exposes `GetExecutionCycle(...)` and `ListExecutionCycles(...)`; this plan is about making replay/debug consumption more obviously public and reusable, not reintroducing duplicate persistence logic.
- `docs/API.md` already contains some stability guidance, but it does not yet classify future helper packages or fully close the loop around embedder-facing stability and versioning expectations.

### Task 1: Lock PTY Embedder Extensibility Semantics With Failing Tests

**Files:**
- Modify: `modules/shell/module_test.go`
- Modify: `modules/shell/pty_test.go`
- Test: `modules/shell/module_test.go`
- Test: `modules/shell/pty_test.go`

- [x] Add failing tests proving `RegisterWithOptions(...)` can use an explicit public PTY execution backend without requiring a local `PTYManager`.
- [x] Add failing tests proving `shell.exec` in `mode=pty` does not silently construct a local-manager-backed implementation when an embedder explicitly supplies an external PTY backend.
- [x] Add failing tests proving PTY-specific verifier kinds are registered only when a local verifier-capable PTY inspector path is present.
- [x] Run focused shell-module tests and verify they fail for the expected missing-extensibility reasons.

### Task 2: Expose A Formal Public PTY Backend Path In `modules/shell`

**Files:**
- Modify: `modules/shell/module.go`
- Modify/Create: `modules/shell/README.md`
- Modify: `docs/MODULES.md`
- Modify: `docs/EXTENSIBILITY.md`
- Test: `modules/shell/module_test.go`
- Test: `modules/shell/pty_test.go`

- [x] Extend `shellmodule.Options` with a formal public PTY execution backend path that embedders can supply directly.
- [x] Preserve `PTYManager` as the default local implementation convenience path rather than the only PTY execution path.
- [x] Keep tool metadata and registration behavior accurate for the resulting PTY/public-backend surface.
- [x] Re-run focused shell-module tests until the PTY backend path is green.

### Task 3: Make PTY Verifier Registration Conditional And Local-Inspector Only

**Files:**
- Modify: `modules/shell/module.go`
- Modify: `modules/shell/pty_verify.go`
- Modify: `modules/shell/README.md`
- Modify: `docs/API.md`
- Modify: `docs/API.zh-CN.md`
- Test: `modules/shell/module_test.go`
- Test: `modules/shell/pty_test.go`

- [x] Register base shell verifiers unconditionally, but register `pty_handle_active`, `pty_stream_contains`, and `pty_exit_code` only when a local PTY inspector path is actually available.
- [x] Keep existing local-`PTYManager` behavior working so built-in reference flows do not regress.
- [x] Document clearly that external PTY backends do not automatically imply local PTY verifier support.
- [x] Re-run focused shell-module tests until conditional verifier wiring is green.

### Task 4: Lock Public Worker Helper Semantics With Failing Tests

**Files:**
- Create: `pkg/harness/worker/worker_test.go`
- Modify: `pkg/harness/harness.go`
- Test: `pkg/harness/worker/worker_test.go`

- [x] Add failing tests for a public helper that claims runnable work, renews leases, runs claimed sessions, and releases leases.
- [x] Add failing tests for recoverable-session preference or fallback so interrupted work can be resumed through the same helper surface.
- [x] Add failing tests covering approval-paused and no-work-found outcomes without encoding platform-specific queue semantics.
- [x] Run focused worker-helper tests and verify they fail for the expected missing-package reasons.

### Task 5: Implement A Public Reusable Worker Helper Package

**Files:**
- Create: `pkg/harness/worker/doc.go`
- Create: `pkg/harness/worker/types.go`
- Create: `pkg/harness/worker/worker.go`
- Modify: `pkg/harness/harness.go`
- Modify: `docs/API.md`
- Modify: `docs/API.zh-CN.md`
- Modify: `docs/STATUS.md`
- Test: `pkg/harness/worker/worker_test.go`

- [x] Implement a small public helper package for claim/renew/run/recover/release flows without adding worker-fleet concepts to kernel types.
- [x] Keep the helper configurable for lease TTL, renew interval, and runnable-vs-recoverable selection while remaining transport-neutral.
- [x] Re-export or explicitly document the helper from the stable embedding path where appropriate.
- [x] Re-run focused helper tests until the public worker surface is green.

### Task 6: Lock Replay / Execution-Cycle Projection Helper Semantics With Failing Tests

**Files:**
- Create: `pkg/harness/replay/replay_test.go`
- Modify: `pkg/harness/harness.go`
- Test: `pkg/harness/replay/replay_test.go`

- [x] Add failing tests proving embedders can build a replay/debug-friendly session projection from public execution cycles plus audit events without scraping storage internals.
- [x] Add failing tests proving the helper preserves logical execution-cycle grouping, event ordering, and runtime-handle visibility.
- [x] Add failing tests that explicitly cover the already-existing `GetExecutionCycle(...)` / `ListExecutionCycles(...)` surface so this work complements rather than replaces it.
- [x] Run focused replay-helper tests and verify they fail for the expected missing-helper reasons.

### Task 7: Implement A Public Replay / Projection Helper Around Existing Execution Facts

**Files:**
- Create: `pkg/harness/replay/doc.go`
- Create: `pkg/harness/replay/types.go`
- Create: `pkg/harness/replay/replay.go`
- Modify: `pkg/harness/harness.go`
- Modify: `docs/API.md`
- Modify: `docs/API.zh-CN.md`
- Modify: `docs/RUNTIME.md`
- Test: `pkg/harness/replay/replay_test.go`

- [x] Add a light public helper package that projects execution cycles and audit events into a replay/debug-friendly view without introducing product-level search/reporting semantics.
- [x] Keep the helper layered on top of existing public execution facts rather than coupling embedders to persistence internals.
- [x] Update the public docs so `ExecutionCycle` reads are clearly part of the recommended replay/debug surface.
- [x] Re-run focused replay-helper tests until the projection surface is green.

### Task 8: Write A Formal Existing-Platform Embedding Guide And Minimal Example

**Files:**
- Create: `docs/EMBEDDING.md`
- Create: `examples/platform-embedding/README.md`
- Create: `examples/platform-embedding/main.go`
- Create: `examples/platform-embedding/main_test.go`
- Modify: `examples/README.md`
- Modify: `docs/API.md`
- Modify: `docs/API.zh-CN.md`
- Modify: `docs/STATUS.md`

- [x] Add a formal embedding guide that explains recommended wrapping patterns around `pkg/harness` and `pkg/harness/postgres`.
- [x] Cover the specific integration cases raised by embedders: external run IDs, external approval UI, remote PTY executor wiring, service restart recovery, and accepted-first API shells around the kernel.
- [x] Add a minimal compilable example that uses only public embedding surfaces and demonstrates the intended integration style.
- [x] Re-run the new example test until it passes.

### Task 9: Clarify Adapter-Facing Public Stability Boundaries

**Files:**
- Create: `docs/VERSIONING.md`
- Modify: `docs/API.md`
- Modify: `docs/API.zh-CN.md`
- Modify: `docs/PACKAGE_BOUNDARIES.md`
- Modify: `docs/STATUS.md`

- [x] Add a formal public-stability note for embedders covering `pkg/harness`, `pkg/harness/postgres`, and any new helper packages added by this plan.
- [x] Classify `modules/*`, `adapters/*`, `examples/*`, and `internal/*` more explicitly so external platforms know what they can depend on and what remains reference/evolving-only.
- [x] Fix the current documentation gap where `API.md` references `VERSIONING.md` but that file does not yet exist.
- [x] Re-read all affected stability docs and ensure the classifications are internally consistent.

### Task 10: Verification And Closeout

**Files:**
- Modify: `docs/plans/2026-03-21-embedder-extensibility-and-integration-execution.md`

- [x] Run focused verification for `modules/shell`, `pkg/harness/worker`, and `pkg/harness/replay`.
- [x] Run focused verification for the new/updated embedding example.
- [x] Run `go test ./... -count=1` after all targeted suites are green.
- [x] Mark completed plan items `[x]` only after fresh verification.
- [x] Re-read this plan file and ensure no unchecked item remains before declaring the project complete.
