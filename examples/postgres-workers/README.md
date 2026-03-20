# postgres-workers

This example shows a durable multi-worker reference flow on top of the public Postgres bootstrap:

- boot multiple runtime instances through `pkg/harness/postgres`
- contend for runnable and recoverable sessions through claim/lease APIs
- renew claimed leases in the background while work is executing
- run and recover work against the same Postgres-backed runtime state
- release leases after completion

Run it with:

```bash
export HARNESS_POSTGRES_DSN='postgres://harness:harness@127.0.0.1:5432/harness_test?sslmode=disable'
go run ./examples/postgres-workers
```
