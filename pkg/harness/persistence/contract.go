package persistence

import "context"

// Runner executes a function within a persistence boundary.
//
// Durable implementations may map this to a real transaction.
// In-memory implementations may simply run the closure directly.
type Runner interface {
	Within(ctx context.Context, fn func(repos RepositorySet) error) error
}
