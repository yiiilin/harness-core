# VERSIONING.md

## Goal

Clarify how external embedders should reason about API stability in a pre-1.0 `harness-core`.

This repository is still evolving quickly, but not all package groups are equally unstable.
The purpose of this document is to reduce unnecessary vendoring, patching, and guesswork for embedding platforms.

---

## Core Policy

`harness-core` follows a **kernel-first pre-1.0 stability model**:

- the top-level embedding path should stay relatively stable
- lower-level kernel/domain packages are public, but may still evolve while correctness gaps close
- modules and adapters are intentionally faster-moving
- `internal/*`, `cmd/*`, and `examples/*` are not compatibility surfaces

This is not a promise of zero churn before `v1.0.0`.
It is a statement of intent about which surfaces embedders should bet on first.

---

## Stability Tiers

### Tier 1: Most stable embedding surfaces

These are the first packages embedders should depend on:

- `pkg/harness`
- `pkg/harness/postgres`
- public helper packages under `pkg/harness/*` whose purpose is explicitly embedding-facing rather than product-facing

Stability intent:

- prefer additive changes over reshaping existing entrypoints
- preserve the overall constructor and orchestration model
- avoid forcing embedders into `internal/*` or reference adapters for normal integration

Examples:

- `harness.New(...)`
- `harness.NewDefault()`
- `builtins.Register(&opts)`
- `postgres.OpenService(...)`
- `worker.New(...)` and `(*worker.Worker).RunOnce(...)`
- `replay.NewReader(...)` and projection helpers

### Tier 2: Public but still evolving

These packages are supported and importable, but may change faster than the facade:

- `pkg/harness/runtime`
- `pkg/harness/task`
- `pkg/harness/session`
- `pkg/harness/plan`
- `pkg/harness/action`
- `pkg/harness/verify`
- `pkg/harness/tool`
- `pkg/harness/permission`
- `pkg/harness/audit`
- `pkg/harness/persistence`
- `pkg/harness/observability`
- `pkg/harness/executor/*`
- `pkg/harness/builtins`

Stability intent:

- keep contracts coherent
- document meaningful changes
- avoid casual breaking churn
- allow targeted changes when needed for runtime correctness, recovery correctness, replay/debug stability, or clearer kernel boundaries

### Tier 3: Reference / fast-moving public packages

These are useful reference surfaces, but embedders should expect faster change:

- `modules/*`
- `adapters/*`

Stability intent:

- keep them usable and documented
- allow capability/transport iteration without forcing kernel redesign
- do not treat them as the same compatibility contract as `pkg/harness`

### Tier 4: No public stability promise

These are not supported as stable integration surfaces:

- `internal/*`
- `cmd/*`
- `examples/*`
- `docs/plans/*`

They may change, move, or disappear when necessary.

---

## What Can Justify Breaking Change Before 1.0

Breaking change is still sometimes justified before `v1.0.0`, but it should be deliberate.

Valid reasons include:

- fixing incorrect runtime semantics
- fixing recovery or approval-state-machine correctness
- fixing replay/debug stability issues
- removing accidental leaks of transport/auth/product concepts into kernel types
- replacing an ambiguous API with a clearer transport-neutral one

Weak reasons include:

- convenience for one embedding app
- reducing a local wrapper in one product
- moving product-layer concerns into the kernel
- renaming public types without a strong correctness or clarity reason

---

## Embedder Guidance

If you are building on `harness-core`:

- start from `pkg/harness`
- use `pkg/harness/postgres` for durable Postgres bootstrap
- treat `modules/*` and `adapters/*` as reference/extensibility layers, not the kernel itself
- avoid importing `internal/*`
- prefer local wrappers around public APIs instead of patching the runtime unless a true public-gap exists

If you find yourself patching:

- `modules/*` for replaceable capability execution
- reusable worker orchestration
- replay/debug projections
- public bootstrap wiring

that is a strong signal the project should consider a better public helper or extension point.

---

## Relationship To Other Docs

Read these together:

- `docs/API.md`
- `docs/API.zh-CN.md`
- `docs/EMBEDDING.md`
- `docs/PACKAGE_BOUNDARIES.md`
- `docs/KERNEL_SCOPE.md`
- `docs/EXTENSIBILITY.md`

If those documents disagree, `docs/API.md` should be treated as the primary embedder-facing reference and the disagreement should be fixed.
