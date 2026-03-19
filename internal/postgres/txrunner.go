package postgres

import (
  "context"
  "database/sql"

  "github.com/yiiilin/harness-core/pkg/harness/persistence"
)

type SQLTx struct {
  tx *sql.Tx
}

func (t SQLTx) Commit() error { return t.tx.Commit() }
func (t SQLTx) Rollback() error { return t.tx.Rollback() }

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
