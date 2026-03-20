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

The runtime event envelope also carries stable identifiers for:
- `task_id`
- `attempt_id`
- `action_id`
- `trace_id`
- `causation_id`

These identifiers exist to support replay, recovery, debugging, and observability consumers.

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
Today they are used for planner step bounds, compactor input, and tool-output truncation boundaries.

---

## Approval and resume

`permission.Ask` is a blocking runtime state, not only a semantic hint.

Current runtime behavior:
- create a durable approval record
- keep the step blocked until a reply exists
- expose `RespondApproval(...)`
- expose `ResumePendingApproval(...)`
- support `once`, `always`, and `reject`

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

---

## Summary

`harness-core` should be the execution kernel that gives structure to agent work:
- one shared state machine
- bounded loops
- explicit steps
- explicit verification
- revisioned planning
- eventful runtime behavior
