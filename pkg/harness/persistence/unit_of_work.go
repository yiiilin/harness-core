package persistence

import "context"

type UnitOfWork interface {
	Within(ctx context.Context, fn func(repos RepositorySet) error) error
}
