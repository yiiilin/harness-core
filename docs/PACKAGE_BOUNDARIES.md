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

These packages define the kernel's reusable contracts and composition points.

---

## Semi-stable / advanced-use packages

These are useful, but consumers should expect them to evolve faster:

- `modules/*`
- `adapters/*`

Rationale:
- modules are capability packs and may expand rapidly
- adapters reflect transport/runtime choices and are not the kernel itself

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
- keep `pkg/harness` and the core domain packages relatively stable
- let modules and adapters evolve faster
- avoid leaking transport-specific assumptions into the core packages
