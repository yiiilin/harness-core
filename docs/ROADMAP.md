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

## Current status snapshot

The project has already completed the core architectural shift that originally
blocked serious use:
- the runtime now has an explicit persistence boundary
- Postgres-backed repositories now exist for session/task/plan/audit
- transaction wiring exists via `TxManager` + `TxRepositoryFactory`
- step-related runtime mutations now flow through the runner boundary
- interrupted-session semantics and restart-oriented read paths now exist

That means the remaining alpha blockers are no longer "invent the core design".
They are mostly:
- prove the durable path end-to-end
- harden shell/runtime/API contracts for near-term consumers
- close the planner/context example and documentation gaps

Resolved during this phase:
- the shipped adapter/bootstrap path can now run in in-memory mode or Postgres mode
- config now exposes durable runtime selection and Postgres DSN wiring
- durable storage is now exercised through shipped server/runtime integration tests
- approval / resume is now a real kernel state machine with durable storage
- execution facts, capability snapshots, and planner-context compaction hooks now exist in core contracts

---

## Recommended execution waves

### Wave 1 — close alpha proof gaps

Do these first, in order:

1. add Postgres runtime/bootstrap wiring for the server path
2. extend config so tests and local runs can select a durable runtime
3. add a reusable Postgres integration-test harness
4. add Postgres-backed WebSocket happy-path E2E
5. add Postgres-backed WebSocket deny-path E2E
6. add restart-read E2E using a fresh runtime instance
7. treat those tests as the alpha gate

Reasoning:
- all three remaining P0 items depend on durable-path test setup
- the current shipped server path does not yet instantiate the Postgres-backed runtime
- these tests prove the current persistence architecture actually works from the public adapter boundary
- until this is green, P1 work improves the project but does not finish alpha enablement

### Wave 2 — harden near-term consumer contracts

Once the alpha gate exists:
- finish shell hardening items that affect runtime safety and predictability
- publish API/versioning discipline so consumers know what is stable
- tighten event/metrics guarantees enough for tooling and observability consumers

### Wave 3 — finish planner/context examples and docs

After the durable/runtime contract is proven:
- add richer examples
- add planner/context-driven integration coverage
- reflect those examples in `README.md` and `docs/API.md`

This keeps the kernel focused: prove correctness first, then improve usability.

---

## P0 — required before serious project use

### Persistence and durability
- [x] Define store interfaces for session/task/plan/audit
- [x] Introduce `RepositorySet`
- [x] Introduce `UnitOfWork` / `Runner`
- [x] Introduce generic `TransactionalRunner`
- [x] Create `internal/postgres/` package layout
- [x] Add initial Postgres schema / migration skeleton
- [x] Implement Postgres-backed session repository
- [x] Implement Postgres-backed task repository
- [x] Implement Postgres-backed plan repository
- [x] Implement Postgres-backed audit repository
- [x] Implement Postgres-backed `TxManager`
- [x] Implement Postgres-backed `TxRepositoryFactory`

### Runtime consistency
- [x] Make `RunStep()` aware of persistence boundary semantics
- [x] Route key state updates through `Runner.Within(...)`
- [x] Route **all** step-related state mutations through `Runner.Within(...)`
- [x] Ensure audit writes are included in the same persistence boundary
- [x] Add tests for transaction success and rollback behavior through runtime paths

### Recovery baseline
- [x] Define interrupted / in-flight step semantics
- [x] Persist enough metadata to identify interrupted sessions after restart
- [x] Add recovery-oriented runtime read path
- [x] Add restart-safe integration tests

### End-to-end project-usable alpha checks
- [x] Add Postgres-backed WebSocket happy-path E2E
- [x] Add Postgres-backed deny-path E2E
- [x] Add restart-read E2E

Completion criteria for this section:
- tests must use the real Postgres runner/repository wiring, not in-memory substitutes
- tests must assert persisted state, not only immediate handler responses
- restart-read must boot a fresh runtime/service against the same database state
- the server/bootstrap path used by the tests must be able to construct a durable runtime from config
- runtime info should not claim `in-memory-dev` when the service is actually running against Postgres
- the setup must be repeatable in CI from an empty schema
- once these pass, they become the main alpha release gate

---

## P1 — important soon after alpha enablement

### Shell hardening
- [x] Shell pipe executor exists
- [x] Shell backend abstraction exists
- [x] Shell sandbox hook abstraction exists
- [x] Add shell output truncation policy to runtime path
- [x] Add shell cwd/path allowlist support
- [x] Stabilize shell error code taxonomy
- [x] Add shell timeout-path benchmark documentation/examples

Completion notes:
- truncation policy should cap runtime-returned output and persisted/audited output while preserving truncation metadata
- cwd/path allowlist should be enforced before process start and be configurable at executor construction/runtime wiring time
- error taxonomy should clearly separate timeout / spawn failure / policy denial / non-zero exit / verification failure classes
- timeout benchmark docs should include the command used, representative output, and how to interpret the numbers

