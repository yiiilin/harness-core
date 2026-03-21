# EVENTS.md

## Goal

Describe the current event model used by `harness-core`.

The runtime emits structured events as part of step execution. These events are currently:
- returned in `StepRunOutput.Events`
- written to the configured audit sink

This document is about the semantic meaning of those events, not about any single transport.

---

## Current event types

### `task.created`
A task record was created.

### `session.created`
A session was created.

### `plan.generated`
A plan revision was created.

### `step.started`
A step entered execution.

### `tool.called`
A tool invocation was dispatched.

### `tool.completed`
A tool invocation completed successfully.

### `tool.failed`
A tool invocation completed unsuccessfully or returned an execution failure.

### `verify.completed`
Verifier evaluation finished.

### `state.changed`
The runtime moved from one state to another.

### `policy.denied`
A policy evaluator denied execution.

### `task.completed`
The task reached terminal success.

### `task.failed`
The task reached terminal failure.

---

## Event shape

```json
{
  "event_id": "evt_01",
  "sequence": 42,
  "type": "tool.completed",
  "session_id": "sess_01",
  "approval_id": "apv_01",
  "step_id": "step_01",
  "attempt_id": "att_01",
  "action_id": "act_01",
  "verification_id": "ver_01",
  "cycle_id": "cyc_01",
  "trace_id": "trc_01",
  "causation_id": "act_01",
  "payload": {},
  "created_at": 1770000000000
}
```

### Notes
- `event_id` should be populated for runtime-emitted events, including in-memory flows
- `sequence` should preserve local emit order even when multiple events share the same timestamp
- `session_id`, `step_id`, `attempt_id`, `action_id`, `verification_id`, `approval_id`, `cycle_id`, `trace_id`, and `causation_id` allow replay/debug correlation without payload scraping
- `payload` is intentionally free-form but should remain structured JSON

---

## Event ordering expectations

For a successful `RunStep()` call, a typical event sequence is:

```text
step.started
tool.called
tool.completed
verify.completed
state.changed
```

For a denied path, a typical sequence is:

```text
step.started
policy.denied
state.changed
```

For a tool failure path, a typical sequence is:

```text
step.started
tool.called
tool.failed
verify.completed
state.changed
```

The runtime should not guarantee perfect global ordering across future distributed adapters,
but should preserve per-step ordering within a single local execution.

---

## Design rule

Events are not human prose. They are machine-readable state breadcrumbs.

That means:
- event names should remain stable
- payloads should remain structured
- events should support replay, audit, and debugging

---

## Relationship to metrics

Events answer:
- what happened
- in what order
- with what payload

Metrics answer:
- how often it happened
- how long it took
- how success/failure ratios evolve over time

Both are necessary and complementary.
