# API.zh-CN.md

## 文档目标

这份文档定义 `harness-core` 的嵌入方公开 API 面。

推荐入口：

```go
import "github.com/yiiilin/harness-core/pkg/harness"
```

范围约束：
- 只暴露执行内核能力
- 不把传输、认证、用户、租户、产品概念塞进内核类型

仓库模块布局：
- 根内核模块：`github.com/yiiilin/harness-core`
- companion 组合模块：`github.com/yiiilin/harness-core/pkg/harness/builtins`
- companion capability-pack 模块：`github.com/yiiilin/harness-core/modules`
- companion adapter 模块：`github.com/yiiilin/harness-core/adapters`
- companion CLI 模块：`github.com/yiiilin/harness-core/cmd/harness-core`
- 仓库内本地开发通过已提交的 `go.work` 组织

配套文档：
- `docs/KERNEL_SCOPE.md`
- `docs/VERSIONING.md`
- `docs/EMBEDDING.md`
- `docs/ADAPTERS.md`
- `docs/RELEASING.md`

## 推荐公开面

### 顶层 facade

- `pkg/harness`
  - 构造器：
    - `harness.New(opts)`
    - `harness.NewDefault()`

### 稳定的根模块辅助包

- `pkg/harness/postgres`
  - `Config`
  - `EnsureSchema(...)`
  - `OpenDB(...)`
  - `OpenDBWithConfig(...)`
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
  - `OpenServiceWithConfig(...)`

- `pkg/harness/worker`
  - `worker.New(worker.Options{Runtime: rt, ...})`
  - `(*worker.Worker).RunOnce(ctx)`
  - `(*worker.Worker).RunLoop(ctx, worker.LoopOptions{...})`
  - `Runtime` 依赖的是 worker 专用窄接口，而不是强制要求具体 `*runtime.Service`
  - 增量 helper 能力：
    - 可选 `Options.Name`，方便嵌入方打日志 / 指标标签
    - 可选 `LoopOptions.Observe`，用于观察每轮 loop 结果
    - 确定性的 idle/error polling backoff：`LoopOptions{IdleWait, MaxIdleWait, IdleBackoffFactor, ErrorWait, MaxErrorWait, ErrorBackoffFactor}`
  - 结果标志：
    - `NoWork`
    - `ApprovalPending`

- `pkg/harness/replay`
  - `replay.NewReader(source)`
  - `(*replay.Reader).SessionProjection(sessionID)`
  - `(*replay.Reader).ExecutionCycleProjection(sessionID, cycleID)`
  - 便捷函数：
    - `LoadSessionProjection(...)`
    - `LoadCycleProjection(...)`

### 公开的 companion 组合模块

- `pkg/harness/builtins`
  - `builtins.New()`
  - `builtins.Register(&opts)`
  - 导入路径不变，但现在由单独的 `go.mod` 发布
  - 它负责默认能力包组合，不属于裸内核稳定承诺的一部分

### 公开的 companion 模块

- `modules/*`
- `adapters/*`
- `cmd/harness-core`

### Runtime 控制面

生命周期：
- `CreateSession`
- `CreateTask`
- `AttachTaskToSession`
- `CreatePlan`
- `CreatePlanFromPlanner`

执行：
- `RunStep`
- `RunClaimedStep`
- `RunSession`
- `RunClaimedSession`
- `RecoverSession`
- `RecoverClaimedSession`
- `AbortSession`

审批与协调：
- `RespondApproval`
- `ResumePendingApproval`
- `ResumeClaimedApproval`
- `ClaimRunnableSession`
- `ClaimRecoverableSession`
- `RenewSessionLease`
- `ReleaseSessionLease`
- `MarkClaimedSessionInFlight`
- `MarkClaimedSessionInterrupted`

执行事实读接口：
- `ListAttempts`
- `ListActions`
- `ListVerifications`
- `ListArtifacts`
- `ListRuntimeHandles`
- `ListCapabilitySnapshots`
- `ListContextSummaries`
- `ListAuditEvents`
- `ListExecutionCycles`
- `GetExecutionCycle`

