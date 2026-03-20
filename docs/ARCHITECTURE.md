# ARCHITECTURE.md

## Positioning

`harness-core` is not an end-user agent product.
It is a reusable harness runtime kernel.

Primary goals:
- compact
- efficient
- composable
- transport-neutral at the core
- suitable for embedding inside higher-level agent systems

Non-goals for v1:
- full SaaS platform
- rich UI
- multi-tenant product surface
- giant built-in tool ecosystem

Boundary rule:
- if a concern is not transport-neutral and identity-neutral, it should not become a kernel concept

See `docs/KERNEL_SCOPE.md` for the hard boundary.

---

## Recommended shape

Monolith-first library + adapter layout:

```text
pkg/harness/
  task/
  session/
  plan/
  action/
  verify/
  tool/
  runtime/
  permission/
  audit/
  observability/
  memory/

modules/
  shell/
  filesystem/
  http/

adapters/
  websocket/

examples/
  minimal-agent/
  websocket-runtime/
  go-client/
```

Rationale:
- keep the runtime kernel small and reusable
- keep transport and deployment concerns at the edge
- make examples and adapters consumers of the same library contracts

Preferred mental model:
- bare kernel: `pkg/harness/runtime` plus the domain contracts under `pkg/harness/*`
- capability packs: `modules/*`
- transport bindings: `adapters/*`
- ownership / auth / scheduling / UI product layer: embedding platform

---

## Core concepts

### Task
Top-level objective container.

### Session
Long-running execution context and lifecycle container.

### Plan
Revisioned set of steps for accomplishing a task.

### Step
Smallest executable unit with action, verification, and failure strategy.

### ToolDefinition
Registry-backed executable capability contract.

### Verifier
Registry-backed postcondition checker.

### Event
Structured runtime/audit/observability record.

### Approval
Durable pending-approval record plus explicit resume decision.

### Capability snapshot
Resolved tool metadata captured before invocation for replay/debug stability.

---

## Runtime architecture

```text
caller
 -> adapter (websocket initially)
 -> runtime kernel
    -> state machine
    -> context assembler
    -> context compactor / summary store
    -> planner hook
    -> tool registry
    -> capability resolver / snapshot store
    -> executor
    -> verifier registry
    -> policy engine
    -> approval store / resume policy
    -> event sink / audit hooks
    -> metrics hook
```

The runtime kernel should own:
- state transitions
- action dispatch
- verifier dispatch
- retry and replan decisions
- event generation

The embedding application should own:
- deployment model
- user auth integration
- external storage/runtime wiring
- UI / operator experience
- multi-user / tenant ownership and projections

Do not move those concerns into `pkg/harness/*`.
See `docs/KERNEL_SCOPE.md`.

---

## Multi-user / Multi-session Platform Embedding

`harness-core` is intended to sit inside a platform that may host many users and many active sessions at once.

The kernel must still own the runtime semantics that make that safe:
- session claim / lease / reclaim primitives
- approval blocking and resume semantics
- restart-safe recovery behavior
- durable execution facts and correlation ids

The platform must own the concepts that make that a product:
- actor identity
- ownership and visibility
- auth, quota, billing, and policy overlays
- parent/child session trees and orchestration topology
- transport protocols, worker fleets, and operator tooling

This split is intentional. Multi-user scale is a deployment property, not a reason to move identity into the kernel.

---

## Storage direction

Chosen direction for v1:
- durable state can start in Postgres
- Redis is optional later
- in-memory development mode allowed for local examples

Important: storage concerns should sit behind interfaces so the kernel is not coupled to a single persistence strategy.

---

## Initial implementation order

1. stable contracts (`TaskSpec`, `SessionState`, `ActionSpec`, `VerifySpec`, `Event`)
2. runtime loop
3. tool registry
4. verifier registry
5. policy evaluator
6. minimal shell executor example
7. websocket adapter example
8. audit/event sink example

---

## Summary

`harness-core` should aim to be:
- a standard runtime core
- a contract library
- a small execution kernel

It should not try to be the entire agent product.


## Public API boundary

The runtime kernel should remain small and generic.

Preferred long-term structure:
- core contracts live in `pkg/harness/*`
- capability packs live in `modules/*`
- transport bindings live in `adapters/*`
- orchestration trees above a session belong in an embedding layer, not in `session.State`

See `docs/PACKAGE_BOUNDARIES.md` for guidance on which packages consumers should import directly.
See `docs/KERNEL_SCOPE.md` for the rule set that decides whether a new concept belongs in the kernel at all.
