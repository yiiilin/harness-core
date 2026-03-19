package main

import (
	"log"

	"github.com/yiiilin/harness-core/adapters/websocket"
	"github.com/yiiilin/harness-core/internal/config"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

func main() {
	cfg := config.Load()
	sessions := session.NewMemoryStore()
	tasks := task.NewMemoryStore()
	plans := plan.NewMemoryStore()
	tools := tool.NewRegistry()
	verifiers := verify.NewRegistry()

	tools.Register(tool.Definition{ToolName: "shell.exec", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskMedium, Enabled: true}, nil)
	tools.Register(tool.Definition{ToolName: "windows.native", Version: "v1", CapabilityType: "executor", RiskLevel: tool.RiskHigh, Enabled: false}, nil)

	verifiers.Register(verify.Definition{Kind: "exit_code", Description: "Verify that an execution result exit code is in the allowed set."}, verify.ExitCodeChecker{})
	verifiers.Register(verify.Definition{Kind: "output_contains", Description: "Verify that stdout or stderr contains a target substring."}, verify.OutputContainsChecker{})

	rt := hruntime.New(sessions, tasks, plans, tools, verifiers)
	srv := websocket.New(cfg, rt)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
