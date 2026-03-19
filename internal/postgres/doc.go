// Package postgres contains durable storage wiring for harness-core.
//
// It is intentionally internal because the kernel should depend on persistence
// abstractions rather than leak concrete database choices into the public API.
package postgres
