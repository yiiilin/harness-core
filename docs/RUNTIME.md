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

## Loop budgets

To avoid drift and runaway loops, the runtime should support bounded execution.

Recommended controls:
- `max_steps`
- `max_retries_per_step`
- `max_plan_revisions`
- `max_total_runtime_ms`
- `max_tool_output_chars`

These should be configurable, but present in v1.

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

---

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
