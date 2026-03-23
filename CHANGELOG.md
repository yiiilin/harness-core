# CHANGELOG.md

## Unreleased

## v1.0.1 - 2026-03-23

### Added
- Postgres-backed runtime bootstrap and configuration via `HARNESS_STORAGE_MODE` / `HARNESS_POSTGRES_DSN`
- Postgres-backed WebSocket happy-path and deny-path integration coverage
- Durable restart-read integration coverage across fresh runtime instances
- Runtime planner helper `CreatePlanFromPlanner(...)`
- Planner/context runnable examples in `examples/planner-context` and `examples/planner-replan`
- Event ordering assertions and stable runtime-emitted `event_id` values
- Shell executor truncation, cwd allowlist support, and stable timeout/start/exit error codes
- Timeout-path and event-volume benchmark coverage
- Runner-aware runtime reads so public APIs and internal execution views observe runner-committed state
- Durable runtime budget anchoring via `session.runtime_started_at`
- Control-plane audit coverage for session-task attach, lease claim/renew/release, recovery state changes, and runtime-handle lifecycle mutations

### Changed
- Multi-step plans now keep the session in `plan` while unfinished steps remain
- `runtime.info` now reflects the configured storage mode instead of always reporting in-memory
- Worker lease renewal now cancels blocked renew calls promptly when a run ends or the worker context stops

### Docs
- Expanded `docs/ROADMAP.md` with execution waves, completion criteria, and verification guidance
- Updated `README.md`, `docs/API.md`, `docs/PLANNER_CONTEXT.md`, `docs/STATUS.md`, `docs/EVAL.md`, and `docs/EVENTS.md`
- Added `VERSIONING.md`
- Added embedding/runtime consistency guidance and the kernel hardening execution checklist
