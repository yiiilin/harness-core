# Postgres WebSocket Embedding Example

This example shows a minimal embedder-facing stack that combines the public durable bootstrap with the reference transport layer.

## What It Demonstrates

- open a durable runtime through `pkg/harness/postgres.OpenServiceWithConfig(...)`
- register the companion builtins/modules stack without importing `internal/*`
- rely on the builtins composition helper to wire an interactive controller
- expose the runtime through the reference WebSocket adapter
- drive interactive PTY control over transport actions:
  - `interactive.start`
  - `interactive.write`
  - `interactive.view`
  - `interactive.close`

This is intentionally more integration-oriented than `examples/postgres-embedded` and more transport-oriented than `examples/platform-durable-embedding`.

## Run

```bash
go test ./examples/postgres-websocket-embedding -count=1
```

Or run it manually:

```bash
export HARNESS_POSTGRES_DSN='postgres://harness:harness@127.0.0.1:5432/harness_test?sslmode=disable'
export HARNESS_POSTGRES_SCHEMA='postgres_websocket_embedding'
go run ./examples/postgres-websocket-embedding
```

## Expected Output

You should see a summary like:

```text
storage: postgres
addr: 127.0.0.1:...
interactive_configured: true
session_id: ...
handle_id: ...
echo: hello from postgres websocket example
```

## When To Use This Example

Use this as a reference when:

- you want the public durable bootstrap plus a shipped adapter in one place
- your platform needs interactive control over WebSocket without inventing a parallel control plane
- you want a small end-to-end wiring example before building your own server shell around the kernel
