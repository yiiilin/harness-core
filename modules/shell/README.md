# modules/shell

This package is the reference capability module for shell execution.

It demonstrates the intended module shape for future capability packs:
- tool registration
- verifier registration
- default policy hints
- extension hooks for alternative shell backends
- extension hooks for sandbox / command gating integrations
- self-contained tests

Current contents:
- `shell.exec` tool registration
- built-in verifiers used by shell tasks
- PTY-specific verifiers for interactive handle/stream/exit checks
- recommended default policy rules
- extensibility hooks:
  - `Backend`
  - `PTYBackend`
  - `PTYInspector`
  - `PTYManager`
  - `SandboxHook`

## Modes

`shell.exec` currently supports two modes:

- `pipe`
  - one-shot command execution
  - inline `stdout` / `stderr` previews are byte-budgeted head-tail middle elisions when truncation is needed
  - preview metadata keeps `has_more`; `next_offset` is exposed only when the preview remains a contiguous prefix fallback
  - recommended default policy: allow
- `pty`
  - interactive PTY-backed process startup
  - returns a runtime handle plus shell-specific stream metadata
  - recommended default policy: ask

Important distinction:

- `pipe` preview truncation is a preview-only rendering contract
- `ReadArtifact(...)` rereads the durable raw payload through exact offset/line windows
- PTY `Read(...)` / `ViewInteractive(...)` style flows remain exact-window reads over the interactive buffer
- `next_offset`, when present, tells consumers where the raw reread stream continues; head-tail previews intentionally omit it because the preview text is not a continuous prefix window

## PTY Control Surface

PTY behavior stays in the module layer.
The kernel only sees the generic runtime handle persisted from the tool result.

Use a shared `PTYManager` instance when you need both:

- the shell module to start PTY-backed sessions through `shell.exec`
- an embedding platform or example to read, write, resize, or close those sessions

Typical wiring:

```go
manager := shellmodule.NewPTYManager(shellmodule.PTYManagerOptions{})
shellmodule.RegisterWithOptions(tools, verifiers, shellmodule.Options{
	PTYManager: manager,
})
```

For an embedder-owned or remote PTY executor, provide `PTYBackend` directly:

```go
shellmodule.RegisterWithOptions(tools, verifiers, shellmodule.Options{
	PTYBackend: remotePTYBackend,
})
```

That path keeps PTY execution pluggable without forcing a local `PTYManager`.

If the embedder also needs `pty_*` verifier support without a local manager, it can provide `PTYInspector` directly:

```go
shellmodule.RegisterWithOptions(tools, verifiers, shellmodule.Options{
	PTYBackend:   remotePTYBackend,
	PTYInspector: remotePTYInspector,
})
```

Then the same `manager` can be used outside the kernel for:

- `Start(...)` indirectly via `shell.exec`
- `Read(...)`
- `Write(...)`
- `Resize(...)`
- `Attach(...)`
- `Detach(...)`
- `Close(...)`
- `CloseAll(...)`

## PTY Verifiers

When the shell module is registered with a local `PTYManager`, it adds three PTY-specific verifier kinds:

- `pty_handle_active`
  - succeeds when the PTY handle from the action result is still active
- `pty_stream_contains`
  - polls the PTY stream for a target substring
  - args:
    - `text`
    - optional `timeout_ms`
    - optional `offset`
    - optional `max_bytes`
- `pty_exit_code`
  - waits for PTY process exit and checks allowed exit codes
  - args:
    - `allowed`
    - optional `timeout_ms`

These verifiers remain module-level behavior.
They use the shell module's shared `PTYManager`, not new kernel verifier contracts.

Important:

- base shell verifiers such as `exit_code` and `output_contains` are always registered
- PTY-specific verifiers are registered only when PTY inspection is available
- local `PTYManager` is one way to provide that inspection surface
- supplying only `PTYBackend` does not imply stream inspection support
- `pty_handle_active` uses the verifier call `context.Context` instead of forcing a detached background context
- verifier handle resolution accepts:
  - `shell_stream.handle_id`
  - `runtime_handle.handle_id`
  - the first entry in `runtime_handles`
- `pty_stream_contains` starts from `shell_stream.next_offset` when that metadata is present, unless an explicit verifier `offset` overrides it

## Attach / Detach

`Attach(...)` is a module-local stream bridge for embedding platforms.

It can attach:

- an external `io.Reader` as PTY input
- an external `io.Writer` as PTY output

`Detach(...)` stops the bridge without closing the underlying PTY session.
That boundary is intentional:

- `Detach(...)` is about external stream wiring
- `Close(...)` is about PTY process lifecycle
- kernel lease ownership is separate from both

Important boundary:

- session lease ownership is a kernel coordination concept
- PTY read/write ownership is a module/platform concern

Releasing a session lease does not automatically grant or revoke PTY I/O rights.
A platform that wants stricter PTY ownership rules should enforce them outside `pkg/harness/*`.
