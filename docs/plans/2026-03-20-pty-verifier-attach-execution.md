# PTY Verifier And Attach Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add PTY-specific verifiers and a module-local attach/detach stream bridge API to `modules/shell` so embedding platforms can verify interactive PTY state and bridge external input/output streams without expanding kernel scope.

**Architecture:** Keep all new behavior in `modules/shell`. PTY verifiers will use the shared `PTYManager` instance wired through `RegisterWithOptions(...)`, and the attach API will bridge external `io.Reader` / `io.Writer` streams to an existing PTY session while remaining independent from kernel lease semantics. The platform reference example will consume these module-local APIs as a reference embedding path.

**Tech Stack:** Go 1.24, `modules/shell`, existing `PTYManager`, verifier registry, in-memory runtime/example wiring, Go tests, Markdown docs.

---

## Execution Rule

Progress must be tracked in this file only:

- Start every pending task as unchecked
- Mark each task with `[x]` immediately after implementation and verification complete
- If a task reveals a prerequisite gap, add a new unchecked task directly below it before continuing
- If any task remains unchecked, the project is not complete

## Scope Guardrails

This plan intentionally stays outside the kernel:

- in scope: PTY-specific verifiers, module-local attach/detach API, example usage, module/docs/tests
- out of scope: `pkg/harness/*` API expansion, WebSocket attach protocol, auth/user/tenant policy, kernel lease changes
- session lease ownership remains a kernel concept; attach/detach remains a module/platform stream-control concept

### Task 1: Lock PTY Verifier Semantics With Failing Tests

**Files:**
- Modify: `modules/shell/module_test.go`
- Modify/Create: `modules/shell/pty_test.go`
- Test: `modules/shell/pty_test.go`
- Test: `modules/shell/module_test.go`

- [x] Add failing tests proving the shell module registers PTY-specific verifier kinds in addition to the existing built-ins.
- [x] Add failing tests for `pty_handle_active`, `pty_stream_contains`, and `pty_exit_code`.
- [x] Run targeted shell-module tests and verify they fail for the expected missing-verifier reasons.

### Task 2: Implement PTY Verifier Checkers

**Files:**
- Create: `modules/shell/pty_verify.go`
- Modify: `modules/shell/module.go`
- Modify: `modules/shell/README.md`
- Test: `modules/shell/pty_test.go`
- Test: `modules/shell/module_test.go`

- [x] Implement PTY-specific checker types that use the shared `PTYManager` to inspect handle activity, stream contents, and final exit code.
- [x] Register the new PTY verifier kinds through `RegisterWithOptions(...)` without changing kernel verifier contracts.
- [x] Document verifier purpose and argument shape in the shell module README.
- [x] Re-run the targeted verifier tests until they pass.

### Task 3: Lock Module-Local Attach Semantics With Failing Tests

**Files:**
- Modify/Create: `modules/shell/pty_test.go`
- Test: `modules/shell/pty_test.go`

- [x] Add failing tests for a module-local attach bridge that can stream external input/output against an existing PTY handle.
- [x] Add failing tests proving `Detach(...)` stops the bridge without closing the underlying PTY session.
- [x] Run targeted attach tests and verify they fail for the expected missing-API reasons.

### Task 4: Implement PTY Attach/Detach Stream Bridge

**Files:**
- Create: `modules/shell/pty_attach.go`
- Modify: `modules/shell/pty_manager.go`
- Modify: `modules/shell/README.md`
- Test: `modules/shell/pty_test.go`

- [x] Implement a module-local attach API that bridges an external `io.Reader` and/or `io.Writer` to a PTY session through the shared `PTYManager`.
- [x] Add `Detach(...)` semantics that stop stream bridging without claiming ownership of kernel session leases and without implicitly closing the PTY.
- [x] Keep the API reusable by embedding platforms and examples without introducing transport-specific framing.
- [x] Re-run the targeted attach tests until they pass.

### Task 5: Update Platform Reference Example And Docs

**Files:**
- Modify: `examples/platform-reference/main.go`
- Modify: `examples/platform-reference/main_test.go`
- Modify: `examples/platform-reference/README.md`
- Modify: `docs/MODULES.md`
- Modify: `docs/EXTENSIBILITY.md`
- Modify: `docs/API.md`
- Modify: `docs/API.zh-CN.md`
- Modify: `docs/plans/2026-03-20-pty-verifier-attach-execution.md`

- [x] Update the platform reference example to demonstrate attach-based PTY I/O instead of only manual read/write polling.
- [x] Add or update example assertions so attach output, detach semantics, and PTY verifier usage are covered.
- [x] Sync docs so PTY verifier and attach capabilities are described as module-layer concerns, not kernel contracts.
- [x] Re-run the example test until it passes.

### Task 6: Verification And Closeout

**Files:**
- Modify: `docs/plans/2026-03-20-pty-verifier-attach-execution.md`

- [x] Run focused verification for `modules/shell` and `examples/platform-reference`.
- [x] Run `go test ./...` after targeted suites are green.
- [x] Mark completed plan items `[x]` only after fresh verification.
- [x] Re-read this plan file and ensure no unchecked item remains.
