package postgresruntime

import (
	"context"
	"database/sql"
	"github.com/yiiilin/harness-core/internal/postgres"
	hpostgres "github.com/yiiilin/harness-core/pkg/harness/postgres"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
)

func OpenDB(ctx context.Context, dsn string) (*sql.DB, error) {
	return hpostgres.OpenDB(ctx, dsn)
}

func ApplySchema(ctx context.Context, db postgres.DBTX) error {
	return hpostgres.ApplySchema(ctx, db)
}

func BuildOptions(db *sql.DB, opts hruntime.Options) hruntime.Options {
	return hpostgres.BuildOptions(db, opts)
}

func OpenService(ctx context.Context, dsn string, opts hruntime.Options) (*hruntime.Service, *sql.DB, error) {
	return hpostgres.OpenService(ctx, dsn, opts)
}
