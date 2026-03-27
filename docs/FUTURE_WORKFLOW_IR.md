# FUTURE_WORKFLOW_IR.md

## Purpose

Define the mandatory design gate for any future first-class workflow IR in
`harness-core`.

This document exists so maintainers do not accidentally grow the current
`session + plan + step` kernel into a graph engine by scattering `Scope`,
`Edge`, `AppendPatch`, or descendant-mutation concepts across unrelated PRs.

Until this design track is completed and explicitly approved, requests in this
area stay deferred.

## Current Boundary

Today the kernel executes through durable session, plan, step, attempt, action,
verification, artifact, approval, blocked-runtime, and runtime-handle facts.

Important constraints of the current architecture:

- `Program` is accepted as an execution shape, but it lowers into flat
  `plan.StepSpec` records before execution.
- recovery and replay reconstruct execution truth from step-oriented runtime
  facts rather than from graph-native mutation history.
- approval, blocked-runtime, and interactive-handle semantics are kernel-owned
  today, but they are still attached to the current step/runtime loop.
- there is no durable store for first-class graph entities such as scopes,
  edges, patches, or graph-native parent/child ownership.

That means a real workflow IR is a separate kernel design problem, not a small
extension to current `Program` execution.

## Why A Separate Design Doc Is Required

Adding graph-native types without deciding their store, scheduler, recovery, and
replay contract would create two competing sources of execution truth:

1. the current step-oriented kernel facts
2. a partially defined graph layer with unclear durability semantics

That split would make approval continuation, restart recovery, replay, and
interactive-handle ownership less reliable, not more.

This document therefore blocks implementation of:

- first-class `ExecutionGraph`
- first-class `Scope`
- first-class `Edge`
- append-patch or descendant-mutation APIs
- nested `primitive` / `loop` / `graph` runtime ownership

unless the design questions below are answered first.

## Design Goals

If `harness-core` adopts a future workflow IR, it must:

- define one kernel-owned source of truth for workflow structure
- define durable graph identity and ownership semantics
- define how scheduling, approval, recovery, and replay operate over that
  structure
- preserve transport neutrality and keep product workflow semantics outside the
  kernel
- provide a migration path from the current `Program` runtime instead of
  creating an unrelated second system

## Non-Goals

This future design must not absorb product responsibilities such as:

- approval UI, escalation UX, or notification flows
- conversation state, titles, or frontend hydration
- tenant, auth, RBAC, or ownership semantics
- Nidus-specific workflow-mode routing or local graph store shapes

## Required Design Sections Before Implementation

The following sections are the minimum design gate. Later checklist items in the
Nidus workflow-runtime assessment fill these in.

### 1. Architecture Placement

Required question:

- does the future workflow IR replace `Plan` execution, lower into `Plan`, or
  run beside it?

Decision:

- the future workflow IR should run beside the current `Plan` runtime as a
  separate durable execution model

Why this is the right cut:

- lowering a graph IR into flat `Plan` steps would throw away the very graph
  entities the new design is supposed to make durable, such as scopes, edges,
  append patches, and graph-native ownership
- replacing `Plan` outright would force an avoidable migration of stable
  current-kernel behavior before the graph runtime has proven parity on
  recovery, approval, replay, and interactive-handle semantics
- a side-by-side model lets maintainers keep the current linear kernel stable
  while designing a graph-native store and scheduler with its own recovery
  contract

Implications:

- `Plan` remains the supported linear execution model for the current kernel
  wave and for simple durable execution
- the future workflow IR owns its own graph store, scheduler, mutation
  validation, and replay model instead of compiling away those concepts
- both runtimes may share lower-level execution primitives such as
  attempts/actions/verifications/artifacts/runtime handles, but they do not
  share the same structural source of truth
- any future convergence of `Plan` and graph execution should happen only after
  the graph runtime proves correctness and a separate migration plan is
  approved

### 2. Mutation / Failure / Replay Contract

Required questions:

- what mutations are legal, and at what ownership boundary?
- how do failure and cancellation propagate through scopes?
- what durable replay facts are primary, and how is restart recovery made
  deterministic?

Decision:

- a future graph append API must ship with explicit mutation-safety rules,
  scope-local failure propagation rules, and replay guarantees as part of the
  kernel contract

#### Mutation-Safety Rules

