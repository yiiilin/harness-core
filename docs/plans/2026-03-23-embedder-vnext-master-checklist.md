# Embedder VNext Master Checklist

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Track all remaining embedder-vNext kernel work from the currently accepted architecture, and execute it as a sequence of bounded slices.

**Architecture:** Preserve the existing single-step runtime path while incrementally adding public model contracts first, then runtime engine semantics, then richer projection and interactive durability. Completed slices should remain small, typed, transport-neutral, and explicit about what is and is not implemented yet.

**Tech Stack:** Go 1.24, `pkg/harness/*`, release compile coverage, runtime tests, Markdown docs.

---

## Execution Rule

Progress must be tracked in this file only:

- Start every pending task as unchecked
- Mark each task with `[x]` immediately after implementation and verification complete
- If a task reveals a prerequisite gap, add a new unchecked task directly below it before continuing
- If any task remains unchecked, the project is not complete

## Completed Foundations

- [x] Wave 1 public terminology / support-matrix docs
- [x] Capability matching with stable unsupported reason codes
- [x] Companion-module publish hygiene and release guardrails
- [x] Approval-backed blocked-runtime lookup/projection surface

## Wave 2: Public Model Layer

- [x] Add `execution target` public contracts and facade re-exports.
- [x] Add artifact / attachment input public contracts.
- [x] Add stable output / artifact / attachment reference contracts.
- [x] Add public preplanned execution-program / tool-graph contracts.
- [x] Extend public projection contracts for target slices and richer blocked-runtime views.
- [x] Add generic blocked-runtime contract types beyond the current approval-backed subset.

## Wave 3: Runtime Engine

- [x] Execute preplanned non-shell tool graph natively through the runtime.
- [x] Add native multi-target fan-out scheduling inside one logical runtime execution.
- [x] Add target-scoped execution facts for actions / verifications / artifacts.
- [x] Add partial-failure strategy, per-target retry, and aggregate result semantics.
- [x] Add unified verification scopes across step / target / aggregate execution.
- [x] Add stable step-to-step dataflow using structured outputs and artifact references.

## Wave 4: Interactive / Projection Strengthening

- [x] Strengthen durable interactive runtime lifecycle beyond current runtime-handle subset.
- [x] Add richer interactive reopen/view/write/close projection semantics.
- [x] Extend replay/projection helpers for target slices, blocked-runtime progression, and interactive state.

## Docs / Examples / Release Discipline

- [x] Add example coverage for public model-layer contracts once the first public types land.
- [x] Add eval/release scenarios for the new execution-model slices as they become real runtime behavior.
- [x] Keep `docs/EMBEDDER_VNEXT.md` and `docs/API.md` aligned after every completed slice.
