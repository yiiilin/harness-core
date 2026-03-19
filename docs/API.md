# API.md

## Purpose

This document describes the intended public API surface of `harness-core`.

The main entry point should be:

```go
import "github.com/yiiilin/harness-core/pkg/harness"
```

## Recommended public surface

### Top-level constructor path
- `harness.New(opts)`
- `harness.NewDefault()`
- `harness.NewWithBuiltins()`
- `harness.RegisterBuiltins(&opts)`

### Re-exported core types
- task/session/plan/action/verify domain types
- tool definition and risk types
- permission decision/action types
- audit event type
- runtime interfaces: planner/context assembler/event sink

### Lower-level packages
Consumers may import lower-level packages directly when they need finer control, but the default path should begin with `pkg/harness`.

## Stability intent

The project is still early, but this is the intended direction:
- keep the top-level facade small and stable
- let subpackages evolve more freely
- avoid forcing consumers to understand every internal package before getting started
