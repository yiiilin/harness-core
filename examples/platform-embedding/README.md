# Platform Embedding Example

This example shows the smallest "existing platform wraps the kernel" flow using only public packages.

It demonstrates:

- an accepted-first API wrapper that returns immediately with a platform-owned `external_run_id`
- a platform-side mapping from `external_run_id -> session_id` instead of pushing external IDs into kernel types
- background execution through `pkg/harness/worker`
- a named worker helper instance that can be wrapped with platform-side observability
- external approval UI behavior through `ListApprovals(...)` and `RespondApproval(...)`
- remote PTY execution through `shellmodule.Options{PTYBackend: ...}`
- replay/debug projection through `pkg/harness/replay`
- the absence of `pty_*` verifiers when no local `PTYManager` is configured

## Run

```bash
go run ./examples/platform-embedding
```

Expected output shape:

```text
accepted run: run-ext-123 -> sess_...
approval pending: appr_...
remote PTY calls: before=0 after=1
runtime handle: remote-pty-1
projection cycles: 1
```

## What The Example Actually Does

1. The platform accepts an external request and creates kernel `session/task/plan` records.
2. The request stores `external_run_id` in platform-owned mapping/state, while the kernel still uses `session_id`.
3. A `worker.Worker` claims runnable work and hits the PTY approval gate before any remote execution happens.
4. The platform inspects pending approvals and simulates an external approval UI reply with `RespondApproval(...)`.
5. The worker runs again, resumes the approved pending step, and the remote PTY backend returns a runtime handle.
6. The example reads replay/debug state back through the public replay helper.

## Why This Example Matters

Use this example when you already have:

- your own API gateway
- your own approval UX
- your own run/request IDs
- your own remote executor or interactive runtime

and you want to embed `harness-core` as a kernel rather than adopting a reference adapter wholesale.

For durable restart/recovery across processes, pair the same pattern with `pkg/harness/postgres` and keep the worker helper running after restart.
