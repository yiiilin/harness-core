# Master Kernel Gap Checklist

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Track the remaining pure-kernel and embedder-facing gaps after the current embedder-vNext work, while keeping non-kernel product concepts out of `pkg/harness/*`.

**Architecture:** Prioritize transport-neutral extension points and runtime semantics that materially improve embedding ergonomics without expanding kernel scope. Close smaller public-hook gaps first, then document and isolate the larger redesign items that still remain.

**Tech Stack:** Go 1.24, `pkg/harness/*`, companion modules, release tests, Markdown docs.

---

## Execution Rule

Progress must be tracked in this file only:

- Start every pending task as unchecked
- Mark each task with `[x]` immediately after implementation and verification complete
- If a task reveals a prerequisite gap, add a new unchecked task directly below it before continuing
- If any task remains unchecked, the project is not complete

## Scope Rule

The checklist is constrained by `docs/KERNEL_SCOPE.md`.

In scope:

- transport-neutral runtime hooks
- runtime correctness and replay stability
- durable execution-state semantics
- embedder-facing public helpers that do not introduce product identity or transport semantics

Out of scope:

- user / tenant / org identity
- auth and gateway concerns
- UI projections and approval consoles
- billing, quotas, provider routing, or business workflow semantics
- transport-specific request / response contracts

## Inputs

This checklist consolidates open items from:

- `docs/EMBEDDER_VNEXT_REALITY_CHECK.md`
- `docs/V1_RELEASE_CHECKLIST.md`
- `docs/CURRENT_STATE.md`
- `docs/STATUS.md`
- `docs/KERNEL_SCOPE.md`

## Task 1: Add discovery-backed `fanout_all` target resolution

**Goal:** make `TargetSelectionFanoutAll` usable through a transport-neutral public resolver contract without introducing product-specific discovery semantics.

**Files:**
- Modify: `pkg/harness/runtime/interfaces.go`
- Modify: `pkg/harness/runtime/options.go`
- Modify: `pkg/harness/runtime/service.go`
- Modify: `pkg/harness/runtime/program.go`
- Test: `pkg/harness/runtime/program_test.go`
- Modify: `docs/EMBEDDER_VNEXT.md`
- Modify: `docs/EMBEDDER_VNEXT_REALITY_CHECK.md`

- [x] Add a public runtime target-resolution interface and service/options wiring.
- [x] Resolve `fanout_all` nodes through that hook during program-plan compilation.
- [x] Add regression tests for resolver-backed `fanout_all` plan creation and execution.
- [x] Update docs to move `fanout_all` from “unsupported” to “resolver-backed”.

## Task 2: Add attachment materialization as a real runtime behavior

**Goal:** make `AttachmentInput.Materialize` do real work through a public, transport-neutral hook and a default temp-file implementation.

**Files:**
- Modify: `pkg/harness/runtime/interfaces.go`
- Modify: `pkg/harness/runtime/options.go`
- Modify: `pkg/harness/runtime/service.go`
- Modify: `pkg/harness/runtime/program_binding_resolution.go`
- Test: `pkg/harness/runtime/program_test.go`
- Modify: `docs/EMBEDDER_VNEXT.md`
- Modify: `docs/EMBEDDER_VNEXT_REALITY_CHECK.md`

- [x] Add a public attachment-materialization interface and service/options wiring.
- [x] Implement default temp-file materialization for inline text / bytes attachments.
- [x] Add runtime support for materialized attachment bindings during program execution.
- [x] Add regression tests for inline attachment materialization and artifact-backed materialization where supported.
- [x] Update docs to reflect the implemented materialization subset precisely.

## Task 3: Lock the interactive control-plane boundary

**Goal:** keep kernel-owned interactive state, replay, and handle control explicit while preventing PTY/view/write/close backend behavior from leaking into core by accident.

**Files:**
- Modify: `docs/KERNEL_SCOPE.md`
- Modify: `docs/CURRENT_STATE.md`
- Modify: `docs/STATUS.md`
- Modify: `docs/EMBEDDER_VNEXT.md`
- Modify: `docs/EXTENSIBILITY.md`

- [x] Document that kernel-owned interactive surface is runtime-handle and interactive-state persistence, not PTY backend operations themselves.
- [x] Document that start / reopen / view / write / close backends remain module or embedder responsibilities unless a future transport-neutral core contract is explicitly introduced.

## Task 4: Add release guardrails for companion-module tagging hygiene

**Goal:** reduce downstream `go mod tidy` surprises by making companion-module release expectations explicit and guarded in-repo.

**Files:**
- Modify: `release/release_test.go`
- Modify: `docs/VERSIONING.md`
- Modify: `docs/RELEASING.md`
- Modify: `docs/V1_RELEASE_CHECKLIST.md`

