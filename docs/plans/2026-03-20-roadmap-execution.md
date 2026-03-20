# Roadmap Execution Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use Markdown checkboxes for tracking.

**Goal:** Implement every remaining non-P2 item in `docs/ROADMAP.md` and update the roadmap only after code, tests, and docs are actually complete.

**Architecture:** The work proceeds in three waves. First, wire the shipped runtime/server path to support durable Postgres-backed operation and prove it with end-to-end tests. Second, harden shell/runtime/event/versioning contracts. Third, complete planner/context examples, tests, and user-facing docs so the shipped defaults, examples, and public documentation agree.

**Tech Stack:** Go 1.24, `database/sql`, Postgres-backed repositories, Gorilla WebSocket, Go tests/benchmarks, Markdown docs.

---

## Execution Rule

Progress must be tracked in this file only:

- Start every pending task as unchecked
- Mark each task with `[x]` immediately after implementation and verification complete
- If a task reveals a prerequisite gap, add a new unchecked task directly below it before continuing
- If any task remains unchecked, the project is not complete

### Task 1: Durable Runtime Bootstrap

**Files:**
- Modify: `internal/config/config.go`
- Modify: `cmd/harness-core/main.go`
- Modify: `pkg/harness/runtime/service.go`
- Create/Modify: `internal/postgres/*.go`
- Test: `cmd/harness-core/*_test.go` or `internal/config/*_test.go`

- [x] Add failing tests for durable config parsing and storage mode reporting
- [x] Run the targeted tests and verify they fail for the expected reasons
- [x] Implement runtime mode/DSN config and Postgres runtime wiring
- [x] Re-run the targeted tests and make them pass

### Task 2: Postgres Integration Harness And Alpha E2E

**Files:**
- Create: `internal/postgres/testutil/*.go` or equivalent focused helper
- Modify: `adapters/websocket/*_test.go`
- Modify: `pkg/harness/runtime/recovery_test.go` or add durable recovery E2E coverage
- Test: `adapters/websocket/*_test.go`, `pkg/harness/runtime/*_test.go`

- [x] Add failing Postgres-backed happy-path E2E
- [x] Verify the new happy-path test fails correctly
- [x] Implement reusable Postgres test setup and supporting wiring
- [x] Re-run happy-path E2E until it passes
- [x] Add failing Postgres-backed deny-path E2E
- [x] Verify the new deny-path test fails correctly
- [x] Implement deny-path coverage against the durable runtime
- [x] Re-run deny-path E2E until it passes
- [x] Add failing restart-read durable E2E
- [x] Verify the new restart-read test fails correctly
- [x] Implement any missing restart-read durable wiring
- [x] Re-run restart-read E2E until it passes

### Task 3: Shell Hardening

**Files:**
- Modify: `pkg/harness/executor/shell/contracts.go`
- Modify: `pkg/harness/executor/shell/pipe.go`
- Modify: `pkg/harness/runtime/*_test.go`
- Modify: `docs/EVAL.md`

- [x] Add failing tests for shell output truncation
- [x] Verify truncation tests fail correctly
- [x] Implement truncation and metadata reporting
- [x] Re-run truncation tests until they pass
- [x] Add failing tests for cwd/path allowlist enforcement
- [x] Verify allowlist tests fail correctly
- [x] Implement cwd/path allowlist enforcement
- [x] Re-run allowlist tests until they pass
- [x] Add failing tests for shell error taxonomy and timeout classification
- [x] Verify taxonomy tests fail correctly
- [x] Implement stable shell error codes/status mapping
- [x] Re-run taxonomy tests until they pass
- [x] Add timeout benchmark documentation updates
- [x] Verify docs/bench command examples match the codebase

### Task 4: Event And Metrics Stability

**Files:**
- Modify: `pkg/harness/runtime/runner.go`
- Modify: `pkg/harness/runtime/*_test.go`
- Modify: `pkg/harness/runtime/*bench*_test.go`
- Modify: `docs/EVAL.md`
- Modify: `docs/EVENTS.md`

- [x] Add failing tests for audit event IDs
- [x] Verify event ID tests fail correctly
- [x] Implement stable event ID population
- [x] Re-run event ID tests until they pass
- [x] Add failing tests for event ordering on success/deny/verify-failure flows
- [x] Verify ordering tests fail correctly
- [x] Implement or adjust ordering behavior/tests until they pass
- [x] Add event-volume benchmark coverage
- [x] Verify benchmark commands run successfully
- [x] Update docs with benchmark usage examples and sample output

### Task 5: Planner And Context Completion

**Files:**
- Modify: `pkg/harness/runtime/*.go`
- Modify: `pkg/harness/runtime/*_test.go`
- Modify/Create: `examples/*`
- Modify: `docs/PLANNER_CONTEXT.md`

- [x] Add failing tests for planner-driven runtime happy path
- [x] Verify planner-driven tests fail correctly
- [x] Implement planner-driven runtime entry/helpers needed by the tests
- [x] Re-run planner-driven tests until they pass
- [x] Add failing tests for planner failure path
- [x] Verify planner failure-path tests fail correctly
- [x] Implement planner failure-path behavior/docs needed by the tests
- [x] Re-run planner failure-path tests until they pass
- [x] Add failing tests for richer context assembler shape
- [x] Verify richer context tests fail correctly
- [x] Implement richer context example and compaction helper examples
- [x] Re-run richer context tests until they pass
- [x] Add/refresh multi-step planner and replan examples
- [x] Verify examples build or run as documented

### Task 6: API, Versioning, Changelog, And User Docs

**Files:**
- Modify: `README.md`
- Modify: `docs/API.md`
- Modify: `docs/STATUS.md`
- Modify: `docs/ROADMAP.md`
- Modify: `internal/postgres/README.md`
- Create: `VERSIONING.md`
- Create: `CHANGELOG.md`

- [x] Update API stability notes and planner/context usage examples
- [x] Add versioning and deprecation policy docs
- [x] Add initial changelog entries for shipped user-visible milestones
- [x] Sync README/status/postgres docs with actual runtime behavior
- [x] Re-read the docs for contradictions and fix any remaining drift

### Task 7: Full Verification And Roadmap Closeout

**Files:**
- Modify: `docs/ROADMAP.md`
- Modify: `docs/plans/2026-03-20-roadmap-execution.md`

- [x] Run `go test ./...`
- [x] Run `go test -run '^$' -bench RunStep -benchmem ./pkg/harness/runtime`
- [x] Run the durable-path Postgres-focused test command(s)
- [x] Mark completed roadmap items as `[x]` based on fresh verification only
- [x] Re-read this plan file and ensure no unchecked item remains
