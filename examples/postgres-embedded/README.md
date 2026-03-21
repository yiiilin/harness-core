# Postgres Embedded Example

This example shows the smallest durable embedding path through the public Postgres bootstrap.

## What It Demonstrates

- register the default built-in tools and verifiers
- open a durable runtime through `pkg/harness/postgres`
- let the public bootstrap handle DB open, schema apply, repository wiring, and runtime construction
- create a session, task, and plan directly against durable storage
- run one verified step and read back persisted attempts

This example stays adapter-free on purpose. It is the durable counterpart to `examples/minimal-agent`.

## Run

```bash
go test ./examples/postgres-embedded -count=1
```

Or run it manually with your own database:

```bash
export HARNESS_POSTGRES_DSN='postgres://harness:harness@127.0.0.1:5432/harness_test?sslmode=disable'
go run ./examples/postgres-embedded
```

## Expected Output

You should see a short summary like:

```text
storage: postgres
session: ... (complete)
attempts: 1
output: hello from durable runtime
```

## When To Use This Example

Use this as a reference when:

- embedding the runtime directly inside another Go service
- moving from in-memory development to durable Postgres-backed state
- wanting the public bootstrap path rather than `internal/*` wiring
