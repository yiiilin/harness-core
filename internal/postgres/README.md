# internal/postgres

This package tree is the starting point for the durable Postgres-backed storage layer.

Goals of this package:
- provide a Postgres transaction runner
- provide repository implementations for session/task/plan/audit
- keep SQL/backend details out of `pkg/harness/*`

Current status:
- schema and migration skeleton added
- SQL-backed transaction runner added
- repository factory skeleton added
- repository implementations not yet completed
