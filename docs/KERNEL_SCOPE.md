# KERNEL_SCOPE.md

## Goal

Define what `harness-core` is allowed to own and what must stay outside the kernel.

This document exists to keep the runtime small, transport-neutral, and reusable.
If a new concept or hook is proposed, this file is the first review gate.

---

## Core principle

`harness-core` is an execution kernel.
It is not an end-user product, tenant model, auth system, UI backend, or provider-orchestration layer.

The kernel should only own concepts that are required for:

- execution correctness
- recovery correctness
- replay and debugging stability
- governed execution
- global runtime semantics

If a concept does not change one of those, it should not enter the kernel.

---

## Multi-user platform note

A multi-user, multi-session agent platform can embed the kernel without teaching the kernel who the users are.

The kernel should provide:

- safe concurrent session execution, claim, lease, and recovery semantics
- governed approval and resume semantics
- durable execution facts for replay, audit, and debugging

The outer platform should provide:

- user / tenant / org ownership
- auth and visibility rules
- quota, billing, scheduling, and operational policy

Supporting many users and many sessions does not require `user_id` or `tenant_id` to become kernel concepts.

---

## Kernel owns

The following concerns belong in the kernel:

- task / session / plan / step lifecycle semantics
- the runtime loop: plan -> policy -> approval -> execute -> verify -> recover
- deterministic transition logic
- loop budgets and retry / replan accounting
- durable execution facts such as attempts, actions, verifications, artifacts, and runtime handles
- recovery semantics and restart-safe state reconstruction
- capability resolution and replay-stable capability snapshots
- transport-neutral target resolution hooks for executable target fan-out
- transport-neutral attachment materialization hooks when runtime semantics depend on them
- context assembly / compaction hooks when they affect runtime behavior globally
- transport-neutral control-plane primitives required for correct execution
- typed extension points that change runtime semantics globally

---

## Kernel does not own

The following concerns do not belong in the kernel:

- user / tenant / org / ownership / RBAC / ACL
- API gateway auth, bearer tokens, OIDC, session login state, or SSO flows
- WebSocket / HTTP / SSE / gRPC envelope semantics
- UI projections, dashboards, approval consoles, operator panels, or search pages
- billing, quotas, business reports, or analytics surfaces
- product-specific provider routing, prompt strategy, or business policy inheritance
- deployment topology, worker fleet management, queueing, or orchestration wiring
- transport-specific middleware, request wrappers, or adapter behavior
- module-specific capability implementation details such as browser UX, Windows UX, or PTY UX

These may exist in the same repository under `adapters/*`, `modules/*`, `internal/*`, or an embedding application.
They should not define core runtime concepts.

---

## Layer ownership

### Kernel: `pkg/harness/*`

Use the kernel layer when a concern changes runtime behavior globally.

Examples:

- `PolicyEvaluator`
- `Planner`
- `ContextAssembler`
- `Compactor`
- `CapabilityResolver`
- execution record stores
- recovery and coordination primitives

### Module: `modules/*`

Use a module when a concern changes how one capability family is implemented.

Examples:

- shell backend replacement
- filesystem path helper
- browser driver integration
- Windows executor integration

### Adapter: `adapters/*`

Use an adapter when a concern changes how the runtime is exposed to a host or transport.

Examples:

- WebSocket auth handshake
- HTTP request/response shape
- SSE event streaming
- CLI presentation

### Platform / embedding application

Use an outer platform layer when a concern is about ownership, governance, presentation, or deployment of runtime objects.

Examples:

- session ownership and visibility
- tenant partitioning
- approval UI and escalation workflow
- queue workers and scheduling fleet
- billing, analytics, and reporting

---

## Kernel admission test

Every proposed kernel concept should pass all of the following checks:

1. It changes runtime correctness or global runtime semantics.
2. It is transport-neutral.
3. It is identity-neutral.
4. It can be expressed as a small typed contract rather than product flags.
5. It cannot live cleanly in `modules/*`, `adapters/*`, or an embedding application.

If any check fails, the concept should stay out of the kernel.

---

## Fast rejection rules

Reject a kernel proposal immediately if it introduces:

- `tenant_id`, `user_id`, `org_id`, or ownership fields as first-class kernel concepts
- auth/session-login state
- transport request or response envelopes
- UI-specific workflow states
- billing, quota, or analytics fields
- product-provider or model-vendor routing logic
- module-specific behavior that does not alter the global runtime loop

---

## Public surface constraints

The kernel boundary applies to exported types and helper APIs, not only internal implementation.

Rules:

- exported `pkg/harness/*` types must remain transport-neutral and identity-neutral
- exported kernel metadata must not report adapter-specific or auth-specific modes
- `pkg/harness/*` must not depend on `adapters/*`
- convenience helpers that wire module packs are packaging helpers only; they do not expand kernel scope
- if a helper imports `modules/*`, keep that helper mechanical and keep module-specific UX/policy semantics out of kernel domain types

---

## Allowed examples

These are valid kernel additions if implemented cleanly:

- optimistic concurrency / compare-and-swap updates for mutable runtime records
- claim / lease primitives for runnable sessions
- abort / cancel semantics for execution control
- runtime handle lifecycle management
- plan-level capability freeze for replay stability
- target-resolution hooks that let embedders supply concrete execution targets without product semantics
- attachment-materialization hooks that keep input semantics transport-neutral
- richer event / metrics exporter hooks that remain vendor-neutral

---

## Review rule

When reviewing a kernel change, ask:

- Does this make the runtime more correct, more recoverable, or more replay-stable?
- Or does it merely make one product easier to build?

Only the first category belongs in `harness-core`.

---

## Summary

The kernel should stay:

- small
- execution-focused
- typed
- replayable
- transport-neutral
- identity-neutral

Everything else should live outside the kernel.
