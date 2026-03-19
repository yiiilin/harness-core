# PERSISTENCE.md

## Goal

Describe the persistence direction of `harness-core` beyond the current in-memory stores.

The project already has store boundaries for:
- session
- task
- plan
- audit

What is still missing is a **production-grade persistence boundary** for multi-object runtime updates.

---

## Current state

Today the runtime uses in-memory stores for development and tests.

This is good for:
- fast local iteration
- deterministic tests
- refining contracts

It is not sufficient for:
- restart recovery
- durable audit history
- real deployments
- consistent multi-object step commits

---

## Why a persistence abstraction is needed

A single `RunStep()` may update multiple things at once:
- session state
- plan step status
- task status
- audit events

If these writes are not grouped under a durable transaction boundary, the runtime can end up in inconsistent states.

Examples:
- session updated, but plan step not updated
- task marked failed, but audit event missing
- event persisted, but session state not advanced

So production persistence is not just about "having Postgres".
It is about **consistent step-level commits**.

---

## Recommended direction

### Phase 1
Keep domain-specific store interfaces, but introduce a higher-level boundary:
- `RepositorySet`
- `UnitOfWork`
- `Runner`

### Phase 2
Use an in-memory implementation of `UnitOfWork` to validate semantics.

### Phase 3
Add durable repository implementations (e.g. Postgres-backed).

### Phase 4
Add recovery semantics and optimistic concurrency/version checks.

---

## Proposed abstractions

### RepositorySet
A grouped collection of repositories used by the runtime.

Should at least include:
- `Sessions`
- `Tasks`
- `Plans`
- `Audits`

### UnitOfWork / Runner
A transaction-like boundary for a runtime step.

Should support:
- executing a function with repository access
- committing atomically where supported
- rolling back where supported

In-memory implementations may emulate this behavior without a real database transaction.

---

## What production-grade means here

At minimum:
- durable session/task/plan/audit storage
- consistent step-level updates
- restart-safe state reconstruction
- path toward optimistic locking / revision checking

---

## Non-goals for now

Not yet in scope:
- event sourcing as the primary persistence model
- distributed scheduling/leases
- Redis as source of truth

Recommended first durable source of truth:
- Postgres

Redis can come later for:
- caching
- queueing
- locks
- leases

---

## Design note

The next persistence milestone is not just “add database code”.
It is:

> make the runtime capable of executing a step through a grouped persistence boundary.

That means the runtime should eventually be able to route critical updates through:
- a `RepositorySet`
- a `Runner`
- a step-level commit point

Before durable storage exists, this semantic boundary should still be visible in the codebase.

---

## Summary

The next persistence milestone for `harness-core` is not "add a database package".
It is:

> define and enforce a step-level persistence boundary.

That is the job of `RepositorySet` + `UnitOfWork` / `Runner`.
