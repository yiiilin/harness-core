# API.md

## Purpose

This document describes the intended public API surface of `harness-core`.

The main entry point should be:

```go
import "github.com/yiiilin/harness-core/pkg/harness"
```

Scope rule:
- `pkg/harness` exposes the execution kernel, not transport/auth/tenant/product APIs

See `docs/KERNEL_SCOPE.md`.

## Recommended public surface

### Top-level constructor path
- `harness.New(opts)`
- `harness.NewDefault()`
- `harness.NewWithBuiltins()`
- `harness.RegisterBuiltins(&opts)`
- `(*harness.Service).CreatePlanFromPlanner(...)`

### Re-exported core types
- task/session/plan/action/verify domain types
- tool definition and risk types
- permission decision/action types
- audit event type
- runtime interfaces: planner/context assembler/event sink

### Lower-level packages
Consumers may import lower-level packages directly when they need finer control, but the default path should begin with `pkg/harness`.

## Package-group stability notes

### Most stable path
- `pkg/harness`

Intent:
- keep the top-level facade small
- prefer additive changes over reshaping constructor ergonomics
- use this as the default embedding entry point
- avoid turning the facade into a product platform SDK

### Public but still evolving
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
- `pkg/harness/executor/shell`

Intent:
- these packages are importable and supported
- contracts should remain coherent and documented
- pre-1.0 evolution is still expected, especially when closing correctness gaps

### Internal-only / no stability promise
- `internal/*`
- `cmd/*`
- `examples/*`

Intent:
- these are allowed to move, split, or disappear
- they exist to support shipped wiring, tests, examples, and documentation

## Planner / Context usage

The planner/context API is intentionally narrow:
- planner decides the next step
- context assembler produces the structured input for that decision
- runtime execution remains explicit

Typical construction path:

```go
opts := harness.Options{}
harness.RegisterBuiltins(&opts)
rt := harness.New(opts).
	WithPlanner(myPlanner).
	WithContextAssembler(myAssembler)
```

For a planner-driven plan creation flow:

```go
sess := rt.CreateSession("demo", "run planned work")
tsk := rt.CreateTask(harness.TaskSpec{TaskType: "demo", Goal: "echo alpha then beta"})
sess, _ = rt.AttachTaskToSession(sess.SessionID, tsk.TaskID)

pl, assembled, err := rt.CreatePlanFromPlanner(ctx, sess.SessionID, "planner-derived revision", 2)
_ = assembled
_ = pl
_ = err
```

Reference examples:
- `examples/planner-context`
- `examples/planner-replan`

## Stability intent

The project is still early, but this is the intended direction:
- keep the top-level facade small and stable
- let subpackages evolve more freely
- avoid forcing consumers to understand every internal package before getting started

That stability intent applies only to kernel concepts.
Identity, transport, and platform concerns should stay out of the public kernel surface entirely.

Versioning and deprecation expectations are documented in `VERSIONING.md`.
