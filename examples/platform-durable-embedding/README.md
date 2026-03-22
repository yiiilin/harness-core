# Platform Durable Embedding Example

This example shows a more realistic durable embedding flow than `examples/postgres-embedded`.

## What It Demonstrates

- open a durable runtime through `pkg/harness/postgres.OpenServiceWithConfig(...)`
- configure durable bootstrap through public `hpostgres.Config`
- keep an external `run_id -> session_id` mapping in the platform layer
- pause on approval through normal kernel approval semantics
- simulate a service restart by closing and reopening the durable runtime
- continue the same durable session through `RespondApproval(...)` and `ResumePendingApproval(...)`

The point is not to build a product UI. The point is to show the control flow that an embedding platform would wire around the kernel.

## Run

```bash
go test ./examples/platform-durable-embedding -count=1
```

Or run it manually:

```bash
export HARNESS_POSTGRES_DSN='postgres://harness:harness@127.0.0.1:5432/harness_test?sslmode=disable'
export HARNESS_POSTGRES_SCHEMA='platform_demo'
go run ./examples/platform-durable-embedding
```

## Expected Output

You should see a short summary like:

```text
run_id: run_platform_durable_embedding
session_id: ...
approval_id: ...
final_phase: complete
output: approved durable embedding
```

## When To Use This Example

Use this as a reference when:

- your platform already has its own external run identifiers
- approvals are handled by your own UI or operator workflow
- you need to reopen durable state and continue the same kernel session after process restart
- you want to use the public Postgres bootstrap path instead of copying bootstrap wiring
