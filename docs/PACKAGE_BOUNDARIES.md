# PACKAGE_BOUNDARIES.md

## Goal

Clarify which packages are intended as public API surfaces and which are better treated as implementation details.

This is a guidance document, not a compatibility guarantee. The project is still early.

---

## Preferred import path

Most consumers should start here:

```go
import "github.com/yiiilin/harness-core/pkg/harness"
```

This facade is intended to remain the simplest public entry point.

Preferred bare-kernel constructors:
- `harness.New(opts)`
- `harness.NewDefault()`

Companion builtins composition helper:
- `pkg/harness/builtins`
- `builtins.New()`
- `builtins.Register(&opts)`
  - separate `go.mod`
  - same import path, separate release cadence from the root kernel module

Preferred durable Postgres bootstrap helper package:
- `pkg/harness/postgres`
- `postgres.OpenService(...)`
- `postgres.BuildOptions(...)`

Repository companion modules:
- `modules/*`
- `adapters/*`
- `cmd/harness-core`

The root `pkg/harness` facade intentionally stays on the bare-kernel path.
Built-in capability packs are composed through the companion `pkg/harness/builtins` module, not root convenience wrappers.

---

## Public-facing packages (preferred)

These root-module packages are the intended primary surfaces for consumers:

- `pkg/harness`
- `pkg/harness/runtime`
- `pkg/harness/task`
- `pkg/harness/session`
- `pkg/harness/plan`
- `pkg/harness/action`
- `pkg/harness/verify`
- `pkg/harness/tool`
- `pkg/harness/permission`
- `pkg/harness/audit`
- `pkg/harness/observability`
- `pkg/harness/postgres`
- `pkg/harness/worker`
- `pkg/harness/replay`

These packages define the kernel's reusable contracts and composition points.
`pkg/harness/postgres` is a public durable bootstrap helper around the kernel, not a kernel domain package.

---

## Public companion modules

These are useful and supported, but consumers should expect them to evolve faster and to follow their own module versioning:

- `pkg/harness/builtins`
- `modules/*`
- `adapters/*`
- `cmd/harness-core`

Rationale:
- `pkg/harness/builtins` is composition glue for default capability packs, not bare-kernel API
- modules are capability packs and may expand rapidly
- adapters reflect transport/runtime choices and are not the kernel itself
- the CLI is an operational reference surface, not a library contract

---

## Purity constraints for `pkg/harness/*`

The public package boundary should reinforce kernel scope, not weaken it.

Rules:
- root-module `pkg/harness/*` must not import `adapters/*`
- exported kernel types must not encode transport, auth, user, tenant, or UI concepts
- concrete bootstrap helpers such as `pkg/harness/postgres` may exist, but they should stay as composition layers rather than polluting core runtime/domain types
- module packs may register tools or verifiers, but module lifecycle and UX semantics should stay out of kernel domain objects
- convenience bundle helpers should stay mechanically separate from the bare-kernel path whenever possible
- `pkg/harness/runtime` should not directly import `modules/*`; composition should happen in a separate helper layer
- do not infer kernel ownership from the directory prefix alone; `pkg/harness/builtins` is intentionally a separate companion module

---

## Internal / implementation-oriented packages

These are not intended as the primary public integration surface:

- `internal/*`
- `examples/*`
- `docs/plans/*`

They are useful for reference and testing, but embedding applications should avoid depending on them directly.
`cmd/harness-core` is public as a companion CLI module, but it is not a preferred library integration surface.

---

## Rule of thumb

If your code is:
- embedding the kernel
- replacing planners/policies/verifiers/tools
- integrating the runtime into your own application

Prefer the `pkg/harness/*` packages.

If your code is:
- experimenting with transport bindings
- creating reusable capability packs
- copying examples to start local development

Then `adapters/*`, `modules/*`, and `examples/*` are appropriate reference points.

---

## Stability direction

The long-term intent is:
- keep `pkg/harness`, `pkg/harness/postgres`, `pkg/harness/worker`, and `pkg/harness/replay` as the most stable embedder-facing path
- keep core domain packages public and coherent while allowing pre-1.0 correctness-driven changes
- let modules and adapters evolve faster
- avoid leaking transport-specific assumptions into the core packages

See:
- `docs/API.md`
- `docs/VERSIONING.md`
