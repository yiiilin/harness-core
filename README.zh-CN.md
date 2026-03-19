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
- `docs/MODULES.md`

---

## 最推荐的初始化方式

```go
import "github.com/yiiilin/harness-core/pkg/harness"

opts := harness.Options{}
harness.RegisterBuiltins(&opts)
rt := harness.New(opts)
```

如果你后面要增强系统，可以逐步替换：
- `PolicyEvaluator`
- `ContextAssembler`
- `Planner`
- `EventSink`
- tool / verifier registrations

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

## 测试与性能基线

```bash
go test ./...
go test -bench . -benchmem ./pkg/harness/runtime
```

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
