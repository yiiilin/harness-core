# Program Graph Example

This example demonstrates the public `ExecutionProgram` model and the current native runtime support around it.

## What It Demonstrates

- define a preplanned execution graph with typed `ExecutionProgramNode` values
- use `ExecutionProgramInputBinding` with:
  - structured `ExecutionOutputRef`
  - artifact `ExecutionOutputRef`
- fan out one logical node across multiple explicit execution targets
- evaluate aggregate verification with `ExecutionProgramNode.VerifyScope=aggregate`
- inspect replay target slices after execution

## Run

```bash
go run ./examples/program-graph
go test ./examples/program-graph -count=1
```

## Expected Output

The program prints:

- the created session ID
- the final session phase
- the aggregate fan-out summary
- the artifact reference consumed by the downstream step
- the per-target dispatch outputs
- the number of replay cycles that expose target slices

## When To Use This Example

Use this example when you want to:

- submit a preplanned tool graph directly into the kernel
- wire step-to-step structured output and artifact references
- run one logical action across multiple explicit execution targets
- understand what today’s public native fan-out / aggregate / replay surface actually supports
