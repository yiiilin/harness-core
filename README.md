# harness-core

`harness-core` is an experimental Go runtime for multi-user, multi-session AI agent execution.

This v0 skeleton focuses on:
- WebSocket-first transport
- shared-token auth for v1
- session-oriented runtime model
- dynamic tool registry contracts
- pluggable executor interfaces
- structured action/result/verify message types

## Current status

This is the first scaffold.

Implemented today:
- Go module and repo skeleton
- minimal WebSocket server
- auth handshake with shared token
- in-memory session store for development
- protocol message types
- tool/verify contract types
- shell executor placeholder
- Go sample client

Not implemented yet:
- Postgres persistence
- Redis integration
- real tool execution pipeline
- audit event persistence
- browser/windows/knowledge executors
- approvals workflow

## Run

```bash
go run ./cmd/harness-core
```

Environment variables:

```bash
export HARNESS_ADDR=127.0.0.1:8787
export HARNESS_SHARED_TOKEN=dev-token
```

## WebSocket

Connect to:

```text
ws://127.0.0.1:8787/ws
```

Send auth first:

```json
{
  "id": "1",
  "type": "auth",
  "token": "dev-token"
}
```

Then supported actions:
- `session.ping`
- `session.create`
- `session.get`

## Example client

```bash
cd examples/go-client
export HARNESS_URL=ws://127.0.0.1:8787/ws
export HARNESS_TOKEN=dev-token
go run .
```
