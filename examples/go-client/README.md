# Go Client Example

This example is a minimal Go client for the reference WebSocket adapter in `adapters/websocket`.

It is intentionally transport-focused. It does not embed the kernel directly. Instead, it shows what an external process would send over `/ws`.

## What It Demonstrates

- authenticate over the reference WebSocket protocol
- query runtime metadata with `runtime.info`
- create a session and task, then attach them together
- create a one-step plan over the transport surface
- run a step through `step.run`
- inspect execution facts with `attempt.list`
- inspect runtime metrics with `runtime.metrics`

## Prerequisites

Start the reference server in another terminal:

```bash
go run ./cmd/harness-core
```

By default the server listens on `127.0.0.1:8787`, exposes `/ws`, and accepts the shared token `dev-token`.

## Run

```bash
go run ./examples/go-client
```

If you need non-default settings:

```bash
HARNESS_SHARED_TOKEN=my-token go run ./cmd/harness-core
HARNESS_URL=ws://127.0.0.1:8787/ws HARNESS_TOKEN=my-token go run ./examples/go-client
```

## Expected Output

The program prints one line per request, for example:

```text
auth => ...
runtime.info => ...
session.create => ...
task.create => ...
plan.create => ...
step.run => ...
attempt.list => ...
runtime.metrics => ...
```

The exact payloads vary, but you should see:

- a successful auth response
- a created `session_id`
- a created `task_id`
- a `step.run` result whose session phase is terminal or advanced as expected
- at least one persisted attempt in `attempt.list`

## When To Use This Example

Use this as a reference when:

- building a Go SDK or smoke-test client for the WebSocket adapter
- validating adapter behavior without writing a UI
- confirming a deployed server is reachable and can execute a minimal request chain
