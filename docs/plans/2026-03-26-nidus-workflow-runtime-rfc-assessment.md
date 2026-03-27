# Nidus Workflow Runtime RFC Assessment

> **For maintainers:** This document is an architectural assessment plus implementation checklist. It is intentionally stricter than a feature wishlist.

**Goal:** Decide which parts of the Nidus workflow-runtime RFC should be implemented inside `harness-core`, based on current kernel boundaries and the code that exists today.

**Architecture:** Keep the kernel centered on `session + plan + step + execution facts`, extend the existing `Program` / approval / blocked-runtime / interactive-handle model where it closes real kernel gaps, and explicitly defer any move to a full workflow graph runtime until there is a separate IR/store/scheduler design.

**Tech Stack:** Go 1.24, `pkg/harness/*`, Postgres-backed persistence, runtime replay/audit/event projections.

---

## Current Code Reality

The current kernel is already beyond basic step execution, but it is not yet a general workflow engine:

- `docs/CURRENT_STATE.md` describes the project as a pre-1.0 execution kernel, not a product runtime.
- `pkg/harness/plan/spec.go` persists a linear `Plan -> []StepSpec` model; there are no first-class `Scope`, `Edge`, `Graph`, or mutation-patch records.
- `pkg/harness/runtime/program.go` accepts a DAG-shaped `Program`, but compiles it into flat `plan.StepSpec` values before execution.
- `pkg/harness/runtime/session_driver.go` still selects work from the current plan as step-oriented session execution, with one special concurrent path for target fan-out rounds.
- `pkg/harness/runtime/fanout_scheduler.go` proves that concurrent scheduling exists today only for target siblings inside one aggregate fan-out group.
- `pkg/harness/runtime/approval_flow.go` already gives the kernel durable approval pause/resume semantics tied to the original blocked step context.
- `pkg/harness/runtime/blocked_runtime_lifecycle.go` already gives the kernel a generic blocked-runtime record and session-level blocked/unblocked lifecycle.
- `pkg/harness/runtime/interactive_control.go` already gives the kernel durable interactive runtime handles and a transport-neutral interactive controller API.
- `pkg/harness/runtime/execution_cycle_reads.go` already gives the kernel stable execution-cycle projections built from attempts/actions/verifications/artifacts/runtime handles.

This means the RFC is asking for two different kinds of work:

1. incremental kernel extensions that fit the current model
2. a much larger shift from execution kernel to workflow graph kernel

Those two must not be mixed.

## Decision Summary

### Implement In Kernel Now

These requests fit the current kernel boundary and extend existing runtime semantics rather than replacing them:

- whole-request or session-entry approval gates
- stronger approval-state and blocked-runtime continuation semantics
- graph-native interactive handle usage within the existing `Program` model
- richer heterogeneous program execution over current `ProgramNode` actions
- stronger replay / projection / recovery facts for approvals, handles, and program lineage
- broader sibling concurrency for ready `ProgramNode`s, if done as an extension of current `Program` execution rather than as a new workflow store

### Defer Behind A Separate Workflow-IR Design

These requests are valid kernel topics, but they are not incremental changes to the current architecture:

- first-class `ExecutionGraph / Scope / Edge / AppendPatch / GraphInterruptRecord`
- nested runtime kinds: `primitive`, `loop`, `graph`
- descendant-only graph mutation and append validation
- scope-aware failure propagation such as `cancel_scope`, `fail_parent`, `join_partial`
- event-sourced graph mutation history and graph snapshot rebuild

These should only start after maintainers explicitly choose to introduce a new durable graph IR/store/scheduler layer.

### Keep Out Of Kernel

These remain product-layer responsibilities and should not be accepted as kernel scope:

- workflow mode selection based on product semantics
- product-specific approval UX, correlation ids, or notification flows
- frontend hydration protocols
- Nidus-specific run-manager / SSE / conversation projections
- platform-specific continuation payload glue

## Requirement Triage

| RFC area | Decision | Reason |
| --- | --- | --- |
| Workflow-grade runtime | Partial now, full defer | Current kernel has `Program`, but execution persists as flat steps rather than first-class graph entities. |
| Native nested runtime composition | Defer | No current `scope` or child-runtime ownership model exists in storage or scheduler. |
| Approval as first-class primitive | Support now | Approval/resume is already durable and kernel-owned; session/request gates are a natural extension. |
| Interactive runtime handles inside graphs | Support now | Interactive handles are already kernel-owned facts; the missing piece is native program integration. |
| Rich tool-graph / multi-action execution | Support selectively now | Current `Program` path already covers typed dataflow and fan-out; it should be strengthened rather than replaced. |
| Stable projection / replay facts | Support now | Existing execution cycles, audit events, blocked-runtime projections, and handle projections are already public. |
| Durable recovery semantics | Support now | Recovery is already kernel contract; every newly accepted execution-model feature must preserve it. |

