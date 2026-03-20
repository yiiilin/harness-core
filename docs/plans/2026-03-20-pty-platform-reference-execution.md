# PTY Shell And Platform Reference Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a module-scoped PTY / streaming shell capability that reuses kernel runtime-handle semantics, and ship a minimal platform reference example under `examples/` that demonstrates how an embedding layer should drive claim / lease execution and interactive handle control without polluting the kernel.

**Architecture:** Keep PTY behavior in `modules/shell` behind the existing shell backend hook instead of adding PTY-specific concepts to `pkg/harness/runtime`. The shell module will start and manage PTY sessions, return runtime handles plus stream metadata through normal action results, and expose module-local control methods for read / write / resize / close. The platform reference stays outside the kernel in `examples/` and shows a small worker loop that claims sessions, renews leases, runs claimed work, and consumes the shell module's PTY control surface.

**Tech Stack:** Go 1.24, `modules/shell`, runtime handles and artifacts already in `pkg/harness/runtime`, in-memory runtime for the example, optional PTY helper dependency if needed, Go tests, Markdown docs.

---

## Execution Rule

Progress must be tracked in this file only:

- Start every pending task as unchecked
- Mark each task with `[x]` immediately after implementation and verification complete
- If a task reveals a prerequisite gap, add a new unchecked task directly below it before continuing
- If any task remains unchecked, the project is not complete

## Scope Guardrails

This plan intentionally keeps the kernel small:

- in scope: shell module PTY capability, module-local stream/control APIs, runtime-handle integration, example worker loop, docs/tests
- out of scope: user/tenant/auth, queue topology, fleet membership, transport streaming protocol, browser-specific UX
- the example may orchestrate workers, but worker identity and deployment policy must stay outside `pkg/harness/*`

### Task 1: Lock PTY Capability Semantics With Tests

**Files:**
- Create: `modules/shell/pty_test.go`
- Modify: `modules/shell/module_test.go`
- Test: `modules/shell/pty_test.go`
- Test: `modules/shell/module_test.go`

- [x] Add failing tests for `shell.exec` PTY mode that prove the handler returns a runtime handle and shell-specific stream metadata instead of only pipe output.
- [x] Add failing tests for PTY control operations covering write/read and close behavior through a module-local manager.
- [x] Run targeted shell-module tests and verify they fail for the expected missing-PTY reasons.

### Task 2: Implement PTY Backend And Manager In The Shell Module

**Files:**
- Create: `modules/shell/pty.go`
- Create: `modules/shell/pty_manager.go`
- Modify: `modules/shell/module.go`
- Modify: `modules/shell/README.md`
- Modify: `go.mod`
- Modify: `go.sum`
- Test: `modules/shell/pty_test.go`
- Test: `modules/shell/module_test.go`

- [x] Add a shell-module PTY manager that can start, read, write, resize, and close PTY-backed sessions without introducing PTY concepts into kernel contracts.
- [x] Update `shell.exec` registration so `mode=pty` uses the PTY backend and returns runtime-handle data plus shell-specific stream metadata.
- [x] Keep the default policy shape explicit: `mode=pipe` remains allow, PTY/interactive modes stay ask unless a caller overrides policy.
- [x] Document the shell module's PTY behavior, control surface, shared-manager wiring, and runtime-handle expectations in `modules/shell/README.md`.
- [x] Re-run targeted shell-module tests until they pass.

### Task 3: Add Minimal Platform Reference Example Under `examples/`

**Files:**
- Create: `examples/platform-reference/main.go`
- Create: `examples/platform-reference/README.md`
- Create: `examples/platform-reference/main_test.go`
- Modify: `README.md`
- Test: `examples/platform-reference/main_test.go`

- [x] Add a failing example test that proves a worker loop can claim a runnable session, renew its lease, execute claimed work, and surface PTY handle information without touching kernel internals.
- [x] Extend the example acceptance criteria so lease cleanup uses `ReleaseSessionLease` and PTY shutdown maps to `CloseRuntimeHandle` or `InvalidateRuntimeHandle` on the runtime service.
- [x] Run the example test and verify it fails for the expected missing-reference-implementation reasons.
- [x] Implement a minimal platform reference under `examples/platform-reference` that wires the shell module and the example control plane to the same PTY manager instance, launches a worker goroutine around `ClaimRunnableSession` / `RunClaimedSession`, renews and releases leases, and demonstrates basic PTY interaction.
- [x] Document how the example maps platform concerns to kernel APIs and shell-module control APIs, including that PTY I/O ownership is separate from session lease ownership.
- [x] Re-run the example test until it passes.

### Task 4: Full Verification And Docs Sync

**Files:**
- Modify: `docs/STATUS.md`
- Modify: `docs/ROADMAP.md`
- Modify: `docs/API.md`
- Modify: `docs/API.zh-CN.md`
- Modify: `docs/MODULES.md`
- Modify: `docs/EXTENSIBILITY.md`
- Modify: `docs/plans/2026-03-20-pty-platform-reference-execution.md`

- [x] Update status/API docs at the right level and put PTY-specific details primarily in module/extensibility docs so the kernel API surface does not absorb module-local control semantics.
- [x] Run focused verification for shell module and platform example.
- [x] Run `go test ./...` after the targeted suites are green.
- [x] Mark completed plan items `[x]` only after fresh verification.
- [x] Re-read this plan file and ensure no unchecked item remains.