Runtime handle 控制：
- `UpdateRuntimeHandle`
- `CloseRuntimeHandle`
- `InvalidateRuntimeHandle`

上下文维护：
- `CompactSessionContext`

### facade 导出类型

`pkg/harness` 会导出核心领域和控制类型，包括：
- task/session/plan/action/verify 类型
- permission decision/action 类型
- tool definition/risk 类型
- audit event 类型
- execution facts：
  - attempt/action/verification/artifact/runtime handle/execution cycle
- runtime 控制类型：
  - `StepRunOutput`
  - `SessionRunOutput`
  - `AbortRequest`
  - `AbortOutput`
  - `RuntimeHandleUpdate`
  - `RuntimeHandleCloseRequest`
  - `RuntimeHandleInvalidateRequest`
  - `CompactionTrigger`
  - `CompactionPolicy`
  - `LoopBudgets`
- worker helper 类型：
  - `WorkerLoopIteration`

## Shell 模块嵌入说明

`modules/shell` 属于 capability module，不是内核包，但嵌入方常用。
当前扩展语义：

- `RegisterWithOptions(..., shellmodule.Options{PTYBackend: ...})` 支持外部 PTY 执行后端
- `RegisterWithOptions(..., shellmodule.Options{PTYInspector: ...})` 支持外部 PTY 检查 / verifier 接线
- `PTYManager` 仍是本地 PTY 执行与检查的默认路径
- PTY 专用 verifier 条件注册：
  - `pty_handle_active`
  - `pty_stream_contains`
  - `pty_exit_code`
  - 只有存在 PTY inspection 能力时才注册，可以来自 `PTYManager` 或显式 `PTYInspector`
  - `pty_handle_active` 会沿用 verifier 调用时的 `context.Context`
  - `pty_stream_contains` 能从 `shell_stream`、`runtime_handle`、`runtime_handles` 三种结果字段里解析 PTY handle
  - 若结果里包含 `shell_stream.next_offset`，`pty_stream_contains` 默认会从该 offset 开始读取

含义：
- 仅接入远端 PTY backend，并不自动具备本地 PTY stream 检查能力

## 稳定性分层

详细规则见 `docs/VERSIONING.md`。

最稳定嵌入面：
- `pkg/harness`
- `pkg/harness/postgres`
- `pkg/harness/worker`
- `pkg/harness/replay`

公开但 pre-1.0 仍快速演进：
- `pkg/harness/runtime`
- `pkg/harness/task`
- `pkg/harness/session`
- `pkg/harness/plan`
- `pkg/harness/action`
- `pkg/harness/verify`
- `pkg/harness/tool`
- `pkg/harness/permission`
- `pkg/harness/audit`
- `pkg/harness/persistence`
- `pkg/harness/observability`
- `pkg/harness/executor/*`

公开但独立版本演进、变化更快的 companion 模块：
- `pkg/harness/builtins`
- `modules/*`
- `adapters/*`
- `cmd/harness-core`

无兼容承诺：
- `internal/*`
- `examples/*`
- `docs/plans/*`

## 最短接入路径

```go
import (
	"context"
	"time"

	"github.com/yiiilin/harness-core/pkg/harness/builtins"
	hpostgres "github.com/yiiilin/harness-core/pkg/harness/postgres"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/worker"
)

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

helper, err := worker.New(worker.Options{
	Runtime:  rt,
	LeaseTTL: time.Minute,
})
if err != nil {
	panic(err)
}
_, _ = helper.RunOnce(context.Background())
```

关于“外部 run_id、外部审批 UI、远端 PTY、重启恢复、accepted-first API 包装”的完整接入建议，请看 `docs/EMBEDDING.md`。
关于 transport 适配规则与事件/错误映射，请看 `docs/ADAPTERS.md`。

重要边界：
- `pkg/harness/postgres.Config` 是给 embedding 平台使用的 durable bootstrap config
- `internal/config` 仍然只是 reference CLI 的 env loader，不属于公开 embedder API
