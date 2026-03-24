# Embedder VNext Adaptation Spec

## Goal

Define how `harness-core` should absorb the latest embedder-facing requests without expanding kernel scope into platform concerns.

This spec separates:

- true kernel-next execution-model work
- public helper / packaging / documentation work
- explicit non-goals that must stay outside the kernel

## Approved Direction

`harness-core` should evolve by **preserving the current single-step runtime path and adding a transport-neutral execution-program layer beside it**, not by forcing product-specific behavior into `StepSpec` or rewriting the runtime around one product's needs.

That means:

- current `StepSpec -> action -> optional verify` remains valid
- a future execution-program layer can express fan-out, dataflow, artifacts, blocked runtime boundaries, and richer verification scopes
- current public APIs stay usable while new kernel-native execution features are added incrementally

## Terminology

The following terms are accepted for kernel/public use:

- `execution target`: an embedder-supplied executable target. It is transport-neutral and product-neutral.
- `target-scoped action`: an action execution record tied to one execution target inside a broader logical execution.
- `blocked runtime`: a runtime paused on an external condition such as approval, confirmation, or another external readiness signal.

The following must remain outside the kernel:

- product run IDs
- tenant / user / org identity
- approval TTL policy
- second-factor confirmation UX
- target discovery strategy
- platform-specific planner / tool-loop state

## Current State vs Requested State

### Already present in current kernel

- durable session / task / plan / step lifecycle
- governed runtime loop with policy, approval, execution, verification, and recovery
- durable approvals with resume / reopen after restart
- durable attempts / actions / verifications / artifacts / runtime handles
- PTY backend / inspector extension points in `modules/shell`
- replay/debug projection helpers around execution cycles and audit events

### Missing from current kernel

- native multi-target fan-out scheduling inside one runtime
- native preplanned non-shell tool graph execution
- stable step-to-step dataflow via structured output refs / artifact refs
- first-class blocked-runtime model beyond approval-shaped pause/resume
- unified verification scopes across graph, fan-out, and interactive execution
- capability matcher reason codes for feature-level unsupported decisions
- kernel-native artifact / attachment input model
- richer projection model for target slices, blocked pauses, and interactive reopen state

## Scope Decision

### In scope now

These are safe to implement immediately without redefining the kernel runtime model:

1. terminology and boundary docs for the new execution-model vocabulary
2. explicit current support matrix for embedders
3. public capability matching / unsupported reason codes
4. companion-module publish hygiene so external `go mod tidy` does not depend on repo-local `replace`

Initial unsupported reason-code set for Wave 1:

- `CAPABILITY_NOT_FOUND`
- `CAPABILITY_DISABLED`
- `CAPABILITY_VERSION_NOT_FOUND`
- `CAPABILITY_VIEW_NOT_FOUND`
- `CAPABILITY_VIEW_DRIFT`
- `MULTI_TARGET_FANOUT_UNSUPPORTED`
- `PREPLANNED_TOOL_GRAPH_UNSUPPORTED`
- `INTERACTIVE_REOPEN_UNSUPPORTED`
- `ARTIFACT_INPUT_UNSUPPORTED`

### In scope next

These are valid kernel-next items, but require new core model types and durable semantics:

1. execution targets and target-scoped action records
2. preplanned execution-program / tool-graph nodes
3. stable output / artifact / attachment references between steps
4. blocked-runtime records and reopen / resume APIs beyond approval-only pause
5. unified verification scopes and aggregate verification
6. expanded replay / projection views over the richer execution model

### Out of scope

These remain platform concerns even for a multi-user multi-session agent platform:

- `tenant_id`, `user_id`, `org_id`
- auth, gateway tokens, session login state
- approval UI or escalation workflow
- platform projections, inboxes, dashboards, search
- queue topology, fleet deployment, or worker orchestration vendor choices
- model provider routing or prompt strategy

## Execution Strategy

The work should land in four waves.

### Wave 1: public boundary and release hygiene

- publish terminology and support matrix
- expose capability unsupported reason codes
- remove repo-local `replace` directives from companion module release metadata

### Wave 2: public model layer

- add execution-target, artifact-input, and blocked-runtime public contracts
- expose read/query APIs for blocked runtimes and richer projection views

### Wave 3: runtime engine

- implement target fan-out scheduling
- implement preplanned program/tool-graph execution
- add step-to-step dataflow and unified verification scopes

### Wave 4: durable interactive strengthening

- extend interactive reopen/view/write/close semantics
- add richer durable PTY/session projection and verifier resume behavior

## Acceptance Guidance For Wave 1

Wave 1 is complete when:

- embedders can point to one public doc that names the new vocabulary and current support boundary
- the kernel can return stable unsupported reason codes through a public capability-matching API
- companion `go.mod` files no longer ship repo-local `replace` directives
- local multi-module development still works through the committed `go.work`
- release tests guard that module metadata does not regress

## Non-Goals For This Spec

This spec does not claim that Wave 2-4 are already implemented.
It defines the accepted architecture and the first safe slice to ship now.
