# Embedder VNext Adaptation Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Adapt the public embedder-facing surface to the approved vNext direction without prematurely forcing a new execution model into the kernel.

**Architecture:** Keep the existing runtime loop intact while landing the first safe slice of vNext work: public terminology, explicit current support boundaries, stable capability unsupported reason codes, and clean companion-module publish metadata. Defer fan-out, tool-graph, dataflow, and richer blocked-runtime execution semantics to later waves with dedicated model changes.

**Tech Stack:** Go 1.24, `pkg/harness/capability`, `pkg/harness/runtime`, `pkg/harness`, release tests, companion `go.mod` files, Markdown docs.

---

## Execution Rule

Progress must be tracked in this file only:

- Start every pending task as unchecked
- Mark each task with `[x]` immediately after implementation and verification complete
- If a task reveals a prerequisite gap, add a new unchecked task directly below it before continuing
- If any task remains unchecked, the project is not complete

## Scope Guardrails

- in scope now: terminology, current-support docs, public capability match/reason-code API, companion-module publish hygiene, release tests for those contracts
- out of scope now: native fan-out scheduler, tool graph runtime, artifact-native inputs, blocked-runtime store redesign, target-scoped replay model
- do not add product IDs, auth concepts, or platform workflow state into kernel types
- do not claim support for vNext execution features that are not actually implemented

### Task 1: Land The Approved VNext Spec And Execution Checklist

**Files:**
- Create: `docs/plans/2026-03-23-embedder-vnext-adaptation-spec.md`
- Create: `docs/plans/2026-03-23-embedder-vnext-adaptation-execution.md`

- [x] Add the approved vNext architecture/spec in a persistent repo document.
- [x] Record the implementation checklist in this plan file with wave-based scope and guardrails.

### Task 2: Lock Companion Module Publish Hygiene With Failing Release Coverage

**Files:**
- Modify: `release/release_test.go`

- [x] Add release coverage that fails when companion `go.mod` files contain repo-local `replace` directives.
- [x] Run the focused release test first and verify it fails against the current metadata.

### Task 3: Remove Repo-Local Replace Directives From Companion Module Metadata

**Files:**
- Modify: `modules/go.mod`
- Modify: `adapters/go.mod`
- Modify: `pkg/harness/builtins/go.mod`
- Modify: `cmd/harness-core/go.mod`

- [x] Remove repo-local `replace` directives from companion modules while preserving workspace-based local development through `go.work`.
- [x] Re-run the focused release metadata test and verify it passes.
- [x] Verify the committed `go.work` still covers local multi-module development after the metadata cleanup.

### Task 4: Add Public Capability Unsupported Reason Codes With TDD

**Files:**
- Modify: `pkg/harness/capability/types.go`
- Modify: `pkg/harness/capability/resolver.go`
- Modify: `pkg/harness/runtime/service.go`
- Modify: `pkg/harness/harness.go`
- Modify: `pkg/harness/tool/registry_resolution_test.go`
- Modify: `release/release_test.go`

- [x] Add failing tests for a public capability-matching API that returns stable unsupported reason codes rather than only raw resolution errors.
- [x] Lock the initial public reason-code set: `CAPABILITY_NOT_FOUND`, `CAPABILITY_DISABLED`, `CAPABILITY_VERSION_NOT_FOUND`, `CAPABILITY_VIEW_NOT_FOUND`, `CAPABILITY_VIEW_DRIFT`, `MULTI_TARGET_FANOUT_UNSUPPORTED`, `PREPLANNED_TOOL_GRAPH_UNSUPPORTED`, `INTERACTIVE_REOPEN_UNSUPPORTED`, and `ARTIFACT_INPUT_UNSUPPORTED`.
- [x] Implement public request requirements, unsupported reason code types, and matching behavior without breaking existing `ResolveCapability(...)`.
- [x] Expose the matching API through the public facade and release-surface compile coverage.
- [x] Re-run focused capability/release tests and verify they pass.

### Task 5: Publish A Public Support Matrix For Embedders

**Files:**
- Create: `docs/EMBEDDER_VNEXT.md`
- Modify: `docs/CURRENT_STATE.md`
- Modify: `docs/EMBEDDING.md`
- Modify: `docs/API.md`
- Modify: `docs/ROADMAP.md`

- [x] Document the accepted terminology: `execution target`, `target-scoped action`, `blocked runtime`.
- [x] Publish a concrete support matrix table that distinguishes: supported today, partial today, planned vNext, and explicitly not implemented yet.
- [x] Link the new document from the existing current-state, embedding, API, and roadmap docs.

### Task 6: Verification And Checklist Sync

**Files:**
- Modify: `docs/plans/2026-03-23-embedder-vnext-adaptation-execution.md`

- [x] Run focused verification for release metadata and capability matching.
- [x] Run `go test ./release ./pkg/harness/capability ./pkg/harness/tool ./pkg/harness/runtime -count=1`.
- [x] Mark completed tasks `[x]` only after fresh verification.
- [x] Re-read this plan file and ensure unchecked items reflect actual remaining work.
