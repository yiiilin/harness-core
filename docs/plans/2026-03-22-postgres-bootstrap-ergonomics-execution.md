# Postgres Bootstrap Ergonomics Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** improve the public Postgres embedding ergonomics without expanding kernel scope by adding schema-aware bootstrap helpers, clarifying public-vs-CLI config boundaries, and publishing a more realistic durable embedding example.

**Architecture:** keep runtime semantics in `pkg/harness/runtime` unchanged. Add only transport-neutral durable bootstrap helpers to `pkg/harness/postgres`; keep CLI env parsing in `internal/config`; keep adapter transport config separate; publish platform-facing examples and docs around the existing approval/restart control plane.

**Tech Stack:** Go 1.24, `pkg/harness/postgres`, `pkg/harness/runtime`, `internal/postgres/*`, `examples/*`, Markdown docs, Go tests

---

## Execution Rule

Progress must be tracked in this file only:

- Start every pending task as unchecked
- Mark each task with `[x]` immediately after implementation and verification complete
- If a task reveals a prerequisite gap, add a new unchecked task directly below it before continuing
- If any task remains unchecked, the project is not complete

## Scope Guardrails

In scope:
- schema-aware public Postgres bootstrap config and helpers
- connection-pool tuning on the public bootstrap path
- explicit docs separating embedder bootstrap config from CLI env loading
- a durable example showing external run mapping plus approval/restart continuation

Out of scope:
- user / tenant / auth / approval UI concepts in kernel types
- storage config leaking into adapter config
- product-specific planner / MCP / skill / tool-loop concepts in kernel
- opaque continuation storage in kernel durable schema for this round

### Task 1: Lock schema-aware bootstrap behavior with failing tests

**Files:**
- Modify: `pkg/harness/postgres/runtime_test.go`
- Create/Modify: example tests under `examples/*`

- [x] Add failing tests for `EnsureSchema(...)`, `OpenDBWithConfig(...)`, and `OpenServiceWithConfig(...)`.
- [x] Add failing tests proving non-`public` schema bootstrap works through `search_path` rather than copied SQL rewrites.
- [x] Add failing tests for config-driven pool tuning and optional migration-on-open behavior.
- [x] Add failing tests for a durable embedding example that survives restart and resumes after approval.

### Task 2: Implement public schema-aware Postgres bootstrap helpers

**Files:**
- Modify: `pkg/harness/postgres/runtime.go`
- Modify: `pkg/harness/postgres/doc.go`
- Modify: `pkg/harness/postgres/*_test.go`

- [x] Add a public `postgres.Config` covering DSN, Schema, pool knobs, and migration-on-open behavior.
- [x] Add `EnsureSchema(...)`, `OpenDBWithConfig(...)`, and `OpenServiceWithConfig(...)` without changing runtime semantics.
- [x] Make migration/version detection schema-aware under configured `search_path`.
- [x] Keep existing `OpenDB(...)` and `OpenService(...)` working as compatibility shims over the simpler path.

### Task 3: Publish a more realistic durable embedding example

**Files:**
- Create: `examples/platform-durable-embedding/*`
- Modify: `examples/README.md`

- [x] Add a platform-style durable example that maps external `run_id -> session_id`.
- [x] Show approval pause, service reopen, `RespondApproval(...)`, and `ResumePendingApproval(...)` continuing the same durable session.
- [x] Keep platform semantics outside kernel objects; use local wrapper structures only.
- [x] Add example verification tests.

### Task 4: Clarify public bootstrap config vs CLI reference config

**Files:**
- Modify: `docs/API.md`
- Modify: `docs/API.zh-CN.md`
- Modify: `docs/EMBEDDING.md`
- Modify: `docs/ADAPTERS.md`
- Modify: `docs/STATUS.md`
- Modify: `README.md`
- Modify: `README.zh-CN.md`

- [x] Document `pkg/harness/postgres.Config` as the embedder-facing durable bootstrap surface.
- [x] State explicitly that `internal/config` is a reference CLI env loader, not an embedder API.
- [x] Keep adapter config boundaries intact and documented.
- [x] Record that opaque continuation storage remains intentionally out of kernel scope for now.

### Task 5: Verification and closeout

**Files:**
- Modify: `docs/plans/2026-03-22-postgres-bootstrap-ergonomics-execution.md`

- [x] Run focused tests for `pkg/harness/postgres` and the new durable embedding example.
- [x] Run `cd adapters && go test ./... -count=1`.
- [x] Run `cd cmd/harness-core && go test ./... -count=1`.
- [x] Run `make test-workspace`.
- [x] Mark all completed items and leave no unchecked task.
