# RUNTIME.md

## Goal

Describe the execution model of `harness-core` as a reusable runtime kernel.

This document focuses on:
- state machine
- execution loop
- session lifecycle
- plan revisions
- model vs program responsibilities

---

## Runtime philosophy

`harness-core` should not be a giant autonomous product runtime.
It should be a compact execution kernel that other systems can embed.

That means:
- stable state machine
- stable step contracts
- explicit verification
- deterministic transitions where possible
- model involvement only at cognition-heavy decision points

Boundary rule:
- the runtime may own execution semantics
- the runtime may not own tenant, user, auth, UI, or transport concepts

See `docs/KERNEL_SCOPE.md`.

For the stable embedding path, prefer `pkg/harness` as the first import.
That facade is intended to expose the kernel constructor plus the session-level control plane:
- `RunStep`
- `RunSession`
- `RecoverSession`
- `AbortSession`
- approval response / resume
- coordination lease primitives
- context compaction
- runtime handle lifecycle control

A multi-user, multi-session agent platform should rely on the kernel for session correctness, recovery, approvals, and execution facts.
It should keep identity, ownership, auth, quota, routing, and UI concerns outside the kernel.

---

## Main state machine

Recommended shared top-level state machine:

```text
RECEIVED
-> PREPARE
-> PLAN
-> EXECUTE
-> VERIFY
-> RECOVER
-> COMPLETE / FAILED / ABORTED
```

### State meanings
- `RECEIVED`: task accepted, no preparation yet
- `PREPARE`: gather context, validate prerequisites, load policies
- `PLAN`: determine next step or revise plan
- `EXECUTE`: invoke action/tool
- `VERIFY`: check postconditions
- `RECOVER`: retry, reinspect, or replan
- `COMPLETE`: task finished successfully
- `FAILED`: unrecoverable failure
- `ABORTED`: cancelled by user/system

---

## Runtime loop

Minimal execution loop:

```text
load state
-> assemble context
-> decide next step
-> evaluate policy
-> execute tool
-> verify result
-> update state
-> continue or stop
```

### Important principle
Not every transition should involve a model call.

Use the model for:
- task decomposition
- tool selection
- ambiguity resolution
- failure reasoning
- knowledge synthesis

Use program logic for:
- retries
- retry backoff enforcement
- timeout handling
- policy checks
- deterministic success/failure checks
- session completion decisions

---

## Plan model

A plan should be revisioned.

```json
{
  "plan_id": "plan_01",
  "session_id": "sess_01",
  "revision": 1,
  "status": "active",
  "steps": []
}
```

### Replanning rules
- replanning is allowed
- replanning is not free-form chaos
- every revision must capture a reason
- revisions should be bounded by policy

Recommended triggers for replan:
- verification failed
- prerequisite not satisfied
- environment changed
- user changed goal

Recommended non-triggers:
- model idle wandering
- gratuitous step expansion
- speculative complexity without new evidence

---

## Session lifecycle

A session owns:
- the active task
- current plan revision
- current step pointer
- working/session memory bindings
- pending approvals
- latest artifacts and tool results

Recommended lifecycle:

```text
create session
-> attach task
-> run loop
-> summarize/compact if needed
-> complete or fail
-> optionally persist summary
```

### Lease, heartbeat, and reclaim semantics

The kernel exposes lease primitives so an embedding platform can coordinate runnable and recoverable sessions without pushing worker-fleet concepts into core.

The contract is:
- claim order is deterministic: oldest `created_at` wins, with `session_id` as the stable tie-breaker
- `LeaseExpiresAt` is the authority boundary for a claim; once `now >= lease_expires_at`, the previous holder is stale
- `RenewSessionLease` is the lease heartbeat API; a successful renew extends `LeaseExpiresAt` and refreshes `LastHeartbeatAt`
- `LastHeartbeatAt` is an observational liveness timestamp, not an independent reclaim rule; reclaim decisions are driven by lease expiry
- a stale holder cannot renew or release its lease; those calls fail with `session.ErrSessionLeaseNotHeld`
- `ClaimRunnableSession` and `ClaimRecoverableSession` may reclaim work only when no active lease exists or the previous lease has expired
- direct execution and resume APIs are valid only when no active lease exists for the session
- claimed execution APIs (`RunClaimedStep`, `RunClaimedSession`, `ResumeClaimedApproval`) require the matching active lease id
- claim-aware recovery state APIs (`MarkClaimedSessionInFlight`, `MarkClaimedSessionInterrupted`, `RecoverClaimedSession`) use the same rule, so stale holders cannot keep mutating state after reclaim

