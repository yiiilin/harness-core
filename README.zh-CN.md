# harness-core

`harness-core` 是一个可复用的 **Harness Runtime Kernel（运行时内核库）**，用于构建 AI Agent 系统。

它的目标不是做成一个完整的 Agent 产品，而是提供一套：

- 通用状态机
- `action / result / verify` 契约
- 动态工具注册中心
- verifier 注册中心
- 权限 / 审批挂钩
- 审计 / 事件挂钩
- 适配器友好的运行时接口
- 可逐步替换的默认组件

---

## 它适合做什么

适合：
- 作为你自己的 Agent Runtime 的内核
- 嵌入 shell / browser / desktop / knowledge executor
- 构建可验证、可审计、可扩展的 agent 系统

不适合：
- 直接当完整 SaaS 产品用
- 期待内置大量现成功能
- 把它当 UI 或 workflow 平台

---

## 仓库模块布局

- 根内核模块：`github.com/yiiilin/harness-core`
- companion 组合模块：`github.com/yiiilin/harness-core/pkg/harness/builtins`
- companion capability-pack 模块：`github.com/yiiilin/harness-core/modules`
- companion adapter 模块：`github.com/yiiilin/harness-core/adapters`
- companion CLI 模块：`github.com/yiiilin/harness-core/cmd/harness-core`
- 仓库内开发通过已提交的 `go.work` 组织

根 `pkg/harness` facade 现在刻意只保留裸内核入口。
内置能力包组合通过 `pkg/harness/builtins` 完成，不再通过 root facade 的便捷包装器完成。

---

## 当前已经具备的能力

目前仓库里已经有：

- task / session / plan 对象模型
- 共享状态机与 transition primitives
- tool registry
- verifier registry
- 默认 policy evaluator
- shell pipe executor
- step runner（`policy -> action -> verify -> transition -> state update`）
- in-memory audit/event sink
- 默认 context assembler
- 默认 planner placeholder
- 默认 event sink bridge
- `pkg/harness/postgres` 公开 durable bootstrap
- `pkg/harness/worker` 公开 worker loop helper
- `pkg/harness/replay` 公开 replay/debug projection helper
- WebSocket adapter
- Go 示例客户端
- integration tests 和 benchmark baseline
- `modules/shell`
- `modules/filesystem`
- `modules/http`

---

## 推荐先读这些文档

- `docs/ARCHITECTURE.md`
- `docs/PROTOCOL.md`
- `docs/RUNTIME.md`
- `docs/POLICY.md`
- `docs/API.zh-CN.md`
- `docs/architecture-tour.html`（本地浏览器直接打开的中英双语架构总览页）
- `docs/MODULES.md`
- `docs/EXTENSIBILITY.md`
- `docs/ADAPTERS.md`
- `docs/EMBEDDING.md`
- `docs/RELEASING.md`

---

## 最推荐的初始化方式

显式使用裸内核加 companion builtins 模块：

```go
import (
  "github.com/yiiilin/harness-core/pkg/harness"
  "github.com/yiiilin/harness-core/pkg/harness/builtins"
)

opts := harness.Options{}
builtins.Register(&opts)
rt := harness.New(opts)
```

如果你后面要增强系统，可以逐步替换：
- `PolicyEvaluator`
- `ContextAssembler`
- `Planner`
- `EventSink`
- tool / verifier registrations

最稳定的接入路径仍然是：
- `pkg/harness`
- `pkg/harness/postgres`
- `pkg/harness/worker`
- `pkg/harness/replay`

`pkg/harness/builtins`、`modules/*`、`adapters/*`、`cmd/harness-core` 则属于公开但独立版本演进的 companion modules。
传输适配建议看 `docs/ADAPTERS.md`，WebSocket action 映射看 `docs/ADAPTER_PROTOCOL.md`，多模块发布流程看 `docs/RELEASING.md`。

---

## 推荐的 durable Postgres 接入方式

如果你的平台要接 durable runtime，优先使用 `pkg/harness/postgres.Config` 和 `OpenServiceWithConfig(...)`，而不是复制 `internal/*` wiring：

```go
var opts hruntime.Options
builtins.Register(&opts)

rt, db, err := hpostgres.OpenServiceWithConfig(context.Background(), hpostgres.Config{
  DSN:             dsn,
  Schema:          "agent_kernel",
  MaxOpenConns:    8,
  MaxIdleConns:    4,
  ConnMaxLifetime: 30 * time.Minute,
  ApplyMigrations: true,
}, opts)
if err != nil {
  panic(err)
}
defer db.Close()
```

`internal/config` 仍然只是 reference CLI 的 env loader，不是 embedder API。

---

## 运行临时 WebSocket adapter

```bash
export HARNESS_ADDR=127.0.0.1:8787
export HARNESS_SHARED_TOKEN=dev-token
go run ./cmd/harness-core
```

---

## 运行最小 happy-path 示例

```bash
go run ./examples/minimal-agent
```

---

## 运行 Go WebSocket 示例客户端

```bash
cd examples/go-client
export HARNESS_URL=ws://127.0.0.1:8787/ws
export HARNESS_TOKEN=dev-token
go run .
```

---

## 运行 durable 平台接入示例

```bash
export HARNESS_POSTGRES_DSN='postgres://harness:harness@127.0.0.1:5432/harness_test?sslmode=disable'
export HARNESS_POSTGRES_SCHEMA='platform_demo'
go run ./examples/platform-durable-embedding
```

---

## 运行 Postgres + WebSocket 接入示例

```bash
export HARNESS_POSTGRES_DSN='postgres://harness:harness@127.0.0.1:5432/harness_test?sslmode=disable'
export HARNESS_POSTGRES_SCHEMA='postgres_websocket_embedding'
go run ./examples/postgres-websocket-embedding
```

---

## 测试与性能基线

```bash
make test-workspace
make check-companion-versions
make test-external-consumers
make release-check
make release-preflight
go test -bench . -benchmem ./pkg/harness/runtime
```

- `make test-workspace` 会通过 `go.work` 跑完整个仓库的多模块测试
- `make check-companion-versions` 会校验当前已提交的 companion manifest 版本矩阵是否一致且可解析
- `make test-external-consumers` 会在空白外部 module 里验证 `@dev` 消费链，不依赖仓库内 `replace`
- `make release-check` 会在稳定内核 gate 之外一并检查 companion modules 的对外消费链路
- `make release-preflight` 会先做 workspace 测试，再做 release gate
- `make build` 会把参考服务端二进制输出到 `bin/harness-core`

---

## 模块化能力包

当前推荐的分层是：

```text
harness-core (kernel)
  -> modules/* (capability packs)
  -> adapters/* (transport bindings)
  -> examples/* (reference usage)
```

当前模块包括：
- `modules/shell`
- `modules/filesystem`
- `modules/http`

每个模块原则上都应该包含：
- tool 定义
- handler
- verifier
- 默认 policy hints
- tests

---

## 一句话总结

> `harness-core` 当前更像一个“可运行、可测试、可扩展的 Harness 内核雏形”，最适合被当作运行时基础库嵌入自己的 Agent 系统，而不是直接当成完整产品使用。
