# 2026-03-21 v1 Multi-Module Split Plan

## Goal

Adopt strategy 2 for the `v1` compatibility boundary:

- keep the kernel on the root module
- split faster-moving surfaces into separate Go modules
- preserve existing import paths where possible
- avoid turning `modules/*` and `adapters/*` into accidental `v1` commitments

This plan exists to resolve the single-`go.mod` semver ambiguity called out in
`docs/V1_RELEASE_CHECKLIST.md`.

## Decision Summary

The selected direction is:

1. keep the root module `github.com/yiiilin/harness-core` focused on the kernel
2. move faster-moving surfaces into nested modules at their existing path roots
3. treat `pkg/harness/builtins` as a companion composition module, not part of
   the stable kernel surface

The key design choice is to use **nested modules at existing directory roots**
so that most consumer import paths do not need to change.

## Why This Shape

This is the best fit for the current repository because:

- `pkg/harness/*` is already the intended stable embedding surface
- `modules/*` is useful and public, but still intentionally faster-moving
- `adapters/*` is useful and public, but still reference-grade
- `pkg/harness/builtins` currently pulls module-layer code into the root module
- many consumers already import these paths directly

If the split keeps the same import paths, consumers do not need a large code
rewrite.

What changes is the release contract, not the package names.

## Target Module Layout

### Root stable module

Keep the root module:

- module path: `github.com/yiiilin/harness-core`

Owns:

- `pkg/harness`
- `pkg/harness/postgres`
- `pkg/harness/worker`
- `pkg/harness/replay`
- the other kernel/domain packages under `pkg/harness/*`
- `internal/*`
- docs, tests, and release gate for the kernel path

Compatibility intent:

- this becomes the main `v1`-stable module

### Builtins companion module

Create a nested module at:

- directory: `pkg/harness/builtins`
- module path: `github.com/yiiilin/harness-core/pkg/harness/builtins`

Owns:

- the convenience module-pack composition helper

Compatibility intent:

- companion module
- not part of the root kernel `v1` promise by default
- can remain `v0` while the kernel becomes `v1`

### Modules companion module

Create a nested module at:

- directory: `modules`
- module path: `github.com/yiiilin/harness-core/modules`

Owns:

- `modules/shell`
- `modules/http`
- `modules/filesystem`
- future capability packs under `modules/*`

Compatibility intent:

- fast-moving extension layer
- version independently from the kernel

### Adapters companion module

Create a nested module at:

- directory: `adapters`
- module path: `github.com/yiiilin/harness-core/adapters`

Owns:

- `adapters/http`
- `adapters/websocket`
- future adapters under `adapters/*`

Compatibility intent:

- fast-moving transport/reference layer
- version independently from the kernel

## Important Consequence: Import Paths Mostly Stay The Same

This plan does **not** require changing these import paths:

- `github.com/yiiilin/harness-core/modules/shell`
- `github.com/yiiilin/harness-core/modules/http`
- `github.com/yiiilin/harness-core/modules/filesystem`
- `github.com/yiiilin/harness-core/adapters/http`
- `github.com/yiiilin/harness-core/adapters/websocket`
- `github.com/yiiilin/harness-core/pkg/harness/builtins`

That is the main reason to prefer nested module roots over moving packages to
new top-level directories.

## Main Complication: Builtins Wrappers On The Root Facade

The root facade previously exposed:

- `harness.NewWithBuiltins()`
- `harness.RegisterBuiltins(&opts)`

Those wrappers import `pkg/harness/builtins`, which in turn imports module-layer
packages.

If left unchanged, the root stable module still exposes a convenience path tied
to faster-moving companion modules.

Implemented action:

1. remove `harness.NewWithBuiltins`
2. remove `harness.RegisterBuiltins`
3. update docs/examples/tests to import `pkg/harness/builtins` directly when
   built-in capability packs are desired
4. keep the root stable path focused on bare-kernel construction

This is the main API cleanup required by strategy 2.

## Migration Phases

### Phase 1: Boundary Cleanup

Before creating submodules:

- remove root facade builtins wrappers
- update `docs/API*.md`, `docs/VERSIONING.md`, and `docs/PACKAGE_BOUNDARIES.md`
- update examples to prefer direct `pkg/harness/builtins` imports instead of
  `harness.RegisterBuiltins`
