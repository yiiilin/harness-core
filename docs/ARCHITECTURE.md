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

---

## Runtime architecture

```text
caller
 -> adapter (websocket initially)
 -> runtime kernel
    -> state machine
    -> context assembler
    -> planner hook
    -> tool registry
    -> executor
    -> verifier registry
    -> policy engine
    -> event sink / audit hooks
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
