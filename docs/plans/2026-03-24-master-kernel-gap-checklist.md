# Master Kernel Gap Checklist

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Keep one authoritative checklist for the remaining pure-kernel and embedder-facing gaps on `dev`, without pulling product-layer concepts into `pkg/harness/*`.

**Architecture:** Preserve the existing kernel boundary, keep the already-landed public surfaces honest, and finish the few remaining runtime behaviors that are still materially incomplete: true concurrent fan-out scheduling, the rest of the attachment materialization contract, and companion-module consumption/release hygiene.

**Tech Stack:** Go 1.24, `pkg/harness/*`, companion modules, release tests, Markdown docs.

---

## Execution Rule

Progress must be tracked in this file only:

- Start every pending task as unchecked
- Mark each task with `[x]` immediately after implementation and verification complete
- If a task reveals a prerequisite gap, add a new unchecked task directly below it before continuing
- If any task remains unchecked, the project is not complete

## Scope Rule

This checklist is constrained by `docs/KERNEL_SCOPE.md`.

In scope:

- transport-neutral runtime semantics
- durable execution-state correctness
- replay/projection stability for kernel-owned facts
- embedder-facing public helpers and extension points that stay product-neutral

Out of scope:

- `tenant_id`, `user_id`, `org_id`
- auth, gateway, or transport session identity
- approval UI or operator console workflow
- queue topology and worker-fleet product orchestration
- billing, quotas, provider routing, or reporting semantics
- product-specific continuation blobs or run IDs

## Re-baselined Current State

Already delivered and intentionally not reopened here:

- [x] Resolver-backed `fanout_all`
- [x] Generic blocked-runtime lifecycle and durable reads/projections
- [x] Transport-neutral interactive control plane in core
- [x] PTY companion hooks via `modules/shell`
- [x] Worker helper and replay/projection public helpers
- [x] Public Postgres bootstrap helper surface
- [x] Temp-file attachment materialization for inline text / bytes and artifact-ref payloads
- [x] Companion-module placeholder-version / repo-local replace guardrails

Still materially open after the above:

- [x] No remaining repo-local kernel/runtime implementation gaps are open in this checklist.
- [x] Remote tag publishing and module-proxy indexing remain explicit release operations, not in-repo kernel tasks.

## Task 1: Implement true concurrent multi-target fan-out scheduling

**Goal:** upgrade native fan-out from sequential step expansion to a scheduler-owned execution round that actually enforces concurrency limits while preserving current durable facts and replay semantics.

**Files:**
- Modify: `pkg/harness/execution/aggregate.go`
- Modify: `pkg/harness/execution/target.go`
- Modify: `pkg/harness/runtime/program.go`
- Modify: `pkg/harness/runtime/program_fanout.go`
- Modify: `pkg/harness/runtime/session_driver.go`
- Modify: `pkg/harness/runtime/runner.go`
- Add: `pkg/harness/runtime/fanout_scheduler.go`
- Test: `pkg/harness/runtime/program_test.go`
- Modify: `docs/EMBEDDER_VNEXT.md`
- Modify: `docs/EMBEDDER_VNEXT_REALITY_CHECK.md`
- Modify: `docs/CURRENT_STATE.md`
- Modify: `docs/STATUS.md`

- [x] Add stable fan-out group metadata/helpers so runtime execution can recover group identity and concurrency settings from compiled target-scoped steps.
- [x] Write a failing runtime test proving `TargetSelection.MaxConcurrency` is ignored today and must be consumed during execution.
- [x] Add a scheduler-owned fan-out round path in the session driver that executes ready siblings from one fan-out group concurrently up to the declared limit.
- [x] Preserve per-target attempts/actions/verifications/artifacts/runtime handles and aggregate summaries through the new concurrent path.
- [x] Preserve current retry / partial-failure / aggregate-verification semantics for fan-out groups, with serial fallback when a round cannot safely run concurrently.
- [x] Update runtime/docs status so fan-out is no longer described as only logical sequential expansion.

## Task 2: Finish the remaining generalized attachment materialization semantics

**Goal:** close the gap between the current temp-file subset and a more complete kernel-native attachment materialization contract, without introducing transport-specific policy into core.

**Files:**
- Modify: `pkg/harness/execution/attachment.go`
- Modify: `pkg/harness/runtime/interfaces.go`
- Modify: `pkg/harness/runtime/program_binding_compile.go`
- Modify: `pkg/harness/runtime/program_binding_resolution.go`
- Modify: `pkg/harness/runtime/attachment_materialization.go`
- Test: `pkg/harness/runtime/program_test.go`
- Modify: `docs/EMBEDDER_VNEXT.md`
- Modify: `docs/EMBEDDER_VNEXT_REALITY_CHECK.md`
- Modify: `docs/CURRENT_STATE.md`
- Modify: `docs/STATUS.md`

- [x] Re-state the exact missing attachment behaviors in code-level terms and pin them to tests instead of leaving this as a vague doc-only “partial”.
- [x] Support stable non-temp-file materialization passthrough for attachment inputs when the embedder materializer wants to return a structured handle rather than only a filesystem path.
- [x] Add regression coverage for inline bytes/text, artifact-backed payloads, and custom materializer return values across native program execution.
- [x] Update docs to describe the supported materialization contract precisely, including what still remains intentionally outside the kernel.

## Task 3: Tighten companion-module external-consumption and release hygiene

**Goal:** make dev-branch and release-branch consumption less surprising for external Go modules without turning remote publishing workflow into a kernel concern.

**Files:**
- Modify: `release/release_test.go`
- Modify: `docs/VERSIONING.md`
- Modify: `docs/RELEASING.md`
- Modify: `docs/STATUS.md`
- Modify: `docs/CURRENT_STATE.md`

- [x] Re-check all companion-module `go.mod` relationships against the current `dev` branch and capture the remaining failure modes as executable release assertions where possible.
- [x] Add release guardrails that distinguish acceptable dev pseudo-version linkage from release-tag linkage, so repo tests fail before shipping an unresolvable companion version graph.
- [x] Update versioning/releasing docs so embedders know exactly what is supported on `@dev`, what requires published companion tags, and what remains an external release operation rather than a kernel API guarantee.

## Task 4: Final verification and checklist synchronization

**Files:**
- Modify: `docs/plans/2026-03-24-master-kernel-gap-checklist.md`

- [x] Run focused tests for each completed task before checking it off.
- [x] Run the relevant repo-level verification after all open tasks land.
- [x] Re-read this file and ensure every `[x]` has matching implementation plus fresh verification evidence.