- graph patches are additive and descendant-scoped by default
- a patch may create only new scopes, steps, and edges that are owned by the
  caller's current scope or one of its descendants
- a patch may not rewrite or delete ancestors, siblings, or already-committed
  graph structure outside that owned subtree
- id reuse is forbidden once a scope, step, or edge id has been committed
- structural validation must happen before visibility:
  - parent scope must exist
  - ownership must match the caller scope
  - new edges must remain acyclic
  - referenced nodes must exist in the committed graph or in the same patch
  - patch application must be atomic: after restart, the patch is either fully
    visible or not visible at all
- scheduler-owned lifecycle fields such as running/completed/failed/cancelled
  are not user-patchable through append APIs

#### Failure-Propagation Rules

- every scope must have an explicit failure policy, with `fail_parent` as the
  default for safety and parity with the current fail-closed kernel posture
- the minimum semantic set is equivalent to:
  - `continue`: the failed node stays terminal, but the enclosing scope may keep
    running independent siblings
  - `cancel_scope`: cancel runnable descendants in the same scope, but do not
    implicitly cancel ancestors or unrelated scopes
  - `cancel_run`: terminalize the whole workflow run
  - `fail_parent`: mark the parent scope failed and let that failure continue
    upward through the same rules
  - `join_partial`: allow the parent scope to complete with an explicit partial
    result summary instead of treating one failed child as a full-scope failure
- cancellation must be precise rather than global by accident:
  - cancelling one scope must not cancel siblings unless the parent policy says
    so
  - partial-failure semantics must be visible in durable state, not inferred
    later from missing children

#### Replay And Recovery Guarantees

- committed graph structure is a primary durable fact, not an in-memory
  scheduler reconstruction
- every structural mutation and every execution-state transition must have a
  stable per-run id plus stable ordering information
- restart recovery must rebuild the same graph identity, parent/child
  ownership, and edge topology before resuming runnable work
- replay must never depend on product glue to infer:
  - which patch created a node
  - which scope owned a node
  - which failure or cancellation policy applied
- repeated recovery must be idempotent:
  - no duplicate sibling execution
  - no duplicate patch application
  - no duplicate approval replay
  - no duplicate runtime-handle close or invalidation
- graph projections exposed to embedders must be sufficient to hydrate the
  current run state after restart without reinterpreting raw internal metadata

### 3. Migration From Current Program Execution

Required question:

- what concrete migration path moves a current `Program`-based execution flow
  into the future graph model without duplicating runtime semantics?

Decision:

- the first migration target must be the existing `Program` subset, not an
  unrelated external graph DSL

Concrete migration story:

1. keep today's public `Program` contract and current `RunProgram(...)` entry
   point stable while the future graph runtime is incubated beside `Plan`
2. introduce a narrow `Program -> workflow IR` translator for the subset that is
   already semantically real today:
   - one root scope for the program run
   - one primitive graph node per `ProgramNode`
   - direct translation of declared dependencies into graph edges
   - direct translation of current input bindings into graph-native data edges
   - explicit target fan-out lowered into graph-native child scopes or
     target-scoped child nodes instead of flat plan-step expansion
3. keep leaf execution semantics shared with the current kernel:
   - primitive graph nodes still execute through the same action / verify /
     approval / runtime-handle machinery that exists today
   - attempts, actions, verifications, artifacts, approvals, blocked runtimes,
     and runtime handles remain the durable leaf facts
4. route only eligible programs through the graph runtime at first:
   - start with acyclic `Program` graphs without descendant append
   - require parity on recovery, replay, approval continuation, fan-out, and
     interactive-handle identity before broadening coverage
   - keep a defined fallback to the current `Plan`-backed `Program` runtime for
     unsupported cases during the migration window
5. only after the translated `Program` subset is release-grade should the
   kernel consider graph-only features such as descendant append, nested
   subgraphs, or richer scope-native failure policies

Why this migration story matters:

- it proves the future graph runtime against a real workload the kernel already
  supports
- it avoids inventing a second set of approval, replay, and interactive-handle
  semantics just for the migration
- it gives maintainers a measurable parity gate before any broader graph API is
  exposed

## Review Rule

Any proposal that introduces workflow-IR concepts in kernel packages must point
to this document and show that the relevant pending section has been completed.

If it cannot do that, the change should be treated as out of scope for the
current wave.
