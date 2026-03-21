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

配套文档：
- `docs/KERNEL_SCOPE.md`
- `docs/VERSIONING.md`
- `docs/EMBEDDING.md`

## 推荐公开面

### 顶层 facade

- `pkg/harness`
  - 构造器：
    - `harness.New(opts)`
    - `harness.NewDefault()`
  - 兼容组合包装：
    - `harness.NewWithBuiltins()`
    - `harness.RegisterBuiltins(&opts)`

### 组合辅助包

- `pkg/harness/builtins`
  - `builtins.New()`
  - `builtins.Register(&opts)`

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

- `pkg/harness/worker`
  - `worker.New(worker.Options{Runtime: rt, ...})`
  - `(*worker.Worker).RunOnce(ctx)`
  - `(*worker.Worker).RunLoop(ctx, worker.LoopOptions{...})`
  - `Runtime` 依赖的是 worker 专用窄接口，而不是强制要求具体 `*runtime.Service`
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
  - 只有存在本地 `PTYManager` 时才注册

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
- `pkg/harness/builtins`

参考实现，变化更快：
- `modules/*`
- `adapters/*`

无兼容承诺：
- `internal/*`
- `cmd/*`
- `examples/*`

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

rt, db, err := hpostgres.OpenService(context.Background(), dsn, opts)
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
