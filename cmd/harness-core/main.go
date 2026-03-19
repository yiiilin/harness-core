package main

import (
	"log"

	"github.com/yiiilin/harness-core/internal/config"
	"github.com/yiiilin/harness-core/internal/runtime"
	"github.com/yiiilin/harness-core/internal/server"
	"github.com/yiiilin/harness-core/internal/tool"
)

func main() {
	cfg := config.Load()
	store := runtime.NewStore()
	registry := tool.NewRegistry()
	registry.Register(tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium})
	registry.Register(tool.Definition{ToolName: "windows.native", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskHigh})
	srv := server.New(cfg, store, registry)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
