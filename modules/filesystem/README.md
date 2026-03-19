# modules/filesystem

This package is the reference capability module for common filesystem operations.

Current scope:
- `fs.exists`
- `fs.read`
- `fs.write`
- `fs.list`
- verifier helpers:
  - `file_exists`
  - `file_content_contains`

The goal is to demonstrate how a capability module should bundle:
- tool definitions
- handlers
- verifier registrations
- default policy hints
- tests
