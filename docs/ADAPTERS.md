# ADAPTERS.md

## Goal

Define the public guidance for transport adapters that expose `harness-core`
without turning transport choices into kernel semantics.

This document applies to:

- repository-shipped companion adapters under `adapters/*`
- embedding platforms that wrap the kernel behind HTTP, WebSocket, RPC, SSE, or similar transports

This document does not make `adapters/*` part of the Tier 1 kernel stability
promise. It defines how adapters should behave when they expose kernel contracts.

See also:

- `docs/API.md`
- `docs/PROTOCOL.md`
- `docs/ADAPTER_PROTOCOL.md`
- `docs/EVENTS.md`
- `docs/VERSIONING.md`
- `docs/EMBEDDING.md`

## Adapter Boundary

Adapters are transport bindings, not execution kernels.

That means an adapter should:

- accept transport-specific envelopes and auth/session wrappers
- translate them into calls on public `pkg/harness/*` APIs
- return or stream kernel objects without redefining their meaning

An adapter should not:

- invent a parallel execution state machine
- bypass policy / approval / lease / recovery semantics
- encode user / tenant / UI concepts into kernel objects
- query internal tables directly when public runtime reads already exist

The rule is simple:

> transport shape is adapter-owned, runtime semantics are kernel-owned

## Public Dependency Rule

Adapters should depend on:

- `pkg/harness`
- `pkg/harness/postgres`
- `pkg/harness/worker`
- `pkg/harness/replay`
- companion composition/module packages when needed

Adapters should avoid:

- `internal/*`
- direct durable-table reads for session/task/approval/execution facts
- transport-specific forks of runtime semantics

Repository-shipped adapters may still use internal helpers during development,
but that should be treated as reference-layer debt rather than the preferred
embedding path.

## Event Streaming Rules

If an adapter streams runtime events, it should preserve the canonical event
envelope defined by the kernel.

Required behavior:

- keep kernel `event_id`, `sequence`, `type`, `session_id`, `task_id`, `step_id`, `attempt_id`, `action_id`, `verification_id`, `approval_id`, `cycle_id`, `trace_id`, and `causation_id` intact when present
- keep `payload` structured
- preserve per-session local event order
- wrap events in transport envelopes if needed, but do not rename core event fields inside the event payload

If an adapter does not stream events yet:

- document that clearly
- do not imply that request/response payloads are a full observability surface
- point embedders to runtime event/fact reads instead

## Error Mapping Rules

Kernel errors should be mapped through the transport-neutral error taxonomy
already documented in `docs/PROTOCOL.md`.

Recommended transport mapping:

- `invalid` -> HTTP `400`, request-validation failure, or transport-equivalent client error
- `not_found` -> HTTP `404` or transport-equivalent missing-resource error
- `conflict` -> HTTP `409` or transport-equivalent optimistic-concurrency error
- `lease` -> HTTP `409` or transport-equivalent ownership/claim conflict
- `state` -> HTTP `409` or `422`, depending on whether the caller can retry after state changes
- `budget` -> HTTP `429` or transport-equivalent budget exhaustion
- `runtime_handle` -> HTTP `409`, `410`, or transport-equivalent invalid-handle state
- `unknown` -> HTTP `500` or transport-equivalent server error

For request/response adapters:

- keep the transport error envelope small
- preserve machine-readable `code`
- avoid baking product/UI wording into kernel-originated errors

For streaming/event adapters:

- do not convert runtime events into human prose
- keep event/error records structured and correlation-friendly

## Payload Reuse Rules

Adapters may choose their own outer request/response envelope, path layout, or
action names.

Inside those envelopes, prefer reusing the kernel object shapes directly:

- `TaskSpec`
- `SessionState`
- `PlanSpec`
- `StepSpec`
- `ActionSpec`
- `VerifySpec`
- audit event envelopes
- execution facts and replay projections

If an adapter needs a product-specific projection:

- build it outside the kernel objects
- do not mutate the meaning of the kernel fields

## Versioning Rules

Adapters are public companion modules, not Tier 1 kernel packages.

Implications:

- adapter endpoints, action names, and transport envelopes can evolve on their own cadence
- kernel object meaning must still follow the root-module protocol docs
- breaking transport changes should be called out as adapter-module release changes, not kernel semantic changes

If an adapter exposes a protocol version:

- version the transport envelope or action namespace
- do not version kernel field names independently from the documented protocol unless the kernel docs change too

## Current Repository Guidance

Today:

- `adapters/http` is a thin reference HTTP control plane
- `adapters/websocket` is the richer reference adapter
- `adapters/websocket` transport actions are documented in `docs/ADAPTER_PROTOCOL.md`
- `adapters/websocket` exposes its own public `websocket.Config` for adapter-owned transport settings such as listen address and shared token
- embedders should not need `internal/config` just to construct a repository-shipped adapter

They are useful for examples and local integration, but they are not the
required durable/bootstrap path for embedders.

Do not move durable bootstrap settings such as:

- storage mode
- Postgres DSN
- schema
- migration policy

into adapter config types.
Those belong in the embedding app or public bootstrap helpers such as `pkg/harness/postgres.Config`.

Embedders should still treat:

- `pkg/harness/postgres` as the durable bootstrap entrypoint
- `pkg/harness/worker` as the reusable worker loop helper
- `pkg/harness/replay` as the preferred replay/debug projection helper

## Adapter Checklist

Before publishing or depending on an adapter, check:

- [ ] It uses public runtime/service APIs for execution semantics
- [ ] It preserves canonical event and execution-fact identifiers
- [ ] It maps kernel errors through the documented taxonomy instead of inventing new semantics
- [ ] It documents whether it is request/response-only or actually streams events
- [ ] It keeps user/tenant/auth/UI concepts outside kernel object definitions
- [ ] It does not require embedders to import `internal/*` for normal use

## Practical Summary

Adapters should be thin, explicit, and replaceable.

The adapter owns:

- envelopes
- auth handshakes
- path/action naming
- transport-specific status codes

The kernel owns:

- lifecycle semantics
- approval and resume correctness
- lease and recovery correctness
- event and execution-fact meaning
