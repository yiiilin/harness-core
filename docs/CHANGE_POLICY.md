# CHANGE_POLICY.md

## Purpose

Define the narrow post-`v1` change policy for `harness-core`.

This document exists so maintainers can answer a practical question quickly:

> Is this change compatible with the stable embedding surface, or does it require
> a new major version?

Read this together with:

- `docs/API.md`
- `docs/VERSIONING.md`
- `docs/KERNEL_SCOPE.md`
- `docs/V1_RELEASE_CHECKLIST.md`

## Scope

This policy applies first to the intended stable embedding path:

- `pkg/harness`
- `pkg/harness/postgres`
- `pkg/harness/worker`
- `pkg/harness/replay`

If the repository later expands the set of `v1`-stable packages, this document
must be updated explicitly.

This policy does **not** automatically grant the same stability promise to:

- `pkg/harness/builtins`
- `modules/*`
- `adapters/*`
- `internal/*`
- `examples/*`
- `cmd/harness-core`

Those surfaces follow the compatibility classification documented in
`docs/VERSIONING.md`.

## Versioning Rule Of Thumb

After `v1.0.0`:

- patch releases should fix bugs without changing intended public behavior
- minor releases may add capability in a backward-compatible way
- major releases are required for breaking changes to the stable path

If there is serious doubt whether a change is breaking, treat it as breaking
until the opposite is clearly justified.

## Allowed In Patch Releases

These are generally safe for patch releases:

- correctness fixes that preserve the documented public contract
- internal refactors with no public API or durable-behavior change
- doc fixes and example fixes
- stricter validation of clearly invalid input
- observability additions that do not alter stable kernel semantics

Patch releases should avoid:

- new required configuration
- changed success/failure semantics on valid existing inputs
- changed durable upgrade expectations

## Allowed In Minor Releases

These are generally safe for minor releases:

- additive public methods or helper functions
- additive fields in output structs, when zero-value compatible
- additive event payload fields, if old readers keep working
- additive migration inspection helpers
- new companion modules, examples, and optional helper packages outside the
  stable kernel path

Minor releases must keep:

- existing constructors working
- existing runtime control methods working
- existing durable upgrade path working
- existing replay/debug reads working

## Requires A Major Version

The following changes should be treated as major-version changes on the stable
path:

- removing or renaming exported public functions, methods, or types
- changing method signatures on the stable path
- changing the meaning of successful existing calls in a way embedders must
  adapt to
- changing persisted upgrade requirements so an old deployment can no longer
  follow the documented migration path safely
- changing durable object semantics in a way that breaks replay, recovery, or
  approval/resume expectations
- changing the public compatibility scope itself without an explicit versioning
  decision

## Deprecation Policy

When a stable public API should be replaced:

1. mark it deprecated in code comments
2. document the replacement path
3. include the deprecation in release notes
4. keep the deprecated API for at least one minor release before removal

Exceptions should be rare and explicit.
They may be justified only for:

- security issues
- data corruption risk
- severe correctness bugs where keeping the old behavior is unsafe

In those cases, release notes should call out the forced change clearly.

## Durable Schema Policy

For `pkg/harness/postgres`, compatibility is not just API-level.
It also includes the documented migration path.

Post-`v1` expectations:

- migrations are forward-only
- `OpenService(...)` remains the canonical durable bootstrap path
- upgrading from the immediately previous shipped schema version must remain
  supported and tested
- migration helpers such as status / pending / drift checks must continue to
  describe the canonical migration set accurately

If a release would require a destructive or manual operator step outside the
documented migration path, that should be treated as a major-version event
unless explicitly designed and announced otherwise.

## Replay And Workflow Stability

The stable path also includes behavior, not only signatures.

Minor and patch releases should preserve the kernel's documented high-level
workflow expectations:

- approval pause -> respond -> resume
- claim / lease / recoverable execution
- durable restart and resumed execution
- replay/debug visibility through public read surfaces

If a change would require embedders to rewrite their control-plane assumptions
around these flows, it should be treated as major unless the old behavior was
plainly incorrect and unsafe.

## Review Checklist

Before merging a post-`v1` change that touches the stable path, ask:

1. Does this change remove or reshape a stable entrypoint?
2. Does it alter durable upgrade behavior?
3. Does it alter approval, recovery, replay, or lease semantics that embedders
   may already rely on?
4. Can the change be shipped additively instead?
5. Do release notes need to call out migration or deprecation guidance?

If any answer indicates user adaptation is required, treat the change as at
least a minor release candidate and likely a major-version candidate.
