# postgres-embedded

This example shows the public durable Postgres bootstrap path:

- `pkg/harness/builtins` for default tool and verifier registration
- `pkg/harness/postgres` for DB open, schema apply, repository wiring, and runtime construction
- a minimal in-process session/task/plan/step execution flow without any transport adapter

Run it with:

```bash
export HARNESS_POSTGRES_DSN='postgres://harness:harness@127.0.0.1:5432/harness_test?sslmode=disable'
go run ./examples/postgres-embedded
```
