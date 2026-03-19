package persistence

import "context"

// Tx is the minimal transaction contract needed by the persistence runner.
// Concrete implementations may wrap *sql.Tx, pgx.Tx, or a custom durable store transaction.
type Tx interface {
	Commit() error
	Rollback() error
}

// TxManager begins transactional execution.
type TxManager interface {
	Begin(ctx context.Context) (Tx, error)
}

// TxRepositoryFactory converts an active transaction into a RepositorySet.
type TxRepositoryFactory interface {
	FromTx(tx Tx) RepositorySet
}

// TransactionalRunner executes work inside a begin/commit/rollback boundary.
// This is the generic durable runner shape; concrete persistence backends can plug in later.
type TransactionalRunner struct {
	Manager TxManager
	Factory TxRepositoryFactory
}

func (r TransactionalRunner) Within(ctx context.Context, fn func(repos RepositorySet) error) error {
	tx, err := r.Manager.Begin(ctx)
	if err != nil {
		return err
	}
	repos := r.Factory.FromTx(tx)
	if err := fn(repos); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
