# Minimal Agent Example

This is the smallest in-process embedding example in the repository.

It wires the kernel directly through `pkg/harness`, registers the built-in tools and verifiers, installs a trivial demo planner, and executes one shell step without any transport adapter.

## What It Demonstrates

- construct the kernel in-process
- register built-in modules through `harness.RegisterBuiltins`
- attach a task to a session
- assemble context explicitly
- ask the planner for the next step explicitly
- persist the planned step into a plan
- run the step and inspect verification plus metrics

This example keeps planning explicit instead of jumping straight to `CreatePlanFromPlanner` so readers can see the planner and context contracts separately.

## Run

```bash
go run ./examples/minimal-agent
```

## Expected Output

You should see a short summary similar to:

```text
planned step title: ...
planned tool: shell.exec
action stdout: ...
session phase: complete
verify success: true
attempts: 1
metrics: ...
```

## When To Use This Example

Use this as the first reference if you want:

- a bare in-process embedding
- no WebSocket, HTTP, or external platform layer
- a small starting point for your own custom planner and policy wiring
