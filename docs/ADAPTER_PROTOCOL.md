# ADAPTER_PROTOCOL.md

## Goal

Document the adapter-facing transport mapping for the repository-shipped
WebSocket adapter without redefining kernel semantics.

This is the concrete companion to:

- `docs/ADAPTERS.md`
- `docs/PROTOCOL.md`
- `docs/API.md`

## Scope

This document describes the current `adapters/websocket` surface:

- request / response envelope shape
- approval and blocked-runtime actions
- interactive control actions
- audit replay / event streaming behavior
- compatibility rules for embedders

It does not turn the WebSocket action names into Tier 1 kernel APIs.
Those action names belong to the adapter module.

## Envelope

The reference adapter uses the transport envelope documented in `docs/PROTOCOL.md`:

```json
{
  "id": "req_001",
  "type": "request",
  "action": "interactive.start",
  "payload": {}
}
```

Current envelope types:

- `auth`
- `request`
- `response`
- `event`

## Authentication

The reference adapter accepts a shared-token bootstrap model:

1. connect to `/ws`
2. send `{"type":"auth","token":"..."}`
3. wait for a normal `response` envelope with `ok: true`

The transport auth shape is adapter-owned.
The authenticated runtime semantics are still kernel-owned.

## Approval And Blocked Runtime Mapping

Current approval-oriented actions:

- `approval.get`
- `approval.list`
- `approval.respond`
- `session.resume`

Current blocked-runtime read actions:

- `blocked_runtime.get`
  - payload: `{"session_id":"..."}` or `{"blocked_runtime_id":"..."}`
- `blocked_runtime.get_by_approval`
  - payload: `{"approval_id":"..."}`
- `blocked_runtime.list`
- `blocked_runtime_projection.get`
  - payload: `{"session_id":"..."}` or `{"approval_id":"..."}`
- `blocked_runtime_projection.list`

Mapping rules:

- approval records remain the kernel approval objects
- blocked-runtime payloads remain the kernel blocked-runtime records / projections
- `approval.respond` changes approval state only; `session.resume` is still the explicit resume step
- blocked-runtime projection payloads expose the kernel wait shape, target slices, and interactive runtimes without adapter-specific reinterpretation

## Interactive Control Mapping

The reference WebSocket adapter now exposes the kernel interactive control plane directly:

- `interactive.get`
  - payload: `{"handle_id":"..."}`
- `interactive.list`
  - payload: `{"session_id":"..."}`
- `interactive.start`
  - payload: `{"session_id":"...","request":{...}}`
- `interactive.reopen`
  - payload: `{"handle_id":"...","request":{...}}`
- `interactive.view`
  - payload: `{"handle_id":"...","request":{"offset":0,"max_bytes":4096}}`
- `interactive.write`
  - payload: `{"handle_id":"...","request":{"input":"..."}}`
- `interactive.close`
  - payload: `{"handle_id":"...","request":{"reason":"..."}}`

The nested `request` object reuses the public runtime request types:

- `InteractiveStartRequest`
- `InteractiveReopenRequest`
- `InteractiveViewRequest`
- `InteractiveWriteRequest`
- `InteractiveCloseRequest`

The response bodies reuse the public runtime result / projection objects:

- `InteractiveRuntime`
- `InteractiveViewResult`
- `InteractiveWriteResult`

This is intentionally transport-neutral at the object layer:

- the adapter owns the action names
- the kernel owns the meaning of runtime handles, offsets, statuses, observations, and capability flags

## Audit Replay And Event Streaming

Current audit-oriented actions:

- `audit.list`
- `event.replay`

`audit.list` returns the stored audit event objects as a normal response payload.

`event.replay` is a request-scoped replay stream:

1. the adapter first sends a normal `response` envelope with a small summary such as replay count
2. it then sends one or more `event` envelopes with:
   - `type: "event"`
   - `action: "audit.event"`
   - `payload: <kernel audit event>`

Important limitation:

- this is not yet a live unsolicited event subscription stream
- today it is explicit replay-over-WebSocket, not a general push channel

Embedders that need live streaming should treat that as adapter work, not as a reason to fork kernel event semantics.

## Compatibility Rules

For embedders and adapter authors:

- kernel object fields and meanings must follow `docs/API.md` and `docs/PROTOCOL.md`
- adapter action names and outer envelopes may evolve under the `adapters` module release cadence
- transport changes must not silently rename or reinterpret kernel identifiers such as `session_id`, `approval_id`, `attempt_id`, `action_id`, `trace_id`, or `handle_id`
- if an adapter adds a protocol version, version the action namespace or outer envelope, not the kernel field names by themselves
- request/response success is not a substitute for replay or audit surfaces; use `audit.list`, `event.replay`, and execution-fact reads when you need observability

## Practical Summary

The current reference WebSocket adapter now exposes:

- normal lifecycle reads/writes
- approval response and resume
- blocked-runtime inspection
- interactive control
- audit replay

It is still a companion transport module, not the kernel API itself.