### API / versioning discipline
- [x] Public facade exists in `pkg/harness`
- [x] Package boundary guidance exists
- [x] Add explicit API stability notes per package group
- [x] Add `VERSIONING.md`
- [x] Add `CHANGELOG.md`
- [x] Define deprecation / breaking-change policy

Completion notes:
- `docs/API.md` should classify package groups into stable / evolving / internal-only expectations
- `VERSIONING.md` should explain pre-1.0 behavior and how minor/patch releases are interpreted
- `CHANGELOG.md` should record user-visible runtime/API/documentation changes rather than internal churn
- deprecation policy should define notice expectations before removing or reshaping public surface

### Event / metrics stability
- [x] Event types documented
- [x] Metrics snapshot exists
- [x] Runtime metrics endpoint exists in WebSocket adapter
- [x] Stabilize event id strategy
- [x] Add event ordering assertions beyond current path tests
- [x] Add event volume benchmark
- [x] Add benchmark usage examples with sample output in docs

Completion notes:
- event ids should be deterministic enough for tracing/debugging and unique enough for downstream consumers
- ordering assertions should cover at least success, deny, and verification-failure flows
- volume benchmarks should measure runtime event emission cost separately from tool execution cost
- docs should show the benchmark command and a small sample of expected output shape

---

## Advanced planner / context (explicitly included in current push)

### Planner
- [x] `Planner` interface exists
- [x] `NoopPlanner` exists
- [x] `DemoPlanner` exists
- [x] Add a structured multi-step planner example
- [x] Add plan revision / replan example using planner output
- [x] Add planner-driven happy-path integration test (beyond direct step injection)
- [x] Add planner failure-path test

Completion notes:
- the structured example should show a planner deriving more than one step, not just one direct shell action
- the replan example should demonstrate a revision reason and visible plan-state change
- planner integration coverage should exercise the `Planner` interface from runtime entry to persisted/runtime-visible outcome
- failure-path coverage should show what happens when planning cannot derive a valid next step

### Context assembly
- [x] `ContextAssembler` interface exists
- [x] default context assembler exists
- [x] Add a richer context assembler example with layered context sections
- [x] Add context assembly tests that assert minimal/expected fields
- [x] Add context compaction / reduction helper examples

Completion notes:
- richer examples should include task/session/core state plus at least one derived or layered section
- tests should guard against accidental context bloat and accidental omission of required fields
- compaction examples should show reduction strategy, not only describe it abstractly

### Documentation
- [x] `PLANNER_CONTEXT.md` exists
- [x] Add planner/context usage examples to `API.md`
- [x] Add planner/context examples to `README.md`

Documentation rule for this section:
- do not mark planner/context items done unless example code, tests, and user-facing docs all agree on the same construction path

---

## Concrete task breakdown

This section turns the remaining checklist into directly assignable implementation slices.

### P0 alpha proof chain

1. durable runtime bootstrap
   - add a small constructor/wiring path that builds:
     - SQL `DB`
     - Postgres repositories
     - Postgres `TxManager`
     - Postgres-backed `TransactionalRunner`
   - likely touch points:
     - `internal/config/`
     - `internal/postgres/`
     - `cmd/harness-core/main.go`
     - `pkg/harness/runtime/service.go`
   - minimum observable outcome:
     - the server can be started in in-memory mode or Postgres mode
     - `runtime.info` reflects the selected storage mode

2. Postgres test harness
   - provide a repeatable integration helper that:
     - acquires a Postgres database/DSN
     - applies schema/migrations
     - constructs a runtime backed by the real Postgres stores
     - cleans state between tests
   - design choice to make explicit:
     - env-provided DSN with skip semantics
     - or a test-only ephemeral database strategy
   - minimum observable outcome:
     - happy-path, deny-path, and restart-read tests share the same setup utility instead of duplicating DB bootstrap logic

3. Postgres-backed WebSocket happy-path E2E
   - start the WebSocket adapter against the durable runtime
   - create session/task
   - attach task
   - execute one successful shell step
   - assert:
     - response payload indicates success
     - session/task/plan state persisted as terminal success
     - audit trail persisted

4. Postgres-backed WebSocket deny-path E2E
   - run the same adapter boundary with a deny policy
   - execute a denied step
   - assert:
     - request returns a structured result instead of crashing
     - session/task reflect failure-safe deny handling
     - `policy.denied` audit record is persisted

5. restart-read E2E
   - use runtime instance A to create durable state
   - simulate restart by constructing runtime instance B against the same database
   - assert:
     - recoverable/interrupted state is still visible
     - read APIs return the same durable state after restart

### P1 shell hardening

1. output truncation
   - likely touch points:
     - `pkg/harness/executor/shell/pipe.go`
     - runtime paths that return/persist action output
   - required assertions:
     - stdout/stderr are capped deterministically
     - truncation metadata survives in action result/audit-visible data

