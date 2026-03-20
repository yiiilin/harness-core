# adapters/http

This package is a minimal HTTP/JSON reference adapter for `harness-core`.

It is intentionally thin:

- `GET /healthz`
- `GET /runtime/info`
- `POST /sessions`
- `POST /tasks`
- `POST /sessions/{session_id}/attach-task`
- `POST /plans`
- `POST /sessions/{session_id}/steps/run`
- `POST /sessions/claim/runnable`
- `POST /sessions/claim/recoverable`
- `POST /sessions/{session_id}/lease/renew`
- `POST /sessions/{session_id}/lease/release`
- `POST /sessions/{session_id}/run-claimed`
- `POST /sessions/{session_id}/recover-claimed`
- `POST /sessions/{session_id}/approval/resume-claimed`

It is a reference transport layer, not a kernel contract.
Platforms can use it to expose worker claim/lease control over HTTP, but the runtime semantics still live in `pkg/harness/runtime`.
