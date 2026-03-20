# STATUS.md

## Current maturity snapshot

`harness-core` is currently a **pre-1.0 runtime kernel prototype**.

It already has:
- persistence abstractions (`RepositorySet`, `UnitOfWork` / `Runner`)
- Postgres-backed repositories and transaction wiring
- Postgres-backed WebSocket happy/deny integration coverage
- restart-read durability coverage
- a minimal demo planner example
- planner-driven plan creation helpers
- a default context assembler
- stable-enough domain objects for `task / session / plan / step`
- a tool registry
- a verifier registry
- a default policy evaluator
- a shell pipe executor
- shell output truncation and cwd/path allowlist support
- a `RunStep()` execution loop
- in-memory audit sink
- in-memory metrics recorder
- stable runtime-emitted event ids
- websocket adapter
- planner/context examples
- reference modules (`shell`, `filesystem`, `http`)
- integration tests and benchmark baselines

It does **not** yet have:
- multi-process/distributed execution
- complete public API stability guarantees
- richer shell modes such as PTY execution
- advanced capability packs such as Windows-native or knowledge modules

## Best use today

Use it for:
- studying harness runtime structure
- embedding in prototypes and controlled project integrations
- refining contracts
- building capability modules
- experimenting with execution / verify / policy patterns

Do not assume it is production-ready infrastructure yet.

## Current execution plan

The active execution plan for the repository is tracked in `docs/ROADMAP.md`.

Current instruction set:
- complete P0
- complete P1
- complete advanced planner/context work
- defer P2 for now
