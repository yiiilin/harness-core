package main

import (
	"log"

	"github.com/yiiilin/harness-core/adapters/websocket"
	"github.com/yiiilin/harness-core/internal/config"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
)

func main() {
	cfg := config.Load()
	opts := hruntime.Options{}
	hruntime.RegisterBuiltins(&opts)
	rt := hruntime.New(opts)
	srv := websocket.New(cfg, rt)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
