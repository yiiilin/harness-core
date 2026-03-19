# PLANNER_CONTEXT.md

## Goal

Explain the current planner/context story in `harness-core`.

This document is intentionally modest: the current project does **not** ship a sophisticated planner.
It ships:
- an interface for planners
- an interface for context assemblers
- a tiny default context assembler
- a tiny runnable demo planner

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
