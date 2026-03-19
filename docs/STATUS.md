# STATUS.md

## Current maturity snapshot

`harness-core` is currently a **pre-1.0 runtime kernel prototype**.

It already has:
- a minimal demo planner example
- a default context assembler
- stable-enough domain objects for `task / session / plan / step`
- a tool registry
- a verifier registry
- a default policy evaluator
- a shell pipe executor
- a `RunStep()` execution loop
- in-memory audit sink
- in-memory metrics recorder
- websocket adapter
- reference modules (`shell`, `filesystem`, `http`)
- integration tests and benchmark baselines

It does **not** yet have:
- production persistence
- multi-process/distributed execution
- complete public API stability guarantees
- advanced planners or context assemblers
- richer shell modes such as PTY execution
- advanced capability packs such as Windows-native or knowledge modules

## Best use today

Use it for:
- studying harness runtime structure
- embedding in prototypes
- refining contracts
- building capability modules
- experimenting with execution / verify / policy patterns

Do not assume it is production-ready infrastructure yet.
