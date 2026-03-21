// Package worker provides a thin claim/renew/run helper that works on top of the
// public runtime service surface. It keeps worker-specific concepts out of the
// kernel while letting embedders orchestrate short-lived claim loops with minimal
// repeated code.
package worker