Recovery follows the same rule:
- `RecoverSession` is valid only when the session has no active recoverable lease
- if a caller has already claimed a recoverable session, it must continue via `RecoverClaimedSession(session_id, lease_id)`
- if another holder still owns an unexpired recoverable lease, recovery must fail cleanly rather than relying on transport-specific worker identity

Approval resume follows the same rule:
- a still-pending approval remains non-runnable because the session stays in `awaiting_approval`
- once approval is granted and the session returns to `idle`, it may be claimed again as runnable work
- after claim, resume must continue through the claimed API or the session driver on `RunClaimedSession`

This gives the kernel a transport-neutral correctness rule:
- active holder keeps exclusive recovery authority until lease expiry
- after expiry, reclaim is explicit and deterministic
- queue topology, worker pools, and heartbeating policy remain outside core

---

## Transition decisions

The runtime should expose a small, explicit transition model.

Example:

```json
{
  "from": "verify",
  "to": "recover",
  "step_id": "step_01",
  "reason": "verification failed"
}
```

This gives the runtime a stable internal language for:
- audit logs
- traces
- replay
- unit tests
- policy reasoning

A simple kernel can own deterministic transitions such as:
- `RECEIVED -> PREPARE`
- `PREPARE -> PLAN`
- `EXECUTE -> VERIFY`
- `VERIFY -> COMPLETE|RECOVER`

---

## Execution records and event envelope

The runtime now treats execution facts as first-class records, not only opaque step payloads.

That includes:
- attempts
- action records
- verification records
- artifacts
- runtime handles

These records now also support a logical `cycle_id`.

Use the ids this way:
- `attempt_id` identifies a concrete attempt record
- `cycle_id` groups one logical execution cycle across approval blocking, resumed execution, verification, artifacts, and runtime handles
- `trace_id` remains the observability correlation id for events and spans emitted during that cycle

Runtime-handle lifecycle is also kernel-governed:
- `active` handles may be updated, closed, or invalidated
- `closed` and `invalidated` handles are terminal execution facts
- control-surface mutations against terminal handles must fail cleanly instead of silently reopening or rewriting them
- runtime handles now also carry a monotonically increasing `version` so conflicting mutations can fail cleanly instead of silently overwriting newer state
- the unclaimed runtime-handle control surface is lease-aware: if the owning session has an active lease, direct handle mutation must fail rather than racing the current session holder

The runtime event envelope also carries stable identifiers for:
- `approval_id`
- `task_id`
- `attempt_id`
- `action_id`
- `verification_id`
- `cycle_id`
- `trace_id`
- `causation_id`
- `sequence`

These identifiers exist to support replay, recovery, debugging, and observability consumers.

Recommended embedder read path:
- `ListExecutionCycles(session_id)`
- `GetExecutionCycle(session_id, cycle_id)`
- `ListAuditEvents(session_id)`
- optional projection helper: `pkg/harness/replay`
  - current replay projections add target slices, interactive runtimes, blocked-runtime views, and structured program lineage without requiring direct storage reads

This keeps replay/debug consumers on public runtime contracts instead of storage internals.

---

## Loop budgets

To avoid drift and runaway loops, the runtime should support bounded execution.

Recommended controls:
- `max_steps`
- `max_retries_per_step`
- `max_plan_revisions`
- `max_total_runtime_ms`
- `max_tool_output_chars`

These should be configurable, but present in v1.

The current runtime exposes these via `runtime.Options.LoopBudgets`.
Today they are used for:
- planner step bounds
- plan revision caps
- step retry caps
- total runtime preflight guards
- compactor input
- tool-output truncation boundaries

### Current `OnFail` runtime behavior

The kernel currently treats `OnFailSpec` as executable runtime policy, not planner-only metadata.

- `abort` fails the session after verification failure
- `replan` routes the session back to planning after verification failure
- `retry` keeps the session recoverable and may persist `step.metadata.retry_not_before` when `backoff_ms` is set
- `reinspect` re-enters `PREPARE` before the next attempt and may also persist `step.metadata.retry_not_before` when `backoff_ms` is set
- when `retry_not_before` is still in the future, direct step execution must fail cleanly and the session driver must stop instead of busy-looping

