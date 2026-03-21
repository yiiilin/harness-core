// Package worker provides a thin claim/renew/run helper that works on top of the
// public runtime service surface. It keeps worker-specific concepts out of the
// kernel while letting embedders orchestrate short-lived claim loops with minimal
// repeated code.
//
// The helper intentionally stays transport-neutral and fleet-neutral. It offers:
//   - one-shot claim/run/recover/release execution through RunOnce
//   - a reusable outer polling loop through RunLoop
//   - optional worker naming for logs/metrics wrappers
//   - additive loop observation and deterministic backoff controls
package worker
