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
- default runtime components that can be replaced incrementally

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
- default context assembler
- default planner placeholder
- default event sink bridge
- WebSocket adapter
- Go example clients
- integration tests and benchmark baseline

## Read first

- `docs/ARCHITECTURE.md`
- `docs/PROTOCOL.md`
- `docs/RUNTIME.md`
- `docs/POLICY.md`
- `docs/MODULES.md`
- `docs/EXTENSIBILITY.md`
- `docs/API.zh-CN.md` (中文快速理解与接入说明)
- `README.zh-CN.md` (中文仓库说明)
- `CONTRIBUTING.md`
- `docs/PACKAGE_BOUNDARIES.md`
- `docs/EVENTS.md`
- `docs/STATUS.md`
- `docs/PERSISTENCE.md`
- `docs/ROADMAP.md`
- `docs/EVAL.md`

## Default construction style

```go
opts := harness.Options{}
harness.RegisterBuiltins(&opts)
rt := harness.New(opts)
```

Then replace pieces incrementally as needed:
- custom `PolicyEvaluator`
- custom `ContextAssembler`
- custom `Planner`
- custom `EventSink`
- custom tool and verifier registrations

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

## Test and benchmark

```bash
go test ./...
go test -bench . ./pkg/harness/runtime
```
