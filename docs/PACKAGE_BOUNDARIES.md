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

Preferred builtins composition helper package:
- `pkg/harness/builtins`
- `builtins.New()`
- `builtins.Register(&opts)`

Preferred durable Postgres bootstrap helper package:
- `pkg/harness/postgres`
- `postgres.OpenService(...)`
- `postgres.BuildOptions(...)`

Compatibility wrappers on `pkg/harness` may remain for convenience, but the composition helper package is the clearer boundary.

These convenience helpers may wire default module packs for local embedding, but they do not expand what the kernel owns.

---

## Public-facing packages (preferred)

These packages are the intended primary surfaces for consumers:

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
- `pkg/harness/builtins`
- `pkg/harness/postgres`
- `pkg/harness/worker`
- `pkg/harness/replay`

These packages define the kernel's reusable contracts and composition points.
`pkg/harness/postgres` is a public durable bootstrap helper around the kernel, not a kernel domain package.

---

## Semi-stable / advanced-use packages

These are useful, but consumers should expect them to evolve faster:

- `modules/*`
- `adapters/*`

Rationale:
- modules are capability packs and may expand rapidly
- adapters reflect transport/runtime choices and are not the kernel itself

---

## Purity constraints for `pkg/harness/*`

The public package boundary should reinforce kernel scope, not weaken it.

Rules:
- `pkg/harness/*` must not import `adapters/*`
- exported kernel types must not encode transport, auth, user, tenant, or UI concepts
- concrete bootstrap helpers such as `pkg/harness/postgres` may exist, but they should stay as composition layers rather than polluting core runtime/domain types
- module packs may register tools or verifiers, but module lifecycle and UX semantics should stay out of kernel domain objects
- convenience bundle helpers should stay mechanically separate from the bare-kernel path whenever possible
- `pkg/harness/runtime` should not directly import `modules/*`; composition should happen in a separate helper layer

---

## Internal / implementation-oriented packages

These are not intended as the primary public integration surface:

- `internal/*`
- `cmd/*`
- `examples/*`

They are useful for reference and testing, but embedding applications should avoid depending on them directly.

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
