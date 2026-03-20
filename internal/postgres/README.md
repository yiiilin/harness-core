# internal/postgres

This package tree contains the durable Postgres-backed storage internals for `harness-core`.

Goals of this package:
- provide a Postgres transaction runner
- provide repository implementations for session/task/plan/audit
- keep SQL/backend details out of `pkg/harness/*`

Current status:
- schema and migration files exist
- SQL-backed transaction runner exists
- repository factory exists
- repository implementations exist for session/task/plan/audit

Related runtime bootstrap code lives in:
- `internal/postgresruntime`

That separation is intentional:
- `internal/postgres` owns SQL/repository primitives
- `internal/postgresruntime` owns service/bootstrap wiring
