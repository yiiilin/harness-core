# EVAL.md

## Goal

Document the current evaluation strategy for `harness-core`.

This project currently uses three complementary validation layers:

1. **unit / integration tests**
   - assert correctness of core behavior
2. **path coverage tests**
   - cover happy path and failure-path transitions
3. **benchmarks**
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
2. add table-driven transition correctness tests
3. add golden-output tests for protocol responses
4. expose richer runtime metrics hooks beyond the in-memory recorder

---

## Philosophy

`harness-core` should evolve by keeping two promises:
- correctness first
- overhead visible

That means every new major primitive should ideally arrive with:
- at least one correctness test
- at least one path test if it changes control flow
- at least one benchmark if it affects hot runtime paths
