package main

import (
	"context"
	"log"

	"github.com/yiiilin/harness-core/adapters/websocket"
	"github.com/yiiilin/harness-core/internal/config"
	"github.com/yiiilin/harness-core/internal/postgresruntime"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
)

func main() {
	cfg := config.Load()
	opts := hruntime.Options{}
	hruntime.RegisterBuiltins(&opts)
	rt := hruntime.New(opts)
	if cfg.StorageMode == "postgres" {
		var dbClose func() error
		db, err := postgresruntime.OpenDB(context.Background(), cfg.PostgresDSN)
		if err != nil {
			log.Fatal(err)
		}
		if err := postgresruntime.ApplySchema(context.Background(), db); err != nil {
			_ = db.Close()
			log.Fatal(err)
		}
		dbClose = db.Close
		defer func() {
			if err := dbClose(); err != nil {
				log.Printf("close postgres db: %v", err)
			}
		}()
		rt = hruntime.New(postgresruntime.BuildOptions(db, opts))
	}
	srv := websocket.New(cfg, rt)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
