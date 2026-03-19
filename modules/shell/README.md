# modules/shell

This package is the reference capability module for shell execution.

It demonstrates the intended module shape for future capability packs:
- tool registration
- verifier registration
- default policy hints
- extension hooks for alternative shell backends
- extension hooks for sandbox / command gating integrations
- self-contained tests

Current contents:
- `shell.exec` tool registration
- built-in verifiers used by shell tasks
- recommended default policy rules
- extensibility hooks:
  - `Backend`
  - `SandboxHook`
