# Planner Replan Example

This example focuses on planner-driven plan revision.

It installs a tiny planner that emits `step_alpha` first and `step_beta` after the first step completes, then demonstrates creating an initial plan, executing part of it, and generating a new plan revision.

## What It Demonstrates

- install a custom planner
- generate a plan from planner output
- execute the first planner-generated step
- replan after state has advanced
- inspect plan revisions and planning records

## Run

```bash
go run ./examples/planner-replan
go test ./examples/planner-replan -count=1
```

## Expected Output

You should see output similar to:

```text
initial revision=1 steps=2
phase after first step=plan
replan revision=2 steps=1
phase after replanned step=complete
planning records=2
```

The exact revision numbers may vary if you adapt the example, but the important behavior is:

- the initial plan contains more than one step
- replanning creates a later revision
- the second run finishes the session

## When To Use This Example

Use this as a reference when:

- building planner-driven workflows
- implementing “execute, inspect, then revise plan” behavior
- wiring custom replanning logic without adding approval or transport concerns yet
