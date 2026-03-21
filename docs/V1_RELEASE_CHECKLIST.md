# V1_RELEASE_CHECKLIST.md

## Purpose

Define what must be true before `harness-core` should be tagged `v1.0.0`.

This document is intentionally strict.
It is not a feature wishlist.
It is a release gate for a small, execution-focused kernel.

Use this together with:

- `docs/KERNEL_SCOPE.md`
- `docs/API.md`
- `docs/VERSIONING.md`
- `docs/CURRENT_STATE.md`
- `docs/STATUS.md`

## What `v1.0.0` Means Here

For `harness-core`, `v1.0.0` should mean:

- the kernel scope is stable and explicit
- the embedder-facing public surface is clear
- the project can make a real compatibility promise on the intended stable path
- durable upgrade behavior is tested enough to be relied on
- core workflow semantics are protected by release-grade black-box tests

It does **not** mean:

- complete product platform
- built-in user / tenant / org concepts
- built-in approval UI
- built-in provider routing or LLM client integrations
- a large adapter/module ecosystem

Those remain outside the kernel scope.

## Release Decision Rule

Do not tag `v1.0.0` unless every `must-have` item below is complete.

`Should-have` items are important, but they should not block `v1` if the kernel
surface is otherwise ready.

`Post-v1` items are deliberately deferred expansion work.

## Must-Have

### 1. Freeze the real `v1` compatibility boundary

The current docs already identify the intended stable path:

- `pkg/harness`
- `pkg/harness/postgres`
- `pkg/harness/worker`
- `pkg/harness/replay`

Before `v1`, the project must explicitly resolve the current single-module
semver problem:

- today the repository has one `go.mod`
- in practice, external users may read a `v1` tag as a compatibility promise for
  all public import paths in that module
- current docs still describe `modules/*` and `adapters/*` as faster-moving

One concrete strategy must be chosen and documented:

1. treat `modules/*` and `adapters/*` as part of the `v1` compatibility promise
2. split faster-moving surfaces into separate Go modules
3. otherwise restructure public paths so unstable surfaces are not accidentally
   implied to be `v1`-stable

Exit criteria:

- one strategy chosen
- `docs/VERSIONING.md` updated to match the chosen strategy exactly
- `docs/API.md` and `docs/PACKAGE_BOUNDARIES.md` agree with it
- no contradictory statements remain in docs

### 2. Add compatibility tests for the Tier 1 public surface

`v1` should not rely only on prose promises.

The intended stable path needs explicit release guards for:

- constructor and bootstrap presence
- worker helper presence and core behavior
- replay helper presence and core behavior
- key facade re-export types and runtime control methods

Recommended minimum:

- compile-time API shape tests for the Tier 1 packages
- black-box behavior tests that use only the Tier 1 path
- a release CI target that runs those tests independently

Exit criteria:

- Tier 1 compatibility tests exist
- CI has a release-oriented target for them
- release notes can point to those packages as the supported stable path

### 3. Lock the durable Postgres upgrade contract

Durable bootstrap is already public and strong enough to use.
Before `v1`, it should also be strong enough to promise.

That requires explicit upgrade coverage for:

- migration application from an older schema to the current schema
- service restart after migration
- recovery and approval state surviving upgrade/restart paths
- migration status / pending / drift helpers behaving predictably

Exit criteria:

- at least one upgrade-path integration test exists
- migration and upgrade expectations are documented in public docs
- `pkg/harness/postgres` is clearly stated as the canonical durable path
- schema-breaking change policy is documented for post-`v1` releases

### 4. Define a release-grade workflow eval matrix

The project already has strong correctness tests.
Before `v1`, the release gate should also include a small required black-box
matrix covering the kernel's core promises:

- planner-driven execution
- approval pause -> respond -> resume
- claim / lease / recoverable execution
- durable restart / recovery
- replay/debug fact visibility

The purpose is not more coverage for its own sake.
The purpose is to protect the high-level behaviors embedders are buying.

Exit criteria:

- a documented release eval matrix exists
- the matrix is runnable in CI
- failures in those scenarios are treated as release blockers

### 5. Publish a narrow post-`v1` change policy

Before tagging `v1`, maintainers should write down what kinds of future changes
are still allowed on the stable path and what requires a new major version.

At minimum, document:

- deprecation expectations
- additive vs breaking API changes
- migration expectations for durable schema evolution
- how adapter/module churn relates to the stable embedding surface

Exit criteria:

- public policy exists in docs
- release maintainers can answer “is this breaking?” without guesswork

## Should-Have

These are valuable, but they should not delay `v1` if the must-have gates are
satisfied.

### 1. Stronger `pkg/harness/worker` ergonomics

Examples:

- jitter/backoff helpers
- better shutdown control
- thin observability wrappers

Constraint:

- keep it transport-neutral
- do not turn it into fleet orchestration

### 2. More validation for remote PTY inspection

The current `PTYBackend` and `PTYInspector` direction is correct.
What remains is more proving, not more kernel scope.

Examples:

- more remote-style integration tests
- more example embedders using non-local PTY execution

### 3. Stronger adapter-facing guidance

This is mainly documentation and helper cleanup around:

- protocol versioning expectations
- event mapping guidance
- kernel error to transport error guidance

### 4. More golden examples for embedders

Examples:

- accepted-first platform wrapping
- remote PTY integration
- durable worker deployment patterns

These help adoption, but they are not `v1` blockers.

## Post-V1

These are worth doing after `v1`, but should not expand the release gate now.

### 1. More capability modules

Examples:

- browser
- windows
- retrieval / knowledge
- richer shell integrations

### 2. More reference adapters

Examples:

- SSE
- gRPC
- richer HTTP control/data planes

### 3. More platform-side reference implementations

Examples:

- accepted-first API shell
- queue-backed worker fleet reference
- projection/read-model reference

### 4. Optional LLM companion packages outside the kernel

If the project later wants to help embedders with planning integrations, the
right place is likely a companion package, not the core runtime.

Examples:

- planner helper packages
- prompt/context packaging helpers
- provider-specific examples outside the kernel surface

## Current Recommendation

If the goal is a disciplined `v1`, the next sequence should be:

1. resolve the single-module compatibility story
2. add Tier 1 compatibility tests
3. lock the durable upgrade contract
4. formalize the release eval matrix
5. publish the post-`v1` change policy

After those are complete, the project should be in a credible position to tag a
small, focused `v1.0.0` as a harness-engineering kernel.
