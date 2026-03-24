# Platform Reference Example

This example is intentionally outside the kernel and demonstrates platform-side orchestration around `pkg/harness` claim/lease APIs with the shell module PTY control surface.

## What It Demonstrates

- Shared module-layer PTY manager:
  - A single `modules/shell` `PTYManager` instance is created.
  - That same instance is passed into `shellmodule.RegisterWithOptions(...)`.
  - The platform then uses that same manager for PTY read/write/close.
- Claimed worker loop pattern:
  - `ClaimRunnableSession`
  - renew lease while work is in progress (`RenewSessionLease`)
  - `RunClaimedSession`
  - `ReleaseSessionLease`
- Plan content:
  - Creates session/task/plan in-memory.
  - Step invokes `shell.exec` with `mode=pty` and `command=cat`.
- PTY interaction:
  - After claimed execution starts the PTY-backed `cat`, platform uses `Attach(...)` to bridge external input/output streams.
  - The example then calls `Detach(...)` to stop the bridge while leaving the PTY session alive.
  - Direct manager reads confirm the PTY still runs after detach.
- PTY verifier usage:
  - `pty_handle_active`
  - `pty_stream_contains`
- Runtime-handle lifecycle reconciliation:
  - The platform explicitly closes PTY process state via PTY manager.
  - It persists typed interactive runtime observation through `UpdateInteractiveRuntime`.
  - Then it calls `CloseRuntimeHandle` in harness runtime to keep persisted handle state aligned with real process lifecycle.

## Run

```bash
go test ./examples/platform-reference -run TestRunReferenceDemo -count=1
go run ./examples/platform-reference
```

## Expected Output

You should see a short summary similar to:

```text
session: ...
phase: complete
runtime handle: ... (closed)
active verify: true
stream verify: true
attach output: hello from platform reference
attach detached: true
lease released: true
```

The important behaviors are:

- the worker claims and later releases the session lease
- the PTY-backed step creates a persisted runtime handle
- verifier checks succeed against the live PTY session
- detach stops the bridged output while the PTY remains alive
- the runtime handle is explicitly closed after the PTY shuts down
- the typed interactive runtime projection records the final closed observation

## Why `CloseRuntimeHandle` Instead of `InvalidateRuntimeHandle`

This flow is a normal, graceful shutdown requested by the platform after successful PTY interaction. `CloseRuntimeHandle` is the appropriate lifecycle signal for clean completion.

`InvalidateRuntimeHandle` is better reserved for stale/unsafe/unknown handle state (for example crash, orphaning, or invalid recovery path), not intentional close.

## When To Use This Example

Use this as a reference when:

- building a platform worker around claimed execution
- integrating PTY-backed shell execution
- bridging external input/output streams into a runtime-managed PTY
- keeping platform-side process lifecycle aligned with persisted runtime handles
