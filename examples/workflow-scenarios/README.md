# Workflow Scenarios Example

This example runs a few concrete tasks through the public harness workflow and prints the result of each scenario.

Unlike the lower-level `evals/` package, this example is meant to be read by humans first:

- it uses the real built-in shell module instead of a fake tool
- it drives approval, resume, and recovery through the public runtime surfaces
- it prints the resulting session phase, stdout, and persisted execution-fact counts

## What It Demonstrates

- planner-derived pipe execution with `CreatePlanFromPlanner(...)` and `RunSession(...)`
- approval pause, external approval response, and resumed execution via `pkg/harness/worker`
- interrupted claimed work recovering through the same worker helper
- replay/debug facts staying queryable through `ListAttempts`, `ListActions`, `ListVerifications`, and `pkg/harness/replay`

## Run

```bash
go test ./examples/workflow-scenarios -count=1
go run ./examples/workflow-scenarios
```

## Expected Output Shape

You should see three sections similar to:

```text
scenario: planner-pipe
  tool: shell.exec
  phase: complete
  stdout: planner walkthrough

scenario: approval-resume
  first_run_approval_pending: true
  pending_approval: ...
  phase: complete
  stdout: approval walkthrough

scenario: recover-interrupted
  recovered: true
  lease_released: true
  phase: complete
  stdout: recovery walkthrough
```

## Why This Example Exists

This is the quickest way to answer “what does the workflow actually do when I feed it a few real tasks?”

Use it when you want:

- visible, concrete output instead of only assertions
- a small reference for worker-helper approval and recovery behavior
- a sanity check before wiring your own planner, approval UI, or external transport
