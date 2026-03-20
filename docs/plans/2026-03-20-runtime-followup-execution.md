# Runtime Follow-Up Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use Markdown checkboxes for tracking.

**Goal:** Close the kernel-level gaps surfaced in the review by adding real approval/resume mechanics, richer execution facts and observability identifiers, and first-class context/capability extension points without leaking product concerns into `harness-core`.

**Architecture:** The work proceeds in dependency order. First, convert policy `ask` into a durable pending-approval state machine with explicit approval records and resume APIs. Second, expand runtime execution facts and event envelopes so replay, recovery, and observability have stable identifiers and structured records. Third, introduce core context budgets/compaction and capability snapshot foundations so the runtime owns these contracts instead of leaving them in docs and examples only.

**Tech Stack:** Go 1.24, in-memory stores plus Postgres repositories, runtime kernel packages under `pkg/harness/*`, WebSocket adapter, Go tests, Markdown docs.

---

## Execution Rule

Progress must be tracked in this file only:

- Start every pending task as unchecked
- Mark each task with `[x]` immediately after implementation and verification complete
- If a task reveals a prerequisite gap, add a new unchecked task directly below it before continuing
- If any task remains unchecked, the project is not complete

## Scope Guardrails

This plan covers generic kernel concerns only:

- approval flow, execution facts, context/capability contracts, and observability are in scope
- transport-neutral adapter wiring is in scope
- multi-user tenancy, UI projections, provider catalogs, and organization-specific approval UX remain out of scope

### Task 1: Approval And Resume Kernel

**Files:**
- Create: `pkg/harness/approval/*.go`
- Modify: `pkg/harness/runtime/interfaces.go`
- Modify: `pkg/harness/runtime/options.go`
- Modify: `pkg/harness/runtime/service.go`
- Modify: `pkg/harness/runtime/runner.go`
- Modify: `pkg/harness/session/state.go`
- Modify: `pkg/harness/persistence/*.go`
- Modify/Create: `internal/postgres/*`
- Modify: `internal/postgres/schema.sql`
- Modify: `adapters/websocket/server.go`
- Test: `pkg/harness/runtime/*approval*_test.go`
- Test: `adapters/websocket/*approval*_test.go`

- [x] Add failing runtime tests showing `permission.Ask` must persist a pending approval and must not execute the action
- [x] Run the new approval tests and verify they fail for the expected reasons
- [x] Introduce generic approval contracts (`Store`, `Record`, `Decision`, `ResumePolicy`) with in-memory implementations
- [x] Extend session/runtime state so pending approval blocks execution and records the approval handle on the session
- [x] Implement `RunStep` ask-path persistence, audit/event emission, and non-execution behavior
- [x] Add failing tests for approval response and resume behavior through the runtime API
- [x] Verify the response/resume tests fail for the expected reasons
- [x] Implement approval response APIs, resume semantics, and updated WebSocket transport commands
- [x] Add durable Postgres approval storage and repository wiring
- [x] Re-run targeted approval runtime and adapter tests until they pass

### Task 2: Execution Facts, Event Envelope, And Replay Foundations

**Files:**
- Create: `pkg/harness/execution/*.go`
- Modify: `pkg/harness/audit/event.go`
- Modify: `pkg/harness/observability/*.go`
- Modify: `pkg/harness/plan/spec.go`
- Modify: `pkg/harness/session/state.go`
- Modify: `pkg/harness/runtime/runner.go`
- Modify: `pkg/harness/runtime/service.go`
- Modify: `pkg/harness/persistence/*.go`
- Modify/Create: `internal/postgres/*`
- Modify: `internal/postgres/schema.sql`
- Test: `pkg/harness/runtime/*execution*_test.go`
- Test: `pkg/harness/runtime/event_stability_test.go`
- Test: `pkg/harness/runtime/recovery*_test.go`

- [x] Add failing tests for stable attempt/action/verification identifiers in runtime events and persisted execution records
- [x] Run the new execution-fact tests and verify they fail for the expected reasons
- [x] Introduce generic execution, verification, artifact, and runtime-handle store contracts with memory implementations
- [x] Expand event envelopes with `task_id`, `attempt_id`, `action_id`, `trace_id`, and `causation_id`
- [x] Update runtime step execution to create first-class attempt/action/verification records instead of encoding all facts only in step/audit payloads
- [x] Add durable Postgres schema/repository support for the new kernel records
- [x] Re-run targeted runtime recovery/event tests until they pass

### Task 3: Context Budgets, Compaction, And Capability Snapshots

**Files:**
- Create: `pkg/harness/capability/*.go`
- Modify/Create: `pkg/harness/runtime/context*.go`
- Modify: `pkg/harness/runtime/interfaces.go`
- Modify: `pkg/harness/runtime/options.go`
- Modify: `pkg/harness/tool/registry.go`
- Modify: `pkg/harness/tool/definition.go`
- Modify: `pkg/harness/runtime/service.go`
- Modify/Create: `internal/postgres/*`
- Modify: `internal/postgres/schema.sql`
- Modify: `examples/planner-context/main.go`
- Test: `pkg/harness/runtime/*context*_test.go`
- Test: `pkg/harness/tool/*_test.go`

- [x] Add failing tests for runtime loop budget defaults and context compaction entry points
- [x] Run the new context-budget tests and verify they fail for the expected reasons
- [x] Introduce typed context package/compactor contracts and add loop budget fields to runtime options
- [x] Add capability resolver/snapshot contracts so tool execution resolves a stable capability snapshot before invocation
- [x] Update the default registry/runtime wiring to honor enabled/version/risk metadata through capability resolution rather than direct name lookup
- [x] Add durable storage hooks for capability snapshots and compacted context summaries where the runtime needs persistence
- [x] Re-run targeted runtime/tool tests until they pass

### Task 4: Policy, EventSink, Module, And Docs Sync

**Files:**
- Modify: `pkg/harness/runtime/runner.go`
- Modify: `pkg/harness/runtime/interfaces.go`
- Modify: `pkg/harness/runtime/service.go`
- Modify: `pkg/harness/permission/*.go`
- Modify: `modules/*/module.go`
- Modify: `README.md`
- Modify: `docs/POLICY.md`
- Modify: `docs/RUNTIME.md`
- Modify: `docs/MODULES.md`
- Modify: `docs/EXTENSIBILITY.md`
- Modify: `docs/ARCHITECTURE.md`
- Modify: `docs/ROADMAP.md`

- [x] Add failing tests showing runtime events must flow through `EventSink`, not only direct audit writes
- [x] Run the EventSink tests and verify they fail for the expected reasons
- [x] Refactor runtime event emission so `Audit` is one sink implementation rather than a bypass path
- [x] Connect module `DefaultPolicyRules()` into a real composable default policy evaluator path
- [x] Update docs so approval/resume, execution facts, context/capability contracts, and out-of-scope platform concerns are accurately documented
- [x] Re-read the docs for contradictions and fix any remaining drift

### Task 5: Full Verification And Closeout

**Files:**
- Modify: `docs/plans/2026-03-20-runtime-followup-execution.md`

- [x] Run `go test ./...`
- [x] Run focused Postgres/runtime suites added by this work
- [x] Run any benchmark/doc commands introduced by this work and record representative output where docs require it
- [x] Mark completed plan items `[x]` only after fresh verification
- [x] Re-read this plan file and ensure no unchecked item remains
