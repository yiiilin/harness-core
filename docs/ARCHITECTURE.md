# Architecture

## v1 direction

Monolith-first Go runtime with modular internal packages:
- protocol
- server
- runtime
- tool
- verify
- executor
- auth
- audit

## current scaffold

```text
client
  -> websocket server
  -> auth gate
  -> in-memory session runtime
  -> action router
```
