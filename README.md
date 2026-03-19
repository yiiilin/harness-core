# harness-core

`harness-core` is a reusable harness runtime kernel for AI agent systems.

It is designed for builders who want a **small, composable, high-leverage core** for agent execution instead of a full end-user product.

## What it aims to provide

- a shared runtime state machine
- structured `action / result / verify` contracts
- a dynamic tool registry
- a verifier registry
- explicit permission / approval hooks
- structured event / audit hooks
- adapter-friendly runtime interfaces

## What it is not

- not a full SaaS agent platform
- not a giant built-in tool catalog
- not a UI product
- not a provider-specific framework

## Current repository status

This repository currently contains the first scaffold and will be reshaped toward a library-first layout.

Near-term goals:
1. stabilize contracts
2. stabilize runtime state machine
3. add minimal shell executor example
4. add WebSocket adapter example
5. add event/audit hooks

## Current scaffold

Implemented so far:
- Go module and repository skeleton
- minimal WebSocket server scaffold
- shared-token auth handshake
- in-memory session store for development
- protocol type placeholders
- tool/verify contract placeholders
- shell executor placeholder
- Go sample client

This scaffold is a temporary stepping stone toward the library-first architecture described in `docs/ARCHITECTURE.md`.

## Read first

- `docs/ARCHITECTURE.md`
- `docs/PROTOCOL.md`
- `docs/RUNTIME.md`
- `docs/POLICY.md`

## Temporary local run

```bash
go run ./cmd/harness-core
```

Environment variables:

```bash
export HARNESS_ADDR=127.0.0.1:8787
export HARNESS_SHARED_TOKEN=dev-token
```

## Temporary WebSocket test

Connect to:

```text
ws://127.0.0.1:8787/ws
```

Authenticate first:

```json
{
  "id": "1",
  "type": "auth",
  "token": "dev-token"
}
```

Currently supported placeholder actions:
- `session.ping`
- `session.create`
- `session.get`
- `tool.list`

## Example client

```bash
cd examples/go-client
export HARNESS_URL=ws://127.0.0.1:8787/ws
export HARNESS_TOKEN=dev-token
go run .
```

## Philosophy

`harness-core` should be to agent systems what a small runtime kernel is to a larger platform:
- opinionated where contracts matter
- minimal where product concerns begin
- extensible through adapters and executors
