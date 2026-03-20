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

### Runtime semantics
- effective retry budget is bounded by both runtime `max_retries_per_step` and step-local `max_retries`
- `abort` fails the session when verification fails
- `replan` routes back to planning after verification failure
- `retry` and `reinspect` keep the session in recovery only while retry budget remains

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
- `approval.requested`
- `approval.approved`
- `approval.rejected`
- `tool.called`
- `tool.completed`
- `tool.failed`
- `verify.completed`
- `state.changed`
- `session.aborted`
- `task.completed`
- `task.failed`
- `task.aborted`
- `policy.denied`

### Canonical event envelope

Runtime-generated events should use a stable envelope with correlation ids when they exist:

```json
{
  "event_id": "evt_01",
  "type": "plan.generated",
  "session_id": "sess_01",
  "task_id": "task_01",
  "planning_id": "pln_01",
  "payload": {
    "plan_id": "plan_01",
    "revision": 2
  },
  "created_at": 1710000000000
}
```

### Event envelope rules
- `session_id` is required for session-scoped runtime events.
- `task_id` should be present whenever a task is attached to the session.
- `planning_id` should be present for planner-derived plan generation events when the runtime persisted a planning cycle record.
- `attempt_id` and `trace_id` should be present for step execution events.
- `action_id` is required for tool invocation/completion/failure events.
- `verification_id` is required for verification events.
- `causation_id` should point to the record that directly caused the event, such as an action or attempt record.
- adapter envelopes may wrap these objects, but must not redefine the meaning of the core fields.

---

## Observability exporters

The kernel may optionally export vendor-neutral metric samples and trace spans in addition to audit events.

### MetricSample
- `name`: stable sample name such as `step.run`, `planning.cycle`, `approval.request`, `approval.response`, `session.recover`, `session.abort`, `lease.claim`, `lease.renew`, `lease.release`
- `labels`: string correlation labels such as `session_id`, `task_id`, `planning_id`, `approval_id`, `lease_id`, `step_id`, `attempt_id`, `trace_id`
- `fields`: structured numeric/boolean detail
- `recorded_at`: emission timestamp

### TraceSpan
- `name`: stable span name such as `tool.invoke`, `verify.evaluate`, `planning.cycle`, `approval.request`, `approval.response`, `session.recover`, `session.abort`, `lease.claim`, `lease.renew`, or `lease.release`
- `trace_id`: correlation id shared across related spans and events
- `span_id`: id for the current span
- `parent_id`: parent span id when the span is nested
- correlation fields such as `session_id`, `task_id`, `planning_id`, `approval_id`, `lease_id`, `step_id`, `attempt_id`, `action_id`, `verification_id`, `causation_id`
- `started_at` / `finished_at`: timestamps for latency analysis
- `attributes`: structured vendor-neutral attributes

### Exporter rules
- exporter contracts must remain transport-neutral and vendor-neutral
- exporters are additive observability hooks, not the source of truth for runtime state
- audit events remain the canonical replay/debug envelope even when exporters are configured

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