### Replaceable kernel clock

Time-sensitive kernel behavior should not be hard-coded to wall-clock reads.

The runtime now supports a small replaceable clock surface for:
- retry backoff enforcement
- runtime budget checks
- lease claim / renew / release timestamps
- runtime-handle lifecycle timestamps

This exists for deterministic tests and replay-oriented embeddings.
Adapters and platforms may still use real time by default; they should not need to fork kernel logic just to freeze or advance time in tests.

---

## Approval and resume

`permission.Ask` is a blocking runtime state, not only a semantic hint.

Current runtime behavior:
- create a durable approval record
- keep the step blocked until a reply exists
- expose `RespondApproval(...)`
- expose `ResumePendingApproval(...)`
- support `once`, `always`, and `reject`
- keep second confirmation on the generic blocked-runtime path via `RequestConfirmation(...)`, not as an approval reply
- scope `always` reuse to the recorded approval request shape, matched rule, and resolved capability version
- persist the blocked step's logical execution cycle so resumed or recovered execution facts stay attached to the same cycle

Direct `action.invoke` style execution is intentionally unsupported at the kernel level.
All governed tool execution should pass through the step runtime path so policy, approval, execution facts, and audit stay in one chain.

WebSocket adapters surface the same kernel path through:
- `approval.get`
- `approval.list`
- `approval.respond`
- `session.resume`

---

## Runtime responsibilities vs caller responsibilities

### harness-core runtime should own
- state transitions
- action dispatch
- verifier dispatch
- retry accounting
- plan revision bookkeeping
- event emission
- audit hook calls

### embedding application should own
- auth integration beyond shared token
- UI / dashboards / CLI polish
- actual model provider integration strategy
- persistence backend implementation details
- tenant/business-specific policy

Those are not deferred kernel features.
They are intentionally outside kernel scope.

---

## Compaction and summarization

Compaction is part of the runtime lifecycle, not an afterthought.

When session context gets too large:
- prune low-value artifacts/tool outputs first
- summarize session state into durable form
- preserve enough structure to continue execution

This should happen under explicit runtime control, not hidden model magic.

The current runtime exposes:
- typed `ContextPackage` assembly
- a replaceable `Compactor`
- durable `ContextSummaryStore` hooks

Compaction currently runs through the planner-context assembly path so the kernel, not demo code, owns the entry point.

---

## Capability resolution

Tool execution now resolves a capability snapshot before invoking the handler.

The runtime exposes:
- `CapabilityResolver`
- `CapabilitySnapshots`

The resolved snapshot captures stable capability metadata such as:
- tool name
- version
- capability type
- risk level

Execution against a frozen plan now validates the live capability definition against the frozen view before handler invocation.
If the pinned capability version disappears or the live definition drifts in stable fields such as capability type, risk level, or metadata, execution must fail cleanly rather than silently replay against a changed capability.

This keeps action execution replay-friendly even when the live registry continues to evolve.

---


## Default runtime components

The current runtime ships with small, explicitly limited defaults:

- `DefaultContextAssembler`
  - returns a minimal typed context package for task + session + metadata
- `NoopCompactor`
  - preserves assembled context unchanged until an embedding app installs a real compactor
- `NoopPlanner`
  - returns `ErrNoPlannerConfigured` and is safe by default
- `DemoPlanner`
  - a tiny example planner that can derive a shell step for trivial goals such as `echo hello`
- `AuditStoreSink`
  - bridges runtime events into the configured audit store and can be rebound inside transaction scopes

These defaults are intentionally simple. Their purpose is to:
- demonstrate composition
- support tests/examples
- remain replaceable by embedding applications

They are not intended to be feature-complete production planning systems.

## Recommended v1 runtime scope

Include:
- task/session model
- main state machine
- step execution loop
- plan revision support
- verifier integration
- event emission hooks
- shell executor support

Exclude from v1:
- distributed runtime scheduling
- complex multi-agent orchestration
- long-lived public tenant runtime controls
- multi-user ownership / session listing / UI projections
- deeply coupled model-provider logic

See `docs/KERNEL_SCOPE.md` for the stronger invariant behind this exclusion list.

---

## Summary

`harness-core` should be the execution kernel that gives structure to agent work:
- one shared state machine
- bounded loops
- explicit steps
- explicit verification
- revisioned planning
- eventful runtime behavior
