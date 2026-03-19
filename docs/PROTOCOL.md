# PROTOCOL.md

## Goal

Define the runtime-neutral contracts used by `harness-core`.

This document is about:
- message envelopes
- task/session objects
- step/action/verify objects
- tool and verifier contracts
- event contracts

This document is not about any specific transport implementation. WebSocket is the first adapter, not the protocol itself.

---

## Design principles

1. **Library-first**
   - Protocol objects must not assume a specific server framework or client.
2. **Transport-neutral**
   - The same objects should work over WebSocket, HTTP, in-process, or RPC transports.
3. **Structured by default**
   - Actions, results, errors, verifies, and events must be machine-readable.
4. **Extensible but bounded**
   - Core fields are stable; scenario-specific data belongs in explicit extension fields.
5. **Verification-aware**
   - Every executable action should support explicit postcondition verification.

---

## Envelope

Transport adapters may wrap runtime messages in a common envelope.

```json
{
  "id": "req_001",
  "type": "request",
  "action": "session.create",
  "payload": {}
}
```

### Envelope fields
- `id`: client-generated correlation id
- `type`: one of `auth | request | response | event`
- `action`: operation name for requests
- `payload`: structured body

---

## Core object model

### TaskSpec
A task is the top-level intent container.

```json
{
  "task_id": "task_01",
  "task_type": "desktop_control",
  "goal": "Open the target app and enter text",
  "constraints": {},
  "metadata": {}
}
```

### SessionState
A session is the long-running execution context.

```json
{
  "session_id": "sess_01",
  "task_id": "task_01",
  "phase": "plan",
  "current_step_id": "step_01",
  "retry_count": 0,
  "summary": "",
  "metadata": {}
}
```

### PlanSpec
A plan is a revisioned sequence of steps.

```json
{
  "plan_id": "plan_01",
  "session_id": "sess_01",
  "revision": 1,
  "status": "active",
  "steps": []
}
```

### StepSpec
A step is the smallest executable unit in the runtime.

```json
{
  "step_id": "step_01",
  "title": "Launch application",
  "action": {},
  "verify": {},
  "on_fail": {},
  "metadata": {}
}
```

---

## ActionSpec

`ActionSpec` describes a tool invocation or executor operation.

```json
{
  "tool_name": "shell.exec",
  "args": {
    "mode": "pipe",
    "command": "echo hello",
    "timeout_ms": 10000
  }
}
```

### Required fields
- `tool_name`: globally unique tool id
- `args`: structured argument object

### Rules
- `tool_name` must be stable and registry-backed
- `args` must validate against the registered input schema
- action objects must not rely on implicit free-form text instructions

---

## VerifySpec

`VerifySpec` defines how completion is checked.

```json
{
  "mode": "all",
  "checks": [
    {
      "kind": "exit_code",
      "args": {
        "allowed": [0]
      }
    }
  ]
}
```

### Verify fields
- `mode`: `all | any`
- `checks`: list of verifier checks

### Check fields
- `kind`: verifier id
- `args`: structured verifier input

### Rules
- verification is explicit, not inferred from assistant text
- verifier kinds come from a registry, just like tools

---

## OnFailSpec

`OnFailSpec` defines the default recovery behavior for a step.

```json
{
  "strategy": "retry",
  "max_retries": 2,
  "backoff_ms": 1000
}
```

### Supported strategy values (initial)
- `retry`
- `reinspect`
- `replan`
- `abort`

---

## Tool contract

Every registered tool should conceptually expose:

```text
ToolDefinition {
  tool_name
  version
  capability_type
  input_schema
  result_schema
  risk_level
  metadata
}
```

### Result contract

```json
{
  "ok": true,
  "data": {},
  "meta": {
    "duration_ms": 42
  },
  "error": null
}
```

### Error contract

```json
{
  "ok": false,
  "data": null,
  "meta": {
    "duration_ms": 42
  },
  "error": {
    "code": "TOOL_FAILED",
    "message": "descriptive failure text"
  }
}
```

---

## Event contract

The runtime should emit structured events.

### Minimum event types
- `task.created`
- `session.created`
- `plan.generated`
- `step.started`
- `tool.called`
- `tool.completed`
- `tool.failed`
- `verify.completed`
- `state.changed`
- `task.completed`
- `task.failed`
- `policy.denied`

### Generic event shape

```json
{
  "event_id": "evt_01",
  "type": "tool.completed",
  "session_id": "sess_01",
  "step_id": "step_01",
  "payload": {},
  "created_at": 1770000000000
}
```

---

## Notes for adapters

### WebSocket adapter
- Maps request envelope to runtime calls
- Streams events as event envelopes
- Should not redefine core objects

### Future HTTP adapter
- May expose synchronous and asynchronous execution APIs
- Should reuse the same `TaskSpec`, `StepSpec`, `ActionSpec`, `VerifySpec`, and `Event` objects

---

## Summary

`harness-core` should own the protocol semantics, not just transport plumbing.

The core contracts are:
- `TaskSpec`
- `SessionState`
- `PlanSpec`
- `StepSpec`
- `ActionSpec`
- `VerifySpec`
- `OnFailSpec`
- `ToolDefinition`
- `ExecutionResult`
- `Event`


## Metrics and observability notes

The protocol does not currently expose a full metrics streaming API, but runtime operations should remain compatible with event and metrics hooks.

Current direction:
- structured runtime events are first-class
- metrics are collected via in-process hooks
- transport adapters may expose metrics later, but should not force kernel contract changes
