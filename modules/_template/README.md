# modules/_template

This directory is a template for adding a new capability module.

A module should bundle together:
- tool definitions
- tool handlers
- verifier definitions (if applicable)
- default policy hints
- tests

A module should **not** own:
- runtime state machine logic
- transport adapters
- product UI concerns
- cross-cutting kernel contracts