- adjust release gate tests so Tier 1 kernel guarantees do not depend on root
  builtins wrappers

Exit criteria:

- root `pkg/harness` no longer needs to promise convenience wrappers tied to
  fast-moving companion modules

### Phase 2: Create Nested Modules

Add `go.mod` files at:

- `pkg/harness/builtins/go.mod`
- `modules/go.mod`
- `adapters/go.mod`
- `cmd/harness-core/go.mod`

Dependency direction should become:

- root kernel module: no dependency on modules or adapters
- builtins module: depends on root kernel module and `modules`
- adapters module: depends on root kernel module and optionally builtins/module
  packages

The implementation also split `cmd/harness-core` so the reference server / CLI
can follow the same independent release cadence as the other companion layers.

### Phase 3: Add Workspace-Aware Local Development

Because the repo becomes multi-module, local development should add:

- a committed `go.work` file

Recommended `go.work use` set:

- `./`
- `./pkg/harness/builtins`
- `./modules`
- `./adapters`
- `./cmd/harness-core`

This keeps in-repo development simple while preserving independent module
versioning.

### Phase 4: Update Build And Test Commands

After the split, plain `go test ./...` at the root is no longer enough to cover
every module.

Recommended approach:

- keep root `go test ./...` for kernel verification
- add explicit multi-module `make` targets that iterate all workspace modules
- keep `make release-check` focused on the root stable kernel promise

Recommended targets:

- `make test-kernel`
- `make test-builtins`
- `make test-modules`
- `make test-adapters`
- `make test-workspace`
- `make release-check`

### Phase 5: Tagging And Release Process

After the split, version tags should reflect module boundaries.

Expected tag shape:

- root kernel module:
  - `v1.0.0`
  - `v1.0.1`
- builtins companion module:
  - `pkg/harness/builtins/v0.x.y`
- modules companion module:
  - `modules/v0.x.y`
- adapters companion module:
  - `adapters/v0.x.y`
- CLI companion module:
  - `cmd/harness-core/v0.x.y`

This gives the kernel a stable `v1` while keeping companion layers on their own
release cadence.

## Consumer Migration Impact

### For kernel embedders

If they already depend mainly on:

- `pkg/harness`
- `pkg/harness/postgres`
- `pkg/harness/worker`
- `pkg/harness/replay`

then the migration impact should be minimal.

### For consumers importing builtins

If they already import:

- `github.com/yiiilin/harness-core/pkg/harness/builtins`

then the import path can stay the same.

The main change is that builtins becomes a separately versioned companion module.

### For consumers using root builtins wrappers

If they rely on:

- `harness.NewWithBuiltins()`
- `harness.RegisterBuiltins(&opts)`

they should migrate to:

```go
import "github.com/yiiilin/harness-core/pkg/harness/builtins"

var opts harness.Options
builtins.Register(&opts)
rt := harness.New(opts)
```

This is the main consumer-facing code migration.

### For module and adapter consumers

Imports can remain the same.

The practical difference is version selection and compatibility expectations,
not source-level package renaming.

## Risks

### 1. Builtins cleanup touches many tests and docs

This is expected.
It is still a worthwhile cleanup because it removes the biggest source of
boundary ambiguity in the root stable module.

### 2. Multi-module CI is more complex than single-module CI

This is also expected.
The tradeoff is worthwhile because the versioning story becomes honest and
defensible.

### 3. Same-looking import paths can still be different release boundaries

`pkg/harness/builtins` still looks like a `pkg/harness/*` package path, but it
is now a separate module and should not be confused with the root kernel
compatibility promise.

## Recommended Execution Order

1. deprecate and remove reliance on root builtins wrappers
2. update docs/examples/tests to direct builtins imports
3. add nested `go.mod` files for `pkg/harness/builtins`, `modules`, and
   `adapters`
4. add `go.work`
5. update Makefile and CI to run per-module tests
6. document tag/release policy for the new module layout

## Practical Bottom Line

Strategy 2 is viable without a large consumer import-path rewrite.

The critical idea is:

- **split by nested module roots, not by moving package directories**

That gives `harness-core` a credible path to:

- root kernel `v1`
- companion module packs and adapters staying independently versioned
- clearer release promises for embedders
