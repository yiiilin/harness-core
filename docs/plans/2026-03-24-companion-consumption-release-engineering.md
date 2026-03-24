# 2026-03-24 Companion Consumption And Release Engineering

## Goal

Finish the non-kernel work that still blocks clean external embedding and release hygiene for the companion modules.

Kernel semantics are no longer the main gap. The remaining work is:

- companion module version alignment
- clean external-consumer validation
- repeatable release ergonomics
- embedder-facing wiring examples
- adapter-facing protocol documentation

## Scope

In scope:

- `go.mod` linkage for `root`, `modules`, `pkg/harness/builtins`, `adapters`, `cmd/harness-core`
- release tests and release helper tooling
- public docs for release/versioning and adapter protocol mapping
- public examples for durable embedding with adapters and interactive control

Out of scope:

- new kernel product semantics
- user / tenant / org / auth platform concepts
- product-specific UI or workflow logic

## Checklist

- [x] Align `pkg/harness/builtins`, `adapters`, and `cmd/harness-core` on the current compatible root/modules/builtins repo-local versions instead of the stale `82b204d` chain.
- [x] Add executable validation for clean external consumers in blank modules without repo-local `replace`.
- [x] Turn companion release linkage into an explicit guardrail and maintainer workflow, not tribal knowledge.
- [x] Fix builtins external compile flow as part of the aligned version graph.
- [x] Add adapter external compile validation as part of the same clean-consumer release gate.
- [x] Add an embedder-facing public example that wires `pkg/harness/postgres`, companion modules/builtins, interactive control, and the WebSocket adapter together.
- [x] Add adapter-facing protocol documentation that maps approvals, blocked runtimes, interactive control, audit replay/events, and compatibility constraints onto the transport surface.
