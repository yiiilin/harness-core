package persistence

import "context"

type MemoryUnitOfWork struct {
	repos RepositorySet
}

func NewMemoryUnitOfWork(repos RepositorySet) *MemoryUnitOfWork {
	return &MemoryUnitOfWork{repos: repos}
}

func (u *MemoryUnitOfWork) Within(ctx context.Context, fn func(repos RepositorySet) error) error {
	_ = ctx
	return fn(u.repos)
}