## Recommended Implementation Checklist

### P0: Lock Boundary Before Adding Features

- [x] Publish a maintainer decision that `harness-core` stays on the current `session + plan + step` architecture for this wave.
- [x] Update `docs/EMBEDDER_VNEXT.md` and `docs/EMBEDDER_VNEXT_REALITY_CHECK.md` with an explicit “not a full workflow graph runtime yet” statement.
- [x] Define acceptance language for “program-runtime extensions” versus “new workflow IR”, so follow-up PRs do not smuggle in graph-engine concepts piecemeal.

### P1: Extend Approval To Cover Request-Level Workflow Gating

- [x] Add a session/request-level approval gate API that blocks execution before the first runnable program/plan step.
- [x] Keep resume semantics pointed at the original blocked execution context instead of creating a fresh session/runtime path.
- [x] Decide whether second confirmation belongs in `approval.Record` or should remain a generic `BlockedRuntime` condition; implement only one model, not both.
- [x] Add restart/recovery tests for request-level approval, mid-program approval, rejection, and repeated resume attempts.

### P2: Make Interactive Handles Program-Native

- [x] Add a typed runtime-handle reference contract that later program steps can consume without scraping opaque metadata.
- [x] Extend program binding resolution so later steps can address runtime handles directly, not only structured output/artifact refs.
- [x] Add native program operations for interactive handle lifecycle: start, inspect/view, write, verify, close.
- [x] Preserve stable handle identity, cycle linkage, and version-safe updates through replay and restart.
- [x] Add approval-gated interactive-program tests so handle operations can sit behind approval boundaries without identity loss.

### P3: Strengthen The Existing Program Runtime Instead Of Replacing It

- [x] Add dependency-aware scheduling for ready sibling `ProgramNode`s beyond target fan-out groups.
- [x] Introduce explicit per-program or per-node concurrency policy instead of relying only on target-fanout aggregate metadata.
- [x] Keep typed dataflow on the current `ProgramInputBinding` model and extend it only where the missing reference kind is kernel-owned.
- [x] Ensure shell, tool, artifact-producing, and interactive-handle operations can coexist in one `Program`.
- [x] Add replay/projection views that expose program lineage clearly enough for restart hydration without inventing a full graph-event store yet.

### P4: Harden Projection And Recovery Around Newly Accepted Features

- [x] Add stable projection fields for program node lineage, approval linkage, blocked-runtime linkage, and runtime-handle lineage.
- [x] Add idempotent recovery tests for concurrent ready-node execution so restart does not duplicate sibling work.
- [x] Add idempotent recovery tests for handle close, handle invalidation, and approval replay.
- [x] Add sequence/causality assertions for any new audit event types introduced by request-level approval or program-native interactive operations.

### P5: Separate Design Track For A Future Workflow Graph Kernel

- [x] Write a dedicated design doc for a real workflow IR before implementing any `Scope`, `Edge`, `AppendPatch`, or descendant-mutation API.
- [x] Decide whether that future design replaces `Plan` execution, lowers into `Plan`, or runs beside it.
- [x] Define mutation-safety rules, failure-propagation rules, and replay guarantees before accepting any graph append API.
- [x] Require at least one concrete migration story from current `Program` execution into the future graph model before building the store/scheduler.

## Explicitly Not Recommended For The Current Wave

- Do not introduce `ExecutionGraph`, `Scope`, and `AppendPatch` types without also deciding their durable store, scheduler semantics, and recovery contract.
- Do not add nested `loop` / `graph` runtimes as ad hoc metadata over `plan.StepSpec`.
- Do not copy Nidus’s local graph-store shape into kernel packages just to make the adapter thinner.
- Do not introduce kernel concepts that exist only to mirror product execution-mode routing.

## Exit Criteria For This Wave

This RFC should be considered successfully addressed for the current wave when all of the following are true:

- request/session-level approval can block and resume original execution without product-side continuation glue
- program execution can use durable runtime handles as first-class inputs to later steps
- mixed program execution supports heterogeneous actions plus clear replay lineage
- newly added runtime behavior is restart-safe and idempotent
- maintainers have a written boundary that distinguishes current program-runtime growth from a future graph-runtime rewrite

If those items land, Nidus can delete a meaningful part of its approval bridge and interactive-graph glue without forcing `harness-core` to prematurely absorb a full graph workflow engine.
