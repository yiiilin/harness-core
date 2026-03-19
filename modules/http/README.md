# modules/http

This package is the reference capability module for common HTTP operations.

Current scope:
- `http.fetch`
- `http.post_json`
- verifier helpers:
  - `http_status_code`
  - `body_contains`
  - `json_field_equals`

The goal is to provide a small but realistic network capability module with
self-contained tool registration, verifier registration, default policy hints,
and tests.
