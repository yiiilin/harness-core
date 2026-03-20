# EXTENSIBILITY.md

## Goal

Document how `harness-core` should support extension without turning the core into a grab bag of product-specific features.

This document focuses on:
- extension points in the core
- extension points inside modules
- when to add hooks vs when to add new modules
- how to keep extensibility disciplined

---

## Core principle

`harness-core` should be **extensible by interface and composition**, not by scattering special-case flags across the runtime.

That means:
- the core exposes clean interfaces
- modules may expose optional hooks or replaceable backends
- embedding applications compose the pieces they need

Before adding a new core hook, read `docs/KERNEL_SCOPE.md`.

---

## Extension layers

### 1. Kernel extension points
These belong in `pkg/harness/*` because they affect the execution kernel itself.

Examples:
- `PolicyEvaluator`
- `ContextAssembler`
- `Compactor`
- `Planner`
- `CapabilityResolver`
- `EventSink`
- metrics hook
- storage interfaces

Rule of thumb:
> If a concern changes how the runtime loop behaves globally, it belongs in the core.

Hard constraint:
> If a concern is transport-specific, identity-specific, or product-specific, it does not belong in the core even if it is convenient to add there.

---

### 2. Capability module extension points
These belong in `modules/*` because they affect how one capability family is executed.

Examples:
- shell backend replacement
- shell sandbox hook
- filesystem path policy helper
- HTTP client override

Rule of thumb:
> If a concern changes how one capability is implemented, it belongs in that module.

---

### 3. Adapter extension points
These belong in `adapters/*` because they affect transport or host integration.

Examples:
- WebSocket auth handshake variants
- request/response wrappers
- event streaming behavior
- middleware

Rule of thumb:
> If a concern changes how the runtime is exposed to a transport or host, it belongs in the adapter.

---

## Preferred extension style

### Prefer: typed interfaces
Good:

```go
type Planner interface {
    PlanNext(...)
}
```

### Prefer: explicit options
Good:

```go
import (
    "github.com/yiiilin/harness-core/pkg/harness"
    "github.com/yiiilin/harness-core/pkg/harness/builtins"
)

opts := harness.Options{}
builtins.Register(&opts)
opts.Policy = myPolicy
opts.ContextAssembler = myAssembler
rt := harness.New(opts)
```

### Prefer: module-local hooks
Good:
- shell module exposes `Backend`
- shell module exposes `SandboxHook`
- modules expose `DefaultPolicyRules()` while core keeps the evaluator/composition logic
- a separate builtins composition helper may assemble several modules without making the runtime package own them

### Avoid: global ad-hoc flags
Avoid:
- `EnableMagicPlanner`
- `UseCustomShell2`
- `ExperimentalWindowsModeX`

These tend to rot quickly and make the core harder to reason about.

---

## Example: shell extensibility

The shell capability is a good reference pattern.

### What belongs in core
- shell action/result contracts
- runtime policy hook interface
- runtime step loop

### What belongs in module
- `shell.exec` tool registration
- built-in shell verifiers
- default shell policy hints
- optional shell backend interface
- optional sandbox hook interface

This is why `modules/shell` can expose:
- `Register(...)`
- `RegisterWithOptions(...)`
- `Backend`
- `SandboxHook`
- `DefaultPolicyRules()`

without polluting `harness-core` with shell-specific policy logic.

---

## When to add a new hook

Ask these questions:

1. Is this concern reusable beyond one product?
2. Is it likely to be replaced by embedding applications?
3. Does it avoid forking core code?
4. Can it be expressed as a small interface or option object?

If all are true, a new hook may be justified.

---

## When *not* to add a hook

Do **not** add a hook just because:
- one current application needs one special behavior
- a temporary workaround is convenient
- a product-specific policy is easier to cram into the kernel
- auth, tenant, or UI logic feels "close enough" to runtime state

In these cases, prefer:
- a module-specific option
- an adapter-layer customization
- or a local wrapper around the runtime

---

## Hook design checklist

Before adding an extension point, check:
- [ ] Is the hook typed, not stringly-typed?
- [ ] Is the hook narrowly scoped?
- [ ] Is the default behavior still simple?
- [ ] Is there at least one test for the hook path?
- [ ] Does the hook avoid leaking transport/product concerns into the kernel?
- [ ] Does the hook pass the admission test in `docs/KERNEL_SCOPE.md`?

---

## Summary

Extensibility in `harness-core` should be:
- explicit
- typed
- layered
- testable
- boring

The ideal outcome is:
- core stays small
- modules stay self-contained
- adapters stay transport-specific
- embedding apps can replace pieces without forking the runtime
