# Postgres Workers Example

This example shows a durable multi-worker flow on top of the public Postgres bootstrap.

## What It Demonstrates

- boot multiple runtime instances through `pkg/harness/postgres`
- seed both runnable and recoverable work into the same Postgres-backed state
- contend for work through `ClaimRunnableSession` and `ClaimRecoverableSession`
- renew leases while work is executing
- run normal claimed execution and claimed recovery from separate runtime instances
- release leases after completion

It is intentionally still platform-neutral: the example shows kernel coordination semantics, not tenant routing, queueing, or deployment orchestration.

## Run

```bash
go test ./examples/postgres-workers -count=1
```

Or run it manually with your own database:

```bash
export HARNESS_POSTGRES_DSN='postgres://harness:harness@127.0.0.1:5432/harness_test?sslmode=disable'
go run ./examples/postgres-workers
```

## Expected Output

You should see a summary similar to:

```text
runnable: ...
recoverable: ...
attempts: 2
renewals: ...
worker-a handled runnable session ...
worker-b handled recoverable session ...
```

The important part is not which worker claims which session. The important part is:

- both runnable and recoverable work get completed
- leases are renewed while work is active
- leases are released afterwards
- persisted attempts are visible from shared durable state

## When To Use This Example

Use this as a reference when:

- building a multi-instance worker fleet on top of the kernel
- validating claim/lease behavior with shared durable state
- understanding how recoverable sessions differ from ordinary runnable sessions
