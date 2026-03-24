# RELEASING.md

## Goal

Document the practical release flow for the multi-module repository layout.

This is the operator-facing companion to:

- `docs/VERSIONING.md`
- `docs/CHANGE_POLICY.md`
- `docs/V1_RELEASE_CHECKLIST.md`

## Module Release Units

The repository now has multiple release units:

- root kernel module: `github.com/yiiilin/harness-core`
- builtins companion module: `github.com/yiiilin/harness-core/pkg/harness/builtins`
- modules companion module: `github.com/yiiilin/harness-core/modules`
- adapters companion module: `github.com/yiiilin/harness-core/adapters`
- CLI companion module: `github.com/yiiilin/harness-core/cmd/harness-core`

Expected tag shapes:

- root kernel: `v1.2.3`
- builtins: `pkg/harness/builtins/v0.4.0`
- modules: `modules/v0.4.0`
- adapters: `adapters/v0.4.0`
- CLI: `cmd/harness-core/v0.4.0`

## Preflight

Before creating any release tag, run:

```bash
make release-preflight
```

That performs:

- `go work sync`
- `make test-workspace`
- `make release-check`

The goal is to validate both:

- the full workspace
- the Tier 1 kernel release gate

## Tag Resolution Helper

The repository includes a small helper:

```bash
bash ./scripts/release-module.sh resolve <module> <version>
```

Examples:

```bash
bash ./scripts/release-module.sh resolve root v1.0.0
bash ./scripts/release-module.sh resolve builtins v0.3.0
bash ./scripts/release-module.sh resolve modules v0.3.0
bash ./scripts/release-module.sh resolve adapters v0.3.0
bash ./scripts/release-module.sh resolve cli v0.3.0
```

Equivalent `make` wrapper:

```bash
make release-resolve MODULE=builtins VERSION=v0.3.0
```

## Creating A Tag

Dry-run preview:

```bash
make release-tag MODULE=modules VERSION=v0.3.0
```

Actual local tag creation:

```bash
make release-tag MODULE=modules VERSION=v0.3.0 APPLY=1
```

The helper:

- validates the module selector
- validates the version shape
- resolves the correct tag name
- refuses to create duplicate local tags

It creates a local annotated tag only.
Pushing remains explicit:

```bash
git push origin modules/v0.3.0
```

## Module Selectors

Supported selectors for the helper:

- `root`
- `builtins`
- `modules`
- `adapters`
- `cli`

## Release Discipline

Use the root kernel module for:

- kernel semantic changes
- Tier 1 API changes
- durable bootstrap changes

Use companion-module tags for:

- builtins composition changes
- capability-pack changes
- adapter transport changes
- CLI-only operational changes

Companion module requirement rule:

- active development branches may use resolvable pseudo-versions between repo-local modules so external consumers can follow `@dev`
- when a companion module already has release tags, its dev pseudo-version should advance from the latest pushed tag instead of falling back to `v0.0.0-...`
- those pseudo-versions must point at reachable commits in this repository and must never use the old `v0.0.0` workspace placeholder form
- published releases must rewrite repo-local dependencies back to real released versions before cutting and pushing tags
- every referenced companion-module release version must have a matching companion-module tag, not only a root tag
- those companion tags must be pushed, not just created locally, or downstream `go mod tidy` may fail to resolve the dependency graph cleanly
- `v0.0.0` is a workspace-only placeholder and must never ship in a published companion module
- root-module `replace` directives are acceptable for local workspace development, but downstream consumers do not inherit dependency-module `replace` lines

Do not tag companion modules as a proxy for kernel semantic changes unless the
root module also needs a release.

## Practical Summary

For maintainers, the default flow is:

1. `make release-preflight`
2. `make release-resolve MODULE=... VERSION=...`
3. `make release-tag MODULE=... VERSION=... APPLY=1`
4. `git push origin <resolved-tag>`
