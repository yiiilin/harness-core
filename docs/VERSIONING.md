# VERSIONING.md

## Goal

Clarify how external embedders should reason about API stability in a pre-1.0 `harness-core`.

This repository is still evolving quickly, but not all package groups are equally unstable.
The purpose of this document is to reduce unnecessary vendoring, patching, and guesswork for embedding platforms.

---

## Core Policy

`harness-core` follows a **kernel-first pre-1.0 stability model**:

- the root module carries the kernel-first stability promise
- lower-level kernel/domain packages are public, but may still evolve while correctness gaps close
- companion modules are public, but versioned independently and allowed to move faster
- `internal/*`, `examples/*`, and planning docs are not compatibility surfaces

This is not a promise of zero churn before `v1.0.0`.
It is a statement of intent about which surfaces embedders should bet on first.

---

## Repository Module Layout

The repository now uses multiple `go.mod` roots plus a committed `go.work` for local development:

- root kernel module: `github.com/yiiilin/harness-core`
- companion composition module: `github.com/yiiilin/harness-core/pkg/harness/builtins`
- companion capability-pack module: `github.com/yiiilin/harness-core/modules`
- companion adapter module: `github.com/yiiilin/harness-core/adapters`
- companion CLI module: `github.com/yiiilin/harness-core/cmd/harness-core`

Important implication:

- `pkg/harness/builtins` looks like a `pkg/harness/*` package path, but it is **not** part of the root module's kernel-stability promise
- module root, not directory prefix alone, defines the release boundary
- release tags remain the source of truth for published releases
- active development branches may temporarily use resolvable pseudo-versions between companion modules so external consumers can follow `@dev` without waiting for a fresh companion tag cut
- until a companion module has a matching published companion tag on the remote, repo-local companion-module pseudo-versions should stay on the zero-base `v0.0.0-...` form
- the root kernel module may still use its normal `v1.x.y-0...` pseudo-version flow because it already has a stable root tag lineage

For local repository verification:

- `make test-workspace` runs tests across all workspace modules
- `make check-companion-versions` verifies that repo-local companion `go.mod` files track the current compatible commit
- `make test-external-consumers` validates blank external consumers without repo-local `replace`
- `make release-check` now includes the companion-module linkage and clean-consumer release gate in addition to the stable kernel assertions

---

## Stability Tiers

### Tier 1: Most stable embedding surfaces

These are the first packages embedders should depend on.
They all live in the root kernel module:

- `pkg/harness`
- `pkg/harness/postgres`
- `pkg/harness/worker`
- `pkg/harness/replay`
- public helper packages under `pkg/harness/*` whose purpose is explicitly embedding-facing rather than product-facing

Stability intent:

- prefer additive changes over reshaping existing entrypoints
- preserve the overall constructor and orchestration model
- avoid forcing embedders into `internal/*` or reference adapters for normal integration

Examples:

- `harness.New(...)`
- `harness.NewDefault()`
- `postgres.OpenService(...)`
- `worker.New(...)` and `(*worker.Worker).RunOnce(...)`
- `replay.NewReader(...)` and projection helpers

### Tier 2: Public but still evolving

These root-module packages are supported and importable, but may change faster than the Tier 1 facade:

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

Stability intent:

- keep contracts coherent
- document meaningful changes
- avoid casual breaking churn
- allow targeted changes when needed for runtime correctness, recovery correctness, replay/debug stability, or clearer kernel boundaries

### Tier 3: Public companion modules with independent cadence

These are useful public surfaces, but they are intentionally versioned independently from the root kernel module and may change faster:

- `pkg/harness/builtins`
- `modules/*`
- `adapters/*`
- `cmd/harness-core`

Stability intent:

- keep them usable and documented
- allow capability/transport iteration without forcing kernel redesign
- do not treat them as the same compatibility contract as the Tier 1 root kernel path
- allow a future root `v1.x` while these companion modules remain on their own `v0.x` cadence

### Tier 4: No public stability promise

These are not supported as stable integration surfaces:

- `internal/*`
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
- treat `pkg/harness/builtins`, `modules/*`, `adapters/*`, and `cmd/harness-core` as companion modules, not the kernel itself
- avoid importing `internal/*`
- prefer local wrappers around public APIs instead of patching the runtime unless a true public-gap exists

External-consumption rule of thumb:

- following `@dev` is supported when companion modules reference resolvable pseudo-versions at reachable commits
- companion-module pseudo-versions should remain zero-base `v0.0.0-...` until the corresponding companion tags are actually published
- released companion versions require matching published companion tags, not only a root-module tag

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
- `docs/CHANGE_POLICY.md`
- `docs/EMBEDDING.md`
- `docs/PACKAGE_BOUNDARIES.md`
- `docs/KERNEL_SCOPE.md`
- `docs/EXTENSIBILITY.md`

If those documents disagree, `docs/API.md` should be treated as the primary embedder-facing reference and the disagreement should be fixed.
