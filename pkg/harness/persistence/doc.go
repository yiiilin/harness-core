// Package persistence defines persistence-layer abstractions for harness-core.
//
// The immediate goal is not to provide production storage implementations yet,
// but to define the transaction and repository boundaries that a production
// runtime will need.
//
// The most important concept here is not a database driver. It is the step-level
// persistence boundary:
//
//	one logical runtime step
//	  -> updates session/task/plan/audit together
//
// Today the project provides an in-memory UnitOfWork implementation to validate
// the semantics. Durable implementations (for example Postgres-backed) should
// plug into the same contracts later.
package persistence
