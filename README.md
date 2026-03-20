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
- composable default policy path for built-in modules
- durable approval / resume kernel with `once` / `always` / `reject`
- execution facts for attempts / actions / verifications / artifacts
- shell pipe executor
- Postgres-backed repository implementations for session/task/plan/audit
- Postgres-backed approval / execution / capability snapshot / context summary storage
- Postgres-backed transaction runner and server bootstrap wiring
- step runner (`policy -> action -> verify -> transition -> state update`)
- in-memory audit/event sink
- stable runtime-emitted `event_id` values plus task / attempt / action / trace identifiers
- default context assembler
- typed planner `ContextPackage`, compactor hook, and loop-budget defaults
- capability resolution with persisted capability snapshots
- default planner placeholder
- planner-assisted plan creation via `CreatePlanFromPlanner(...)`
- default event sink bridge
- WebSocket adapter
- WebSocket approval / resume commands
- Postgres-backed WebSocket happy-path / deny-path E2E coverage
- durable restart-read coverage
- Go example clients
- planner/context examples
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
- `internal/postgres/README.md` (durable storage internals)
- `VERSIONING.md`
- `CHANGELOG.md`
- `docs/EVAL.md`

## Default construction style

```go
import (
  "github.com/yiiilin/harness-core/pkg/harness"
  "github.com/yiiilin/harness-core/pkg/harness/builtins"
)

opts := harness.Options{}
builtins.Register(&opts)
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

## Run Postgres-backed WebSocket adapter

Start a local Postgres first, for example:

```bash
docker run --rm -d \
  --name harness-pg \
  -e POSTGRES_USER=harness \
  -e POSTGRES_PASSWORD=harness \
  -e POSTGRES_DB=harness_test \
  -p 5432:5432 \
  postgres:16-alpine
```

Then start the server in durable mode:

```bash
export HARNESS_ADDR=127.0.0.1:8787
export HARNESS_SHARED_TOKEN=dev-token
export HARNESS_STORAGE_MODE=postgres
export HARNESS_POSTGRES_DSN='postgres://harness:harness@127.0.0.1:5432/harness_test?sslmode=disable'
go run ./cmd/harness-core
```

## Run minimal happy-path example

```bash
go run ./examples/minimal-agent
```

## Run planner/context examples

```bash
go run ./examples/planner-context
go run ./examples/planner-replan
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
go test -run '^$' -bench RunStep -benchmem ./pkg/harness/runtime
```
