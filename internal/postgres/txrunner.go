package postgres

import (
	"context"
	"database/sql"

	"github.com/yiiilin/harness-core/pkg/harness/persistence"
)

type SQLTx struct {
	tx *sql.Tx
}

func (t SQLTx) Commit() error   { return t.tx.Commit() }
func (t SQLTx) Rollback() error { return t.tx.Rollback() }

func (t SQLTx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return t.tx.ExecContext(ctx, query, args...)
}

func (t SQLTx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return t.tx.QueryContext(ctx, query, args...)
}

func (t SQLTx) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return t.tx.QueryRowContext(ctx, query, args...)
}

type TxManager struct {
	DB *sql.DB
}

func (m TxManager) Begin(ctx context.Context) (persistence.Tx, error) {
	tx, err := m.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return SQLTx{tx: tx}, nil
}
