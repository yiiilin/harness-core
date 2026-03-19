# Contributing to harness-core

Thanks for contributing to `harness-core`.

This project is intentionally split into layers:

```text
harness-core (kernel)
  -> modules/* (capability packs)
  -> adapters/* (transport bindings)
  -> examples/* (reference usage)
```

The core rule is simple:

> Keep the kernel small. Add reusable capabilities as modules. Add transport glue as adapters.

---

## What belongs where

### `pkg/harness/*`
Put code here when it defines or stabilizes the runtime kernel itself:
- task/session/plan/action/verify contracts
- runtime state machine
- tool and verifier registries
- policy / approval abstractions
- audit / event / metrics hooks
- public facade / stable constructor patterns

### `modules/*`
Put code here when it represents a reusable capability pack:
- tool definitions
- tool handlers
- verifier definitions
- default policy hints
- tests for that capability

Examples:
- `modules/shell`
- `modules/filesystem`
- `modules/http`

### `adapters/*`
Put code here when it binds the runtime to a transport or delivery surface:
- websocket
- http
- rpc
- queue worker

### `examples/*`
Put code here when it helps demonstrate usage:
- minimal embedded example
- adapter example
- sample client

---

## Contribution rules

1. Prefer improving the kernel only when the abstraction is broadly reusable.
2. Do not add product-specific workflows directly into `pkg/harness`.
3. New capability code should generally start as a module, not as a built-in kernel feature.
4. Every non-trivial addition should come with at least one test.
5. If a change affects runtime hot paths, add or update a benchmark when practical.
6. Keep public API additions deliberate; avoid leaking internal details accidentally.

---

## Required checks

Before opening or merging a change, run:

```bash
go test ./...
go test -bench . -run '^$' ./pkg/harness/runtime
```

If you changed docs, update the relevant docs under `docs/` as well.

---

## Adding a new module

Use `modules/_template` as the starting point.

A new module should usually include:
- `README.md`
- `module.go`
- `module_test.go`

At minimum it should provide:
- tool registration
- verifier registration (if relevant)
- default policy hints
- tests

---

## API stability guidance

When in doubt:
- expose less
- keep kernel contracts stable
- let modules and adapters evolve faster than the core

The preferred public import path is:

```go
import "github.com/yiiilin/harness-core/pkg/harness"
```

Only reach into subpackages when you need lower-level control.
