# PLANNER_CONTEXT.md

## Goal

Explain the current planner/context story in `harness-core`.

This document is intentionally modest: the current project does **not** ship a sophisticated planner.
It ships:
- an interface for planners
- an interface for context assemblers
- a tiny default context assembler
- a tiny runnable demo planner
- runtime helpers that can build a plan revision from planner output
- runnable examples for layered context assembly and replanning

---

## Why this matters

The runtime kernel should prove that:
- planning is pluggable
- context assembly is pluggable
- step execution does not depend on any one planning strategy

The current demo planner exists to demonstrate the shape of the loop, not to act as a production planner.

---

## Current components

### DefaultContextAssembler
Produces a minimal structured context with:
- task identity
- task goal
- session id
- phase
- current step pointer
- retry count
- constraints / metadata

### NoopPlanner
Returns `ErrNoPlannerConfigured`.
Use this when the embedding app wants full control over planning.

### DemoPlanner
A minimal example planner that can derive one shell step for simple goals like:
- `echo hello`

It produces a step with:
- `shell.exec`
- `exit_code` verifier
- simple `OnFail` strategy

### `CreatePlanFromPlanner(...)`
The runtime now provides a helper that:
- loads session + attached task
- assembles planner context
- asks the configured planner for one or more next steps
- persists a new plan revision from that planner output

This keeps the kernel narrow while removing repetitive boilerplate from tests/examples.

---

## Current usage patterns

### Minimal planner-driven path

```go
pl, assembled, err := rt.CreatePlanFromPlanner(ctx, sessionID, "planner-derived revision", 1)
_ = assembled
_ = pl
_ = err
```

### Multi-step planner example

See:
- `examples/planner-replan`

That example demonstrates:
- deriving a structured two-step plan
- executing the first step
- returning the session to `plan` while work remains
- creating a new plan revision from planner output

### Layered context example

See:
- `examples/planner-context`

That example demonstrates:
- task/session core sections
- derived summary fields
- simple compaction helpers for long metadata/constraint fields

---

## Current guarantees and non-goals

What the kernel now proves:
- planners are pluggable
- context assemblers are pluggable
- planner output can be converted into persisted plan revisions
- multi-step plans do not force the session terminal after the first successful step

What the kernel still intentionally does not do:
- ship a heavyweight production planner
- ship retrieval/memory orchestration
- infer long-horizon plans from free-form assistant text by itself

---

## Intended long-term direction

`harness-core` should keep planner/context support deliberately narrow:
- define interfaces
- define contracts
- provide tiny safe defaults
- avoid embedding a heavyweight planning system into the kernel

More advanced planners or context assemblers should likely live in:
- companion packages
- embedding applications
- future modules

---

## Rule of thumb

The kernel should prove the loop.
Applications should provide the intelligence.

That means:
- core: contracts + composition + execution loop
- app/module: sophisticated planning and retrieval logic