2. cwd/path allowlist
   - likely touch points:
     - shell request contract
     - shell executor runtime checks
     - policy/extensibility docs
   - required assertions:
     - disallowed cwd/path is rejected before process launch
     - allowed path still behaves as before

3. shell error taxonomy
   - likely touch points:
     - `pkg/harness/executor/shell/pipe.go`
     - runtime tests that check structured failure output
   - required assertions:
     - timeout and spawn/setup failures are distinguishable from command exit failures
     - downstream tests/docs can rely on stable error codes

4. timeout-path benchmark docs
   - likely touch points:
     - runtime benchmark tests
     - `docs/EVAL.md`
     - `README.md` or module-specific docs if needed

### API / versioning discipline

1. package-group stability notes
   - classify at least:
     - `pkg/harness`
     - lower-level public subpackages
     - `internal/*`
   - destination:
     - `docs/API.md`

2. versioning policy
   - destination:
     - `VERSIONING.md`
   - must explain:
     - current pre-1.0 expectations
     - what counts as breaking at facade level
     - how documentation-only and internal-only changes are treated

3. changelog
   - destination:
     - `CHANGELOG.md`
   - minimum starting scope:
     - recent persistence/runtime/recovery milestones that changed user-visible behavior or documentation

4. deprecation policy
   - may live in:
     - `VERSIONING.md`
     - or a short dedicated section referenced from `docs/API.md`

### Event / metrics stability

1. event id strategy
   - current gap:
     - audit events define `event_id` but runtime emission does not populate it consistently
   - work needed:
     - choose a generation strategy
     - assert uniqueness/presence in tests

2. ordering assertions
   - add coverage for:
     - happy path
     - policy deny
     - verification failure
   - assert relative event ordering, not only presence

3. event volume benchmark
   - isolate runtime event emission cost
   - document how to run and how to read the result

### Planner / context

1. structured multi-step planner example
   - likely destination:
     - new example or expanded example under `examples/`
   - should show:
     - at least two derived steps
     - not just direct `step.run` injection

2. replan example
   - should show:
     - a revision reason
     - revised plan state becoming visible through runtime/read APIs

3. planner-driven tests
   - add at least:
     - one happy path where the runtime relies on planner output
     - one failure path where planner cannot derive a valid step

4. richer context assembly example
   - add:
     - a layered assembled context example
     - tests that lock required keys/shape
     - compaction/reduction helper examples

### Documentation sync work

When the above items land, update at least the following:
- `README.md`
- `docs/API.md`
- `docs/PLANNER_CONTEXT.md`
- `docs/EVAL.md`
- `docs/STATUS.md`
- `internal/postgres/README.md`

The purpose is simple: no durable/planner/runtime milestone should be "done in code but stale in docs".

---

## Verification matrix

Use this as the default evidence set before checking off roadmap items:

### Core regression baseline

```bash
go test ./...
```

### Runtime benchmarks

```bash
go test -run '^$' -bench RunStep -benchmem ./pkg/harness/runtime
```

### WebSocket adapter focus

```bash
go test ./adapters/websocket
```

### Postgres-focused suites

Run the durable-path integration command(s) added in Wave 1.
The final command shape is an output of the roadmap work and should be recorded in:
- `README.md`
- `docs/EVAL.md`
- this roadmap when the task is checked off

---

## P2 — intentionally deferred for now

- [x] PTY shell execution
- [ ] Windows-native module
- [ ] Knowledge / retrieval module
- [ ] Multi-process / distributed runtime
- [ ] Advanced tenant/user auth
- [ ] Organization-specific approval UI / delegation workflow

---

## Milestone definition: "project-usable alpha"

`harness-core` may be considered ready for controlled project use when all of the following are true:

- [x] Postgres repositories are implemented for session/task/plan/audit
- [x] `RunStep()` uses the persistence boundary for all critical state updates
- [x] WebSocket + Postgres happy-path E2E passes
- [x] WebSocket + Postgres deny-path E2E passes
- [x] Restart-read recovery baseline passes
- [x] Shell/filesystem/http modules remain green
- [x] Core docs reflect actual runtime behavior

Practical interpretation:
- alpha is reached when the durable adapter path is proven, not merely when the internal abstractions exist
- P1 and planner/context work remain important, but they are follow-on hardening after the alpha proof chain is in place

---

## Operating rule

Unless there is a strong reason to change direction:

1. finish the remaining P0 durable-path proof chain
2. finish P1 shell/API/event hardening
3. finish advanced planner/context examples and docs
4. keep P2 deferred unless it becomes a direct unblocker

More specifically:
- do not spend time on P2 while the Postgres-backed WebSocket/restart E2Es are still missing
- prefer changes that produce both proof and reusable examples
- when a roadmap item changes runtime behavior, update the relevant docs in the same change if practical

This roadmap is the source of truth for the next implementation phase.
