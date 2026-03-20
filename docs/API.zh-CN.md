# API.zh-CN.md

## 文档目标

这份文档用于说明 `harness-core` 当前阶段的：

- 对外定位
- 推荐引用方式
- 核心对象模型
- 包边界建议
- 默认组合方式
- 最小运行链路

它不是完整 API 参考手册，而是一份**中文的快速理解与接入说明**。

---

## 一句话定位

`harness-core` 不是一个完整的 Agent 产品，而是一个：

> **可复用的 Harness Runtime Kernel（运行时内核库）**

它希望解决的是：
- 状态机
- action / result / verify 契约
- tool registry
- verifier registry
- policy / approval hooks
- audit / event hooks
- 最小默认 runtime 组件

而不是：
- UI
- SaaS 平台
- 大而全的内置工具产品

---

## 当前推荐的引用入口

最推荐先从：

```go
import "github.com/yiiilin/harness-core/pkg/harness"
```

开始，而不是一开始就直接深入很多子包。

### 顶层 facade 当前提供
- `harness.Options`
- `harness.New(...)`
- `harness.RegisterBuiltins(...)`（兼容包装）
- `pkg/harness/builtins.Register(...)`（推荐的 builtins 组合层）

这让嵌入方可以先走最短路径：

```go
import (
	"github.com/yiiilin/harness-core/pkg/harness"
	"github.com/yiiilin/harness-core/pkg/harness/builtins"
)

opts := harness.Options{}
builtins.Register(&opts)
rt := harness.New(opts)
```

后续再逐步替换：
- `PolicyEvaluator`
- `ContextAssembler`
- `Planner`
- `EventSink`
- tool / verifier registrations

### 持久化 Postgres 接入的推荐入口

如果嵌入方希望直接获得一个带持久化能力的 runtime，不要去 import `internal/*`，优先使用：

- `pkg/harness/postgres`
  - `OpenDB(...)`
  - `EmbeddedMigrations()`
  - `ApplyMigrations(...)`
  - `ApplySchema(...)`
  - `ListMigrationStatus(...)`
  - `PendingMigrations(...)`
  - `HasSchemaDrift(...)`
  - `SchemaVersion(...)`
  - `LatestSchemaVersion()`
  - `BuildOptions(...)`
  - `OpenService(...)`

这个包是一个公开的 durable bootstrap / composition 层。
它不是把 Postgres 变成内核概念，而是把已有的 Postgres wiring 公开成稳定接入面，方便平台直接嵌入。
其中 `ApplyMigrations(...)` 是推荐主路径，`ApplySchema(...)` 只是兼容包装。
迁移状态、pending 列表、drift 检查也应该走这个公开包，而不是让平台自己 import `internal/postgres`。

### 当前推荐关注的内核控制面入口

- 生命周期：
  - `CreateSession`
  - `CreateTask`
  - `AttachTaskToSession`
  - `CreatePlan`
  - `CreatePlanFromPlanner`
- 执行：
  - `RunStep`
  - `RunClaimedStep`
  - `RunSession`
  - `RunClaimedSession`
  - `RecoverSession`
  - `RecoverClaimedSession`
- 审批 / 协调：
  - `RespondApproval`
  - `ResumePendingApproval`
  - `ResumeClaimedApproval`
  - `ClaimRunnableSession`
  - `ClaimRecoverableSession`
  - `RenewSessionLease`
  - `ReleaseSessionLease`
  - `MarkClaimedSessionInFlight`
  - `MarkClaimedSessionInterrupted`

---

## 当前核心包说明

### `pkg/harness`
顶层 facade。适合作为默认入口。

### `pkg/harness/task`
定义任务对象：
- `task.Spec`
- `task.Record`
- `task.Status`
- `task.Store`

### `pkg/harness/session`
定义会话状态：
- `session.State`
- `session.Phase`
- `session.Store`

### `pkg/harness/plan`
定义计划与步骤：
- `plan.Spec`
- `plan.StepSpec`
- `plan.Status`
- `plan.StepStatus`
- `plan.Store`

### `pkg/harness/action`
定义动作与结果：
- `action.Spec`
- `action.Result`
- `action.Error`

### `pkg/harness/verify`
定义验证器：
- `verify.Spec`
- `verify.Check`
- `verify.Result`
- `verify.Registry`
- 内置 verifier（如 `exit_code`、`output_contains`）

### `pkg/harness/tool`
定义工具注册系统：
- `tool.Definition`
- `tool.Registry`
- `tool.Handler`

### `pkg/harness/permission`
定义权限与审批抽象：
- `permission.Decision`
- `permission.Action`
- `permission.Evaluator`
- 默认 evaluator

### `pkg/harness/audit`
定义事件与审计：
- `audit.Event`
- `audit.Store`

### `pkg/harness/runtime`
核心运行时：
- `runtime.Service`
- `runtime.Options`
- `runtime.RunStep(...)`
- transition / loop / defaults / planner / context / eventsink

### `pkg/harness/postgres`
公开的 Postgres durable bootstrap：
- 打开 DB
- 应用 versioned migrations
- 组装 Postgres-backed repositories / runner / event sink
- 直接构造持久化 runtime service

---

## 默认组件

