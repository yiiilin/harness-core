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

## Current scaffold status

Implemented today:
- task / session / plan object model
- shared state-machine transitions
- tool registry
- verifier registry
- default policy evaluator
- shell pipe executor
- step runner (`policy -> action -> verify -> transition -> state update`)
- in-memory audit/event sink
- WebSocket adapter
- Go example clients

## Read first

- `docs/ARCHITECTURE.md`
- `docs/PROTOCOL.md`
- `docs/RUNTIME.md`
- `docs/POLICY.md`

## Run temporary WebSocket adapter

```bash
export HARNESS_ADDR=127.0.0.1:8787
export HARNESS_SHARED_TOKEN=dev-token
go run ./cmd/harness-core
```

## Run minimal happy-path example

```bash
go run ./examples/minimal-agent
```

## Run Go WebSocket client example

```bash
cd examples/go-client
export HARNESS_URL=ws://127.0.0.1:8787/ws
export HARNESS_TOKEN=dev-token
go run .
```
