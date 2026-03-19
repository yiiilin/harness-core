# ROADMAP.md

## Goal

Track the path from the current prototype state to a **project-usable alpha** for `harness-core`.

This roadmap is intentionally execution-oriented:
- every item should become a concrete implementation task
- implemented items should be checked off
- items are grouped by delivery priority

---

## Scope decision

For the next phase, we will complete:
- **P0** (must-have for project-usable alpha)
- **P1** (high-priority follow-up for near-term use)
- **advanced planner/context work**

We will **not** prioritize P2 at this time.

---

## P0 — required before serious project use

### Persistence and durability
- [x] Define store interfaces for session/task/plan/audit
- [x] Introduce `RepositorySet`
- [x] Introduce `UnitOfWork` / `Runner`
- [x] Introduce generic `TransactionalRunner`
- [ ] Create `internal/postgres/` package layout
- [ ] Add initial Postgres schema / migration skeleton
- [ ] Implement Postgres-backed session repository
- [ ] Implement Postgres-backed task repository
- [ ] Implement Postgres-backed plan repository
- [ ] Implement Postgres-backed audit repository
- [ ] Implement Postgres-backed `TxManager`
- [ ] Implement Postgres-backed `TxRepositoryFactory`

### Runtime consistency
- [x] Make `RunStep()` aware of persistence boundary semantics
- [x] Route key state updates through `Runner.Within(...)`
- [ ] Route **all** step-related state mutations through `Runner.Within(...)`
- [ ] Ensure audit writes are included in the same persistence boundary
- [ ] Add tests for transaction success and rollback behavior through runtime paths

### Recovery baseline
- [ ] Define interrupted / in-flight step semantics
- [ ] Persist enough metadata to identify interrupted sessions after restart
- [ ] Add recovery-oriented runtime read path
- [ ] Add restart-safe integration tests

### End-to-end project-usable alpha checks
- [ ] Add Postgres-backed WebSocket happy-path E2E
- [ ] Add Postgres-backed deny-path E2E
- [ ] Add restart-read E2E

---

## P1 — important soon after alpha enablement

### Shell hardening
- [x] Shell pipe executor exists
- [x] Shell backend abstraction exists
- [x] Shell sandbox hook abstraction exists
- [ ] Add shell output truncation policy to runtime path
- [ ] Add shell cwd/path allowlist support
- [ ] Stabilize shell error code taxonomy
- [ ] Add shell timeout-path benchmark documentation/examples

### API / versioning discipline
- [x] Public facade exists in `pkg/harness`
- [x] Package boundary guidance exists
- [ ] Add explicit API stability notes per package group
- [ ] Add `VERSIONING.md`
- [ ] Add `CHANGELOG.md`
- [ ] Define deprecation / breaking-change policy

### Event / metrics stability
- [x] Event types documented
- [x] Metrics snapshot exists
- [x] Runtime metrics endpoint exists in WebSocket adapter
- [ ] Stabilize event id strategy
- [ ] Add event ordering assertions beyond current path tests
- [ ] Add event volume benchmark
- [ ] Add benchmark usage examples with sample output in docs

---

## Advanced planner / context (explicitly included in current push)

### Planner
- [x] `Planner` interface exists
- [x] `NoopPlanner` exists
- [x] `DemoPlanner` exists
- [ ] Add a structured multi-step planner example
- [ ] Add plan revision / replan example using planner output
- [ ] Add planner-driven happy-path integration test (beyond direct step injection)
- [ ] Add planner failure-path test

### Context assembly
- [x] `ContextAssembler` interface exists
- [x] default context assembler exists
- [ ] Add a richer context assembler example with layered context sections
- [ ] Add context assembly tests that assert minimal/expected fields
- [ ] Add context compaction / reduction helper examples

### Documentation
- [x] `PLANNER_CONTEXT.md` exists
- [ ] Add planner/context usage examples to `API.md`
- [ ] Add planner/context examples to `README.md`

---

## P2 — intentionally deferred for now

- [ ] PTY shell execution
- [ ] Windows-native module
- [ ] Knowledge / retrieval module
- [ ] Multi-process / distributed runtime
- [ ] Advanced tenant/user auth
- [ ] Production-grade approval workflow persistence

---

## Milestone definition: "project-usable alpha"

`harness-core` may be considered ready for controlled project use when all of the following are true:

- [ ] Postgres repositories are implemented for session/task/plan/audit
- [ ] `RunStep()` uses the persistence boundary for all critical state updates
- [ ] WebSocket + Postgres happy-path E2E passes
- [ ] WebSocket + Postgres deny-path E2E passes
- [ ] Restart-read recovery baseline passes
- [ ] Shell/filesystem/http modules remain green
- [ ] Core docs reflect actual runtime behavior

---

## Operating rule

Unless there is a strong reason to change direction:

1. finish P0
2. finish P1
3. finish advanced planner/context work
4. defer P2

This roadmap is the source of truth for the next implementation phase.
