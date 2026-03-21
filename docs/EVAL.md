# EVAL.md

## Goal

Document the current evaluation strategy for `harness-core`.

This project currently uses three complementary validation layers:

1. **unit / integration tests**
   - assert correctness of core behavior
2. **workflow evals**
   - assert that public embedding surfaces still compose into expected end-to-end flows
3. **path coverage tests**
   - cover happy path and failure-path transitions
4. **benchmarks**
   - provide baseline latency/overhead data for the kernel loop

---

## Current correctness coverage

### WebSocket integration
- `TestWebSocketHappyPath`
- `TestWebSocketStepRunHappyPath`

### Runtime integration
- `TestHappyPathRunStep`
- `TestRunStepPolicyDenied`
- `TestTaskSessionPlanWiring`

These verify:
- task/session/plan relationships
- successful step execution
- denied step execution
- plan completion behavior
- task/session state changes
- audit event emission

### Public workflow evals
- `TestWorkflowEvalApprovalResumeReplay`
- `TestWorkflowEvalRecoverableClaimRun`
- `TestWorkflowEvalRunLoopStopsAfterHandlingWork`
- `TestWorkflowEvalConcreteShellScenarios`

These verify, through public embedding surfaces only:
- approval-paused worker flow
- approval response and resumed execution
- replay/debug projection after execution
- claim -> interrupt -> recover workflow
- worker outer-loop handling through `RunLoop(...)`
- concrete shell-backed planner / approval / recovery scenarios

The workflow evals live under:

- `./evals`

### Release gate tests

The repository now also has a dedicated release-oriented test package:

- `./release`

These tests are intentionally narrower than `go test ./...`.
They protect the public `v1` story rather than every internal invariant.

Current release-gate coverage includes:

- Tier 1 public entrypoint presence and shape
- in-memory approval -> resume -> replay flow through the stable embedding path
- durable Postgres restart plus resumed approval flow through the stable embedding path
- upgrade from the previous schema version to the latest schema version

### Human-readable workflow walkthroughs

For visible scenario output rather than test assertions, run:

- `go run ./examples/workflow-scenarios`

This example uses the real built-in shell module and prints:

- planner-derived execution output
- approval pause -> respond -> resume behavior
- claim interruption -> recovery behavior
- persisted execution-fact counts and replay counts

---

## Current benchmarks

### `BenchmarkRunStepHappyPath`
Measures a minimal successful step:
- policy allow
- shell pipe execution
- two verifier checks
- transition to complete

### `BenchmarkRunStepPolicyDenied`
Measures a denied path:
- policy deny
- no action execution
- immediate fail-safe transition

### `BenchmarkRunStepVerifyFailure`
Measures action success but verification failure:
- action executes
- verifier fails
- transition to recover/failed path

### `BenchmarkRunStepActionFailure`
Measures tool execution failure before a successful verification path:
- action invoked
- tool returns failure status
- runtime still produces a structured failure path

### `BenchmarkRunStepTimeoutPath`
Measures executor timeout handling:
- action starts
- shell execution exceeds timeout budget
- runtime records a structured timeout failure path

### `BenchmarkEmitEventsBatch100`
Measures runtime event emission overhead without tool execution:
- a fixed batch of 100 structured events
- audit sink emission only
- useful for spotting event-pipeline regressions separately from executor cost

---

## Recommended command set

### Run all tests

```bash
go test ./...
```

### Run workflow evals only

```bash
go test ./evals -count=1
```

### Run the release gate only

```bash
go test ./release -count=1
```

Or through `make`:

```bash
make test-release
```

### Run the current release-check matrix

```bash
make release-check
```

Current release-check matrix:

- `./release`
  - Tier 1 API compatibility and durable upgrade/restart gates
- `./evals`
  - public workflow composition gates

### Run the concrete workflow walkthrough

```bash
go test ./examples/workflow-scenarios -count=1
go run ./examples/workflow-scenarios
```

### Run runtime benchmarks

```bash
go test -bench . -benchmem ./pkg/harness/runtime
```

### Compare only one benchmark family

```bash
go test -run '^$' -bench RunStep -benchmem ./pkg/harness/runtime
```

### Run the timeout-path benchmark only

```bash
go test -run '^$' -bench RunStepTimeoutPath -benchmem ./pkg/harness/runtime
```

Example output shape:

```text
BenchmarkRunStepTimeoutPath-8            1200           950000 ns/op          18000 B/op         220 allocs/op
```

### Run the event-volume benchmark only

```bash
go test -run '^$' -bench EmitEventsBatch100 -benchmem ./pkg/harness/runtime
```

Example output shape:

```text
BenchmarkEmitEventsBatch100-8           50000            25000 ns/op          12000 B/op         105 allocs/op
```

---

## What to watch over time

When evolving the runtime, track:
- happy-path latency
- verify-failure overhead
- deny-path overhead
- event emission overhead
- memory allocations (`-benchmem`)
- step-run counters and outcome ratios (via runtime metrics snapshot)

A useful rule of thumb:
- deny path should stay the cheapest
- verify-failure should be more expensive than deny, but cheaper than a full successful execution with heavy actions
- happy-path cost should not regress unexpectedly when adding abstractions

---

## Near-term eval backlog

1. add websocket end-to-end benchmark
2. expand workflow eval cases across adapters and durable restart scenarios beyond the current release matrix
3. add golden-output tests for protocol responses
4. expose richer runtime metrics hooks beyond the in-memory recorder

---

## Philosophy

`harness-core` should evolve by keeping two promises:
- correctness first
- overhead visible

That means every new major primitive should ideally arrive with:
- at least one correctness test
- at least one workflow eval if it changes a public composition path
- at least one path test if it changes control flow
- at least one benchmark if it affects hot runtime paths