- [x] Add a release-oriented guard that verifies companion modules do not carry placeholder versions or repo-local replace directives.
- [x] Add a release-oriented guard or documented procedure for publishing companion-module tags alongside compatible root releases.
- [x] Update public docs so embedders know root `v1.x` and companion-module `v0.x` are separate release tracks.

## Task 5: Re-baseline status docs after the above surface work

**Goal:** keep repository guidance honest once the smaller public-surface gaps above are closed.

**Files:**
- Modify: `docs/CURRENT_STATE.md`
- Modify: `docs/STATUS.md`
- Modify: `docs/EMBEDDER_VNEXT_REALITY_CHECK.md`

- [x] Update the current-state and status docs to reflect completed Tasks 1-4.
- [x] Keep the remaining larger redesign gaps explicit instead of overstating runtime maturity.

## Task 6: Track the remaining major kernel redesign gaps explicitly

**Goal:** keep the two largest still-open runtime gaps visible without quietly expanding kernel scope or pretending they are already solved.

**Files:**
- Modify: `docs/EMBEDDER_VNEXT_REALITY_CHECK.md`
- Modify: `docs/CURRENT_STATE.md`
- Modify: `docs/STATUS.md`

- [ ] Define the remaining true concurrent multi-target scheduler gap precisely, including why current logical fan-out is not the same thing.
- [ ] Add runtime-facing scheduler contracts that can execute one fan-out group across multiple targets with a real concurrency limit.
- [ ] Teach program compilation/runtime execution to preserve fan-out group identity for a scheduler-owned execution round instead of only step expansion.
- [ ] Persist per-target facts and aggregate results from the scheduler path without regressing replay/projection stability.
- [ ] Add focused runtime tests that prove `TargetSelection.MaxConcurrency` is actually consumed at execution time.
- [x] Define the remaining generic blocked-runtime lifecycle gap precisely, including why approval-backed projection is only a subset today.
- [x] Add a first-class generic blocked-runtime record store and session blocked-state marker without introducing product-specific semantics.
- [x] Add public generic blocked-runtime lifecycle APIs for create, respond, resume, abort, and durable lookup by blocked-runtime ID.
- [x] Extend blocked-runtime reads and projection logic so generic blocked runtimes appear alongside approval-backed blocked runtimes.
- [x] Make blocked sessions non-runnable until generic blocked-runtime resume or abort clears the state.
- [x] Add focused runtime tests for generic blocked-runtime lifecycle, lookup, projection, and claim semantics.
- [x] Keep these items open until they are implemented with code, tests, and docs rather than closing them as documentation-only work.

## Task 6B: Add a transport-neutral interactive control plane

**Goal:** move start / reopen / view / write / close into a kernel-owned transport-neutral controller contract while keeping PTY-specific attach/resize and other backend details outside core.

**Files:**
- Modify: `pkg/harness/runtime/interfaces.go`
- Modify: `pkg/harness/runtime/options.go`
- Modify: `pkg/harness/runtime/service.go`
- Modify: `pkg/harness/runtime/interactive_control.go`
- Modify: `pkg/harness/runtime/errors.go`
- Modify: `pkg/harness/harness.go`
- Modify: `pkg/harness/audit/event.go`
- Test: `pkg/harness/runtime/interactive_runtime_test.go`
- Modify: `modules/shell/interactive_controller.go`
- Test: `modules/shell/pty_test.go`
- Modify: `pkg/harness/builtins/builtins.go`
- Test: `pkg/harness/builtins/builtins_test.go`
- Modify: `docs/API.md`
- Modify: `docs/CURRENT_STATE.md`
- Modify: `docs/STATUS.md`
- Modify: `docs/EMBEDDER_VNEXT.md`
- Modify: `docs/EMBEDDER_VNEXT_REALITY_CHECK.md`
- Modify: `docs/KERNEL_SCOPE.md`
- Modify: `docs/EXTENSIBILITY.md`

- [x] Add a public `InteractiveController` runtime hook plus public start/reopen/view/write/close request and result contracts.
- [x] Add kernel runtime service APIs that persist runtime-handle state and audit events around interactive control operations.
- [x] Add a shell-module PTY controller implementation that can back the new kernel control plane through a shared `PTYManager`.
- [x] Wire the default builtins composition helper so the default shell PTY path also satisfies the new interactive control-plane surface.
- [x] Update docs to clarify that the control-plane contract is now in core while PTY attach/resize and other backend-specific behavior remain outside.

## Task 7: Final verification and checklist synchronization

**Files:**
- Modify: `docs/plans/2026-03-24-master-kernel-gap-checklist.md`

- [x] Run focused tests for every completed task before marking it complete.
- [x] Run `go test ./release ./pkg/harness/runtime -count=1`.
- [x] Re-read this file and ensure every `[x]` has matching implementation and verification evidence.
