# Examples

This directory contains runnable reference examples for different embedding styles around `harness-core`.

Each example is intentionally small and focused. Pick the one that matches the integration shape you want to learn first.

## Example Map

- `go-client`
  - A minimal Go client for the reference WebSocket adapter.
  - Shows request/response flow, authentication, lifecycle calls, and one step execution over `/ws`.
- `minimal-agent`
  - The smallest in-process embedding of the kernel with built-in tools and a trivial planner.
  - Best starting point if you want no transport adapter at all.
- `planner-context`
  - A focused `ContextAssembler` example.
  - Shows how to build a layered context package and expose compact previews without committing to a real compactor yet.
- `planner-replan`
  - A focused planner/replan example.
  - Shows plan revisions, planner-generated steps, and executing a new plan after the first step completes.
- `program-graph`
  - A focused preplanned execution-program example.
  - Shows output refs, artifact refs, explicit target fan-out, aggregate verification, and replay target slices through the public kernel API.
- `platform-reference`
  - A small platform-side orchestration example around the kernel.
  - Shows claims, lease renewals, claimed execution, PTY attach/detach, verifiers, typed interactive runtime projection, and runtime-handle reconciliation.
- `platform-embedding`
  - A small existing-platform embedding example built only on public packages.
  - Shows accepted-first run intake, external run ID mapping, worker-helper orchestration, external approval response, remote PTY wiring, and replay projection without local PTY verifiers.
- `platform-durable-embedding`
  - A durable platform-side embedding example built on the public Postgres bootstrap.
  - Shows external run ID mapping, approval pause, service reopen, approval response, and resuming the same durable session.
- `postgres-embedded`
  - The smallest durable embedding example through the public Postgres bootstrap.
  - Shows how to open a durable runtime and run work without adapters.
- `postgres-websocket-embedding`
  - A durable embedder-facing example that also exposes the reference WebSocket adapter.
  - Shows public Postgres bootstrap, builtins/modules wiring, interactive control, and transport actions in one runnable flow.
- `postgres-workers`
  - A durable multi-worker reference example.
  - Shows two runtime instances contending for runnable and recoverable sessions through claim/lease APIs.
- `workflow-scenarios`
  - A human-readable workflow walkthrough.
  - Shows concrete planner, approval/resume, and recovery tasks running through the public kernel surfaces with visible stdout and replay facts.

## How To Use This Directory

- Start with `minimal-agent` if you want to understand the bare kernel surface.
- Move to `planner-context` and `planner-replan` if you are implementing custom planning logic.
- Use `workflow-scenarios` if you want to watch a few concrete tasks move through the workflow before reading lower-level tests.
- Use `program-graph` if you are wiring a preplanned tool graph directly into the kernel.
- Use `platform-reference` if you are building a platform-side worker around PTY or interactive shell execution.
- Use `postgres-embedded`, `postgres-websocket-embedding`, and `postgres-workers` if you need durable runtime state, adapter wiring, or multi-instance coordination.
- Use `go-client` if you are integrating through the shipped WebSocket adapter rather than embedding the kernel directly.

Every example directory has its own `README.md` with:

- what the example demonstrates
- run commands
- expected output shape
- when to use it as a reference
