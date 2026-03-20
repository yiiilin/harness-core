package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/yiiilin/harness-core/adapters/websocket"
	"github.com/yiiilin/harness-core/internal/config"
	"github.com/yiiilin/harness-core/pkg/harness/builtins"
	hpostgres "github.com/yiiilin/harness-core/pkg/harness/postgres"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, args []string, stdout, _ io.Writer) error {
	if len(args) > 0 {
		switch args[0] {
		case "migrate":
			return runMigrate(ctx, args[1:], stdout)
		default:
			return fmt.Errorf("unknown command %q", args[0])
		}
	}

	cfg := config.Load()
	return serveWebsocket(ctx, cfg)
}

func serveWebsocket(ctx context.Context, cfg config.Config) error {
	opts := hruntime.Options{}
	builtins.Register(&opts)
	rt := hruntime.New(opts)
	if cfg.StorageMode == "postgres" {
		durable, db, err := hpostgres.OpenService(ctx, cfg.PostgresDSN, opts)
		if err != nil {
			return err
		}
		defer func() {
			if err := db.Close(); err != nil {
				log.Printf("close postgres db: %v", err)
			}
		}()
		rt = durable
	}
	srv := websocket.New(cfg, rt)
	return srv.ListenAndServe()
}

func runMigrate(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: harness-core migrate [status|up|version]")
	}

	cfg := config.Load()
	if cfg.PostgresDSN == "" {
		return fmt.Errorf("postgres DSN is required for migrate commands")
	}

	db, err := hpostgres.OpenDB(ctx, cfg.PostgresDSN)
	if err != nil {
		return err
	}
	defer db.Close()

	switch args[0] {
	case "status":
		return printMigrationStatus(ctx, db, stdout)
	case "up":
		if err := hpostgres.ApplyMigrations(ctx, db); err != nil {
			return err
		}
		current, err := hpostgres.SchemaVersion(ctx, db)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(stdout, "applied=%s latest=%s up_to_date=%t\n", current, hpostgres.LatestSchemaVersion(), current == hpostgres.LatestSchemaVersion())
		return nil
	case "version":
		current, err := hpostgres.SchemaVersion(ctx, db)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(stdout, "current=%s latest=%s up_to_date=%t\n", current, hpostgres.LatestSchemaVersion(), current == hpostgres.LatestSchemaVersion())
		return nil
	default:
		return fmt.Errorf("unknown migrate subcommand %q", args[0])
	}
}

func printMigrationStatus(ctx context.Context, db hpostgres.SchemaApplier, stdout io.Writer) error {
	current, err := hpostgres.SchemaVersion(ctx, db)
	if err != nil {
		return err
	}
	drift, err := hpostgres.HasSchemaDrift(ctx, db)
	if err != nil {
		return err
	}
	statuses, err := hpostgres.ListMigrationStatus(ctx, db)
	if err != nil {
		return err
	}
	pending, err := hpostgres.PendingMigrations(ctx, db)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "current=%s latest=%s up_to_date=%t pending=%d\n", current, hpostgres.LatestSchemaVersion(), !drift, len(pending))
	for _, status := range statuses {
		state := "pending"
		if status.Applied {
			state = "applied"
		}
		_, _ = fmt.Fprintf(stdout, "%s %s %s\n", status.Version, status.Name, state)
	}
	return nil
}
