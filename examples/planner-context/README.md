# Planner Context Example

This example focuses only on the `ContextAssembler` side of the kernel.

It does not execute tools. Instead, it builds a layered `ContextPackage` and prints the JSON that a planner could consume.

## What It Demonstrates

- implement a custom `ContextAssembler`
- split context into task, session, constraints, metadata, derived fields, and extras
- expose compact previews for large fields
- keep compaction transport-neutral and planner-friendly

The example intentionally stays simple: it demonstrates the shape of a context package without requiring a full runtime loop.

## Run

```bash
go run ./examples/planner-context
go test ./examples/planner-context -count=1
```

## Expected Output

The program prints a JSON document containing:

- a `task` block
- a `session` block
- raw `constraints` and `metadata`
- derived fields such as `goal_word_count`
- an `extras.compaction` section with truncated previews

## When To Use This Example

Use this as a reference when:

- designing your own `ContextAssembler`
- deciding what information a planner should see directly
- introducing structured previews or compaction hints without committing to a full memory system
