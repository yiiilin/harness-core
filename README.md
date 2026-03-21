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

## Repository module layout

- root kernel module: `github.com/yiiilin/harness-core`
- companion composition module: `github.com/yiiilin/harness-core/pkg/harness/builtins`
- companion capability-pack module: `github.com/yiiilin/harness-core/modules`
- companion adapter module: `github.com/yiiilin/harness-core/adapters`
- companion CLI module: `github.com/yiiilin/harness-core/cmd/harness-core`
- local development uses the committed `go.work`

The root `pkg/harness` facade stays intentionally bare-kernel.
Built-in capability packs are wired through `pkg/harness/builtins`, not root convenience wrappers.

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
- public durable Postgres bootstrap helpers under `pkg/harness/postgres`
- public Postgres migration inspection helpers under `pkg/harness/postgres`
- public worker-loop helper under `pkg/harness/worker`
- public replay/debug projection helper under `pkg/harness/replay`
- step runner (`policy -> action -> verify -> transition -> state update`)
- in-memory audit/event sink
- stable runtime-emitted `event_id` values plus task / attempt / action / trace identifiers
- default context assembler
- typed planner `ContextPackage`, compactor hook, and loop-budget defaults
- capability resolution with persisted capability snapshots
- default planner placeholder
- planner-assisted plan creation via `CreatePlanFromPlanner(...)`
- default event sink bridge
- PTY-backed shell execution via `modules/shell`
- WebSocket adapter
- WebSocket approval / resume commands
- Postgres-backed WebSocket happy-path / deny-path E2E coverage
- durable restart-read coverage
- Go example clients
- planner/context examples
- a minimal platform reference example under `examples/platform-reference`
- a durable multi-worker Postgres example under `examples/postgres-workers`
- a minimal HTTP/JSON reference adapter under `adapters/http`
- HTTP worker control-plane routes for claim / lease / claimed run / recovery / approval resume
- integration tests and benchmark baseline

## Read first

- `docs/ARCHITECTURE.md`
- `docs/PROTOCOL.md`
- `docs/RUNTIME.md`
- `docs/POLICY.md`
- `docs/MODULES.md`
- `docs/EXTENSIBILITY.md`
- `docs/ADAPTERS.md`
- `docs/API.zh-CN.md` (中文快速理解与接入说明)
- `README.zh-CN.md` (中文仓库说明)
- `CONTRIBUTING.md`
- `docs/PACKAGE_BOUNDARIES.md`
- `docs/EVENTS.md`
- `docs/STATUS.md`
- `docs/PERSISTENCE.md`
- `docs/RELEASING.md`
- `docs/ROADMAP.md`
- `internal/postgres/README.md` (durable storage internals)
- `VERSIONING.md`
- `CHANGELOG.md`
- `docs/EVAL.md`

## Default construction style

Use the bare kernel plus the companion builtins module explicitly:

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

The most stable embedding path remains:
- `pkg/harness`
- `pkg/harness/postgres`
- `pkg/harness/worker`
- `pkg/harness/replay`

Treat `pkg/harness/builtins`, `modules/*`, `adapters/*`, and `cmd/harness-core` as public companion modules with their own release cadence.
Use `docs/ADAPTERS.md` for transport-binding rules and `docs/RELEASING.md` for multi-module tagging flow.

## Default durable Postgres construction style

If you are embedding `harness-core` in your own platform and want durable runtime state, prefer the public bootstrap package instead of importing `internal/*` or copying the WebSocket server wiring:

```go
import (
  "context"

  "github.com/yiiilin/harness-core/pkg/harness/builtins"
  hpostgres "github.com/yiiilin/harness-core/pkg/harness/postgres"
  hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
)

var opts hruntime.Options
builtins.Register(&opts)

rt, db, err := hpostgres.OpenService(context.Background(), dsn, opts)
if err != nil {
  panic(err)
}
defer db.Close()
```

The reference WebSocket adapter is optional transport glue, not the required public durable integration surface.
The same applies to the minimal HTTP adapter under `adapters/http`.
For migration inspection and bootstrap operations, prefer the public helpers on `pkg/harness/postgres`; the CLI only wraps those same helpers for local operations use.

## Run temporary WebSocket adapter

```bash
export HARNESS_ADDR=127.0.0.1:8787
export HARNESS_SHARED_TOKEN=dev-token
go run ./cmd/harness-core
```

If you want an in-process durable example instead of the WebSocket adapter, see `examples/postgres-embedded`.

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

## Inspect or apply Postgres migrations

Point the CLI at a Postgres DSN:

```bash
export HARNESS_POSTGRES_DSN='postgres://harness:harness@127.0.0.1:5432/harness_test?sslmode=disable'
go run ./cmd/harness-core migrate status
go run ./cmd/harness-core migrate version
go run ./cmd/harness-core migrate up
```

The CLI is an operations convenience wrapper.
Embedding platforms should use `pkg/harness/postgres` directly for bootstrap, status, pending-migration, and drift checks.

## Verification commands

```bash
make test-workspace
make release-check
make release-preflight
make build
```

- `make test-workspace` runs tests across the root kernel module plus all companion modules in `go.work`
- `make release-check` runs the release gate focused on the stable kernel promise
- `make release-preflight` runs workspace tests plus the release gate before tagging
- `make build` writes the reference server binary to `bin/harness-core`

## Run minimal happy-path example

```bash
go run ./examples/minimal-agent
```

## Run planner/context examples

```bash
go run ./examples/planner-context
go run ./examples/planner-replan
```

## Run minimal platform reference example

```bash
go run ./examples/platform-reference
```

## Run Postgres multi-worker reference example

```bash
export HARNESS_POSTGRES_DSN='postgres://harness:harness@127.0.0.1:5432/harness_test?sslmode=disable'
go run ./examples/postgres-workers
```

## Run public Postgres embedding example

```bash
export HARNESS_POSTGRES_DSN='postgres://harness:harness@127.0.0.1:5432/harness_test?sslmode=disable'
go run ./examples/postgres-embedded
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
go test ./examples/platform-reference -count=1
go test -bench . ./pkg/harness/runtime
go test -run '^$' -bench RunStep -benchmem ./pkg/harness/runtime
```
