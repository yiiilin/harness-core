// Package harness exposes the public, library-first entry point for harness-core.
//
// It provides:
//   - the main runtime constructor
//   - commonly used runtime contracts and types
//   - a stable import path for embedding applications
//
// Consumers should prefer importing pkg/harness first, and only reach into
// subpackages when they need lower-level control or implementation-specific details.
package harness
