package main

import (
	"log"

	"github.com/yiiilin/harness-core/adapters/websocket"
	"github.com/yiiilin/harness-core/internal/config"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
)

func main() {
	cfg := config.Load()
	sessions := session.NewMemoryStore()
	registry := tool.NewRegistry()
	registry.Register(tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true})
	registry.Register(tool.Definition{ToolName: "windows.native", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskHigh, Enabled: false})
	rt := hruntime.New(sessions, registry)
	srv := websocket.New(cfg, rt)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