当前 `harness-core` 已经有一套最小默认组合：

### 默认注册的内置工具
- `shell.exec`
- `windows.native`（占位 / 默认高风险）

### 默认注册的 verifier
- `exit_code`
- `output_contains`
- `pty_handle_active`
- `pty_stream_contains`
- `pty_exit_code`

### 默认组件
- `DefaultContextAssembler`
- `NoopPlanner`
- `DemoPlanner`（最小可运行 planner 样例）
- `AuditStoreSink`
- `DefaultEvaluator`

这意味着：
- 你可以很快跑起来
- 但也可以很快替换组件

---

## 当前最小运行链路

`RunStep()` 目前已经能跑通这条闭环：

```text
policy -> action -> verify -> transition -> state update -> audit
```

这条链路当前已经被 integration test 覆盖。

### Happy path 已验证
- `shell.exec`（pipe）
- `echo hello`
- `exit_code + output_contains`
- session / task / plan 最终完成
- audit 事件记录成功

### 当前 shell 参考能力
- `shell.exec` 支持 `pipe` 和 `pty`
- `pipe` 适合一次性命令执行
- `pty` 适合交互式会话启动，并通过 runtime handle 暴露句柄
- PTY 的 read/write/attach/detach/close 是模块/平台层控制面，不是内核 lease 语义的一部分
- PTY 专用 verifier 也在 `modules/shell`，不是内核新增语义

### 持久化嵌入的最短路径

```go
import (
	"context"

	"github.com/yiiilin/harness-core/pkg/harness/builtins"
	hpostgres "github.com/yiiilin/harness-core/pkg/harness/postgres"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
)

var opts hruntime.Options
builtins.Register(&opts)

rt, db, err := hpostgres.OpenService(context.Background(), dsn, opts)
if err != nil {
	panic(err)
}
defer db.Close()
```

如果只是想看一个最小可运行的 durable embedding 示例，可以直接看 `examples/postgres-embedded`。
如果想看多实例 worker 如何共享一个 Postgres-backed runtime，可以看 `examples/postgres-workers`。
默认的 WebSocket adapter 和 `adapters/http` 都只是参考传输层，不是持久化接入的唯一入口。
其中 `adapters/http` 现在额外暴露了 worker control-plane：claim / lease renew/release / claimed run / recover / approval resume，但这些仍然只是 transport 绑定，不是内核新概念。

### Deny path 已验证
- policy 返回 deny
- action 不执行
- task/session fail-safe
- `policy.denied` 事件被记录

---

## 当前适合谁用

当前版本最适合：
- 想做自己的 Agent Runtime
- 想要一套通用状态机 + tool/verify/policy 内核
- 想嵌入 shell / browser / desktop / knowledge executor
- 不想直接绑死到某个产品或模型提供商

不太适合：
- 直接拿来当完整产品
- 期待内置大量现成功能
- 期待零配置自动化平台能力

---

## 一个最小接入示例

```go
import (
	"github.com/yiiilin/harness-core/pkg/harness"
	"github.com/yiiilin/harness-core/pkg/harness/builtins"
)

opts := harness.Options{}
builtins.Register(&opts)
rt := harness.New(opts)

sess := rt.CreateSession("demo", "run one step")
// create task
// attach task to session
// create plan
// run step
```

如果你要走 WebSocket adapter，则：
- 启动 `cmd/harness-core`
- 通过 `/ws` 连接
- 先 auth
- 再发 `session.create` / `task.create` / `plan.create` / `step.run`

如果你要看“平台层如何消费 claim/lease + PTY shell”，则参考：
- `examples/platform-reference`

---

## 当前包边界建议

### 可以直接依赖的
建议优先：
- `pkg/harness`
- `pkg/harness/runtime`
- `pkg/harness/task`
- `pkg/harness/session`
- `pkg/harness/plan`
- `pkg/harness/action`
- `pkg/harness/verify`
- `pkg/harness/tool`
- `pkg/harness/permission`
- `pkg/harness/audit`

### 暂时更像适配层/示例的
- `adapters/websocket`
- `examples/*`
- `cmd/harness-core`

这些更适合参考实现，不建议一开始就把它们当稳定内核 API。

---

## 当前还缺什么

虽然现在已经能跑一个最小闭环，但还没到 API 完全稳定的程度。

接下来仍然值得补的有：
- 更清晰的 public API 边界说明
- 更多 typed errors
- 更多 executor/verifier 实现
- 更标准的 event/metrics hook
- 更完整的 planner / context assembler 默认实现
- 更丰富的系统测试与 benchmark

---

## 当前最推荐的使用姿势

如果你现在就要基于它做自己的系统，我建议：

1. 先走 `pkg/harness` 顶层入口
2. 先用默认 builtins
3. 先替换 `PolicyEvaluator`
4. 再替换 `ContextAssembler` / `Planner`
5. 最后才扩展更多 executor

这样你能最大限度保持：
- 简洁
- 可控
- 易测试

---

## 一句话总结

> **`harness-core` 当前已经像一个“可运行、可扩展、可测试的 Harness 内核雏形”。**
> **它最适合被当成运行时基础库来嵌入，而不是被当成完整产品直接使用。**
