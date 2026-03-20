# MODULES.md

## Goal

Describe the intended module system for `harness-core`.

`harness-core` should remain a reusable runtime kernel.
Generic capabilities should live in separate capability modules.

That means:
- `harness-core` owns contracts, registries, state machine, policy hooks, audit hooks, and runtime execution flow
- `modules/*` own concrete capability implementations

---

## Why modules exist

If every new capability is implemented directly inside `harness-core`, the core will become:
- larger
- harder to maintain
- less reusable
- more opinionated than necessary

Modules keep the design clean:
- core = execution kernel
- modules = capability packs

---

## What belongs in a module

A capability module should bundle together:

1. **tool definitions**
   - stable tool names
   - capability metadata
   - risk level

2. **tool handlers**
   - actual execution logic for the module's tools

3. **verifier definitions**
   - capability-specific postcondition checks

4. **default policy hints**
   - suggested allow/ask/deny rules for that capability

5. **tests**
   - registration tests
   - happy-path tests
   - verifier tests where applicable

---

## Standard shape

A module should typically look like this:

```text
modules/<name>/
  README.md
  module.go
  module_test.go
```

Later, if a module grows more complex, it can evolve into:

```text
modules/<name>/
  README.md
  module.go
  module_test.go
  verify_*.go
  execute_*.go
  policy.go
```

---

## Required module API shape

Every module should at least expose:

```go
func Register(tools *tool.Registry, verifiers *verify.Registry)
func DefaultPolicyRules() []permission.Rule
```

If a capability needs customization, prefer a second explicit constructor such as:

```go
func RegisterWithOptions(tools *tool.Registry, verifiers *verify.Registry, opts Options)
```

This keeps module wiring consistent across the ecosystem while still allowing controlled extension.

When `builtins.Register()` is used, these `DefaultPolicyRules()` are no longer documentation-only hints.
They are composed into the default built-in policy evaluator path unless the embedding app overrides `opts.Policy`.

---

## Current reference modules

### `modules/shell`
Demonstrates:
- `shell.exec`
- `pipe` and `pty` shell modes
- verifier registration including PTY-specific verifier kinds
- default shell policy hints
- a shared `PTYManager` hook for interactive shell control and attach/detach stream bridging
- runtime-handle production for PTY-backed sessions
- tests

### `modules/filesystem`
Demonstrates:
- `fs.exists`
- `fs.read`
- `fs.write`
- `fs.list`
- filesystem verifiers
- tests

### `modules/http`
Demonstrates:
- `http.fetch`
- `http.post_json`
- HTTP verifiers
- tests

---

## What should *not* go into modules

Modules should not own:
- session state machine logic
- runtime step loop logic
- cross-cutting audit architecture
- generic policy evaluation engine
- transport adapters
- UI concerns

These belong to `harness-core` itself.

---

## Relationship to built-ins

`pkg/harness/builtins.Register()` may reuse modules.

That is the preferred direction.

Instead of duplicating built-in tool definitions inside the runtime package,
the builtins composition helper should wire well-defined modules into the default options.

This ensures:
- one source of truth
- less drift
- easier future extraction into a companion module repository

It also means:
- tool registration stays in the module
- suggested policy rules stay in the module
- policy evaluation stays in core

---

## Future extraction path

If the module ecosystem grows, a natural next step is:

- keep `harness-core` focused on runtime kernel
- move generic modules into a companion repository such as:
  - `harness-modules`

Until then, keeping `modules/*` in the same repo is a practical way to refine the module API.

---

## Design rule of thumb

Use this rule:

> If it is a reusable capability with its own action + verifier + policy + tests,
> it should probably be a module.

Examples:
- shell
- filesystem
- http

Examples that can wait:
- browser/page
- windows-native
- knowledge

---

## Summary

`harness-core` should grow by making the core smaller and the capability packs cleaner.

The intended layering is:

```text
harness-core (kernel)
  -> modules/* (capability packs)
  -> adapters/* (transport bindings)
  -> examples/* (reference usage)
```
