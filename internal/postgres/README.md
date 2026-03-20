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

Canonical public runtime bootstrap code now lives in:
- `pkg/harness/postgres`

That separation is intentional:
- `internal/postgres` owns SQL/repository primitives
- `pkg/harness/postgres` exposes public service/bootstrap wiring for embedding platforms
- `internal/postgresruntime` remains only as an internal compatibility wrapper
