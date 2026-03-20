# VERSIONING.md

## Purpose

Describe the versioning and deprecation expectations for `harness-core`.

## Current stage

`harness-core` is still pre-1.0.

That means:
- public API direction matters
- breakage should still be treated seriously
- some reshaping is still allowed while the kernel contracts settle

## Versioning intent

### `pkg/harness`

This is the most stable path.

Expectation:
- prefer additive changes
- avoid renaming/removing the default constructor path casually
- document user-visible changes in `CHANGELOG.md`

### Public subpackages

These are public but evolving:
- `pkg/harness/runtime`
- domain packages such as `task/session/plan/action/verify`
- support packages such as `tool/permission/audit/persistence/executor`

Expectation:
- changes should remain coherent and documented
- correctness fixes may still reshape details before 1.0
- when behavior changes, docs/tests/changelog should move together

### Internal packages

No compatibility promise:
- `internal/*`
- `cmd/*`
- `examples/*`

## Breaking-change policy

Before 1.0:
- breaking changes are allowed when they materially improve correctness, clarity, or kernel boundaries
- avoid unnecessary churn in `pkg/harness`
- record significant public-surface changes in `CHANGELOG.md`

After 1.0 intent:
- breaking changes should require an explicit major-version change

## Deprecation policy

When deprecating a public API:
- keep the old path available when practical for at least one documented release cycle
- point to the replacement in docs and `CHANGELOG.md`
- prefer deprecation over silent removal for `pkg/harness`

Exceptions:
- security or correctness emergencies
- internal packages, which may change without notice
