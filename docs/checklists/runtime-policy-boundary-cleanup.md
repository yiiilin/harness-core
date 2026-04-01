# Runtime Policy Boundary Cleanup

## 模式
- 强审开发模式

## 审核设置
- 审核模型目标：gpt-5.4
- 推理强度目标：xhigh
- 活跃事项上限：1
- 首次等待窗口：5min
- 二次探测窗口：5-10min
- 硬超时门槛：15min

## Checklist
- [x] 1. 锁定新 runtime policy / planner projection 边界并补齐失败测试
- [x] 2. 实现显式 runtime policy、统一 raw/reread contract，并移除旧的模糊预算入口
- [x] 3. 更新文档、完成全量回归与强审收口

## Item 1 - 锁定新 runtime policy / planner projection 边界并补齐失败测试
### 约束
- 依赖事项：无
- 冲突事项：2, 3
- 风险等级：高
- 并行状态：串行

### 计划
- 盘点当前 `LoopBudgets`、`ContextAssembler`、`Compactor`、planner 集成与 truncation surface 的耦合点。
- 先补失败测试，覆盖：
- 删除 `LoopBudgets.MaxToolOutputChars` 后的新 policy 默认与显式 planner projection 要求；
- planner 调用链必须区分 raw assembled context 与 projected planner context；
- truncation-capable surface 暴露统一 `ResultWindow` / `RawResultHandle` 契约；
- runtime-wide / per-tool / per-step policy override 的解析优先级。
- 只新增测试与必要测试夹具，不修改生产实现。

### 实施记录
- 已盘点当前耦合点：
- `LoopBudgets.MaxToolOutputChars` 仍被 `runner` / `fanout` inline trimming、context budget 测试与 planner 路径共享引用，输出预算与 planner/context 预算尚未拆开。
- `AssembleContextForSession` 目前直接走 `CompactSessionContext(..., plan)`，planner 没有显式 projection policy，也没有 raw-context 与 planner-context 的公开分离。
- truncation-capable surface 仍以 `RawRef` / 平铺 preview 字段暴露，没有统一 `ResultWindow` / `RawResultHandle` 契约。
- 新增失败测试：
- `pkg/harness/runtime/runtime_policy_cleanup_test.go`
- `modules/shell/result_window_contract_test.go`

### 验证记录
- `go test ./pkg/harness/runtime -run 'Test(WithDefaultsSetsRuntimePolicyDefaultsAndPlannerProjectionRemainsExplicit|CreatePlanFromPlannerRequiresExplicitPlannerProjectionPolicy|ProjectPlannerContextAppliesInlineProjectionWithoutMutatingRawContext|CreatePlanFromPlannerUsesProjectedContext|RunStepUsesStepToolThenRuntimeOutputPolicyPrecedence|RunStepExposesUnifiedResultWindowAndRawHandle)$'`
- 结果：构建失败；当前 `runtime.Options` 没有 `RuntimePolicy`，也缺少 `ErrPlannerProjectionPolicyRequired`、`ProjectPlannerContext`、`Result.Window` / `Result.RawHandle` 等新契约入口。
- `go test ./modules/shell -run 'TestInteractiveViewExposesUnifiedWindowAndRawHandleContract$'`
- 结果：构建失败；当前 `InteractiveViewResult` 仍是平铺 preview 字段，没有统一 `Window` / `RawHandle` 契约，`runtime.Options` 也尚无新的 output policy 配置。
- 根据 reviewer 反馈已放松 3 个过度约束断言：
- 不再把 `RawHandle.Kind` 锁定为特定 discriminator；
- 不再把 planner inline projection 锁定为固定前缀截断算法。
- 放松后重新运行上述 2 组测试，失败点保持不变，仍然稳定指向缺失的新 runtime policy / planner projector / unified window contract API。

### 审核记录
- Reviewer：`019d4813-f0f6-7472-93c9-3631f6f20150` (`Carson`)
- Reviewer 状态：`closed`
- 开始时间：`2026-04-01T06:15:00Z`
- 累计等待时长：`<5min`
- 超时次数：`0`
- 审核轮次：`2`
- 审核结论：首轮未通过；指出 2 个 medium + 1 个 low 的测试过度约束点。修正后复审结论为 `无 findings`，`Item 1 approved`。
- Replacement Reviewer：待填写
- 关闭状态：reviewer 已正常关闭
- 关闭原因：review 完成

## Item 2 - 实现显式 runtime policy、统一 raw/reread contract，并移除旧的模糊预算入口
### 约束
- 依赖事项：1
- 冲突事项：3
- 风险等级：高
- 并行状态：串行

### 计划
- 引入新的 runtime policy 类型，拆分 transport / inline / planner / raw-contract 边界。
- 删除 `LoopBudgets.MaxToolOutputChars`，并把旧调用点切换到新的 policy 解析路径。
- 重构 planner 集成：raw assembled context、runtime compaction、planner projection 三者显式分离，planner projection 必须配置。
- 收敛 pipe / PTY / direct tool / fanout 的 raw handle 与 result window 公开契约。
- 预计改动文件：
- `pkg/harness/runtime/context_types.go`
- `pkg/harness/runtime/options.go`
- `pkg/harness/runtime/service.go`
- `pkg/harness/runtime/interfaces.go`
- `pkg/harness/runtime/planning.go`
- `pkg/harness/runtime/action_result_channels.go`
- `pkg/harness/runtime/artifact_reads.go`
- `pkg/harness/runtime/interactive_control.go`
- `pkg/harness/runtime/runner.go`
- `pkg/harness/runtime/fanout_scheduler.go`
- `pkg/harness/runtime/program_interactive_actions.go`
- `pkg/harness/action/spec.go`
- `pkg/harness/harness.go`
- `modules/shell/pty_manager.go`
- `modules/shell/interactive_controller.go`
- 重点验证：
- `go test ./pkg/harness/runtime -run 'Test(WithDefaultsSetsRuntimePolicyDefaultsAndPlannerProjectionRemainsExplicit|CreatePlanFromPlannerRequiresExplicitPlannerProjectionPolicy|ProjectPlannerContextAppliesInlineProjectionWithoutMutatingRawContext|CreatePlanFromPlannerUsesProjectedContext|RunStepUsesStepToolThenRuntimeOutputPolicyPrecedence|RunStepExposesUnifiedResultWindowAndRawHandle)$'`
- `go test ./modules/shell -run 'TestInteractiveViewExposesUnifiedWindowAndRawHandleContract$'`
- 视编译影响补跑：
- `go test ./pkg/harness/runtime -run 'TestRunStep(ExposesRawArtifactReferenceAndRereadWindows|StoresRecoverableRawOutputWhenInlineTrimmed|VerificationUsesRawActionResultWhenInlineTrimmed)$'`
- 已知风险和边界：
- `LoopBudgets` 仍承载 step/retry/runtime budget；本项只移除输出预算，不顺手改动其它 loop 预算语义。
- planner compaction 仍可存在，但必须从 planner-facing projection 中剥离，避免再次把 runtime compaction 当成 planner 语义压缩。
- 统一 raw/window contract 会破坏旧字段访问点，需要同步更新 runtime、shell module 与文档/测试引用。

### 实施记录
- 新增显式 runtime policy：
- `pkg/harness/runtime/runtime_policy.go` 定义 `RuntimePolicy`、`OutputPolicy`、`OutputModePolicy`、`TransportBudgetPolicy`、`InlineBudgetPolicy`、`RawResultPolicy`、`PlannerPolicy`、`PlannerProjectionPolicy`、`PlannerContextBudgetPolicy`。
- `pkg/harness/runtime/options.go` / `pkg/harness/runtime/service.go` 接入 `Options.RuntimePolicy` 与 `Service.RuntimePolicy`，并新增默认 policy 归一化逻辑。
- `pkg/harness/runtime/context_types.go` 删除 `LoopBudgets.MaxToolOutputChars`，保留 `LoopBudgets` 只承担 step/retry/runtime budget。
- 重构 planner 边界：
- `pkg/harness/runtime/planning.go` 将 `AssembleContextForSession` 改为 raw assemble，不再隐式 compact。
- `pkg/harness/runtime/planner_projection.go` 新增 `ProjectPlannerContext`，支持 `raw` / `inline` / `custom` 三种显式模式；未配置时返回 `ErrPlannerProjectionPolicyRequired`。
- `CreatePlanFromPlanner` 现在显式执行：`raw assemble -> runtime compaction(side-effect/durable summary) -> planner projection -> planner call`。
- 统一 raw/window 公开 contract：
- `pkg/harness/action/spec.go` 为 `action.Result` 增加 `Window` / `RawHandle`，替换旧的零散 trim metadata。
- `pkg/harness/runtime/action_result_channels.go` 将 inline trimming 改为写统一 `ResultWindow`，并保留 raw payload 通道。
- `pkg/harness/runtime/artifact_reads.go`、`pkg/harness/runtime/interactive_control.go`、`modules/shell/pty_manager.go`、`modules/shell/interactive_controller.go` 全部切到统一 `Window` / `RawHandle` 合约。
- `pkg/harness/runtime/runner.go` 与 `pkg/harness/runtime/fanout_scheduler.go` 改为按 `step override -> tool override -> runtime default` 解析 output policy，并在持久化 raw artifact 后回填 `RawHandle`。
- `pkg/harness/runtime/program_interactive_actions.go` 改为透传统一 `window` / `raw_handle` 结构。
- `pkg/harness/harness.go` 暴露新的 runtime policy / result contract 别名与常量。
- 同步迁移受影响测试夹具：
- runtime tests 改为显式配置 planner projection，并把 inline/raw 断言改到 `Window` / `RawHandle`。
- shell PTY tests 改为统一 contract 断言。
- 根据 reviewer 第 1 轮反馈补充修复：
- `pkg/harness/runtime/planner_projection.go` 的 inline projection 改为对 `map[string]any` 做排序后稳定遍历，并按 rune 而不是 byte 消耗 `MaxChars`，避免随机字段保留和无效 UTF-8 截断。
- `pkg/harness/runtime/artifact_reads.go` 为 `ReadArtifact` 增加结构化值的 JSON 默认 reread 路径；空 `Path` 时可以直接按 `RawHandle.Ref` 返回整份 raw payload window，不再要求调用方猜内部 schema。
- `pkg/harness/executor/shell/pipe.go` 的 preview `Window` 改为按 payload JSON 大小计算 `OriginalBytes` / `ReturnedBytes` / `NextOffset`，避免与默认 reread 路径脱节。
- 新增补充回归：
- `pkg/harness/runtime/runtime_policy_cleanup_test.go` 新增 `TestProjectPlannerContextInlineProjectionIsDeterministicAndRuneSafe`
- `pkg/harness/runtime/runtime_policy_cleanup_test.go` 在 `TestRunStepExposesUnifiedResultWindowAndRawHandle` 中补充“仅凭 `RawHandle.Ref` 直接 reread 默认 payload window”断言。
- 根据 reviewer 第 2 轮反馈补充修复：
- `pkg/harness/runtime/action_result_channels.go` 的 `Window` 计量改为基于和 artifact 持久化完全一致的 JSON envelope（`data/meta/error` map），让 `Action.Window.NextOffset` 与默认 `ReadArtifact(rawHandle.Ref,{})` 流保持一致。
- `pkg/harness/executor/shell/contracts.go` / `modules/shell/module.go` 为 pipe execution request 增加显式 `max_output_bytes`。
- `pkg/harness/runtime/runner.go` 在执行 `shell.exec` pipe 动作前按 `OutputPolicy.Transport.MaxBytes` 注入 transport budget，确保 runtime policy 真正接管 pipe preview 边界，而不是继续落回 backend 私有默认值。
- `pkg/harness/runtime/runtime_policy_cleanup_test.go` 新增 `TestRunStepAppliesRuntimeTransportBudgetToPipeExecution`。
- 根据 reviewer 第 3 轮反馈补充修复：
- `pkg/harness/executor/shell/pipe.go` 的 `Invoke` 现在也会读取 `args["max_output_bytes"]` 并传入 `Request.MaxOutputBytes`，保证直接注册 `shellexec.PipeExecutor{}` 的 runtime 路径同样 obey `RuntimePolicy.Output.Transport`。
- `pkg/harness/runtime/runtime_policy_cleanup_test.go` 新增 `TestRunStepAppliesRuntimeTransportBudgetToDirectPipeExecutor`，覆盖不经过 shell module wrapper 的 direct executor 路径。

### 验证记录
- `go test ./pkg/harness/runtime -run 'Test(WithDefaultsSetsRuntimePolicyDefaultsAndPlannerProjectionRemainsExplicit|CreatePlanFromPlannerRequiresExplicitPlannerProjectionPolicy|ProjectPlannerContextAppliesInlineProjectionWithoutMutatingRawContext|CreatePlanFromPlannerUsesProjectedContext|RunStepUsesStepToolThenRuntimeOutputPolicyPrecedence|RunStepExposesUnifiedResultWindowAndRawHandle)$'`
- 结果：通过
- `go test ./modules/shell -run 'TestInteractiveViewExposesUnifiedWindowAndRawHandleContract$'`
- 结果：通过
- `go test ./pkg/harness/runtime -run 'TestRunStep(ExposesRawArtifactReferenceAndRereadWindows|StoresRecoverableRawOutputWhenInlineTrimmed|VerificationUsesRawActionResultWhenInlineTrimmed)$'`
- 结果：通过
- `go test ./pkg/harness/runtime ./modules/shell`
- 结果：通过
- reviewer 修复后追加验证：
- `go test ./pkg/harness/runtime -run 'Test(WithDefaultsSetsRuntimePolicyDefaultsAndPlannerProjectionRemainsExplicit|CreatePlanFromPlannerRequiresExplicitPlannerProjectionPolicy|ProjectPlannerContextAppliesInlineProjectionWithoutMutatingRawContext|ProjectPlannerContextInlineProjectionIsDeterministicAndRuneSafe|CreatePlanFromPlannerUsesProjectedContext|RunStepUsesStepToolThenRuntimeOutputPolicyPrecedence|RunStepExposesUnifiedResultWindowAndRawHandle)$'`
- 结果：通过
- `go test ./pkg/harness/runtime ./modules/shell`
- 结果：通过
- 第 2 轮 reviewer 修复后追加验证：
- `go test ./pkg/harness/runtime -run 'Test(WithDefaultsSetsRuntimePolicyDefaultsAndPlannerProjectionRemainsExplicit|CreatePlanFromPlannerRequiresExplicitPlannerProjectionPolicy|ProjectPlannerContextAppliesInlineProjectionWithoutMutatingRawContext|ProjectPlannerContextInlineProjectionIsDeterministicAndRuneSafe|CreatePlanFromPlannerUsesProjectedContext|RunStepUsesStepToolThenRuntimeOutputPolicyPrecedence|RunStepExposesUnifiedResultWindowAndRawHandle|RunStepAppliesRuntimeTransportBudgetToPipeExecution)$'`
- 结果：通过
- `go test ./pkg/harness/runtime ./modules/shell ./pkg/harness/executor/shell`
- 结果：通过
- 第 3 轮 reviewer 修复后追加验证：
- `go test ./pkg/harness/runtime -run 'Test(WithDefaultsSetsRuntimePolicyDefaultsAndPlannerProjectionRemainsExplicit|CreatePlanFromPlannerRequiresExplicitPlannerProjectionPolicy|ProjectPlannerContextAppliesInlineProjectionWithoutMutatingRawContext|ProjectPlannerContextInlineProjectionIsDeterministicAndRuneSafe|CreatePlanFromPlannerUsesProjectedContext|RunStepUsesStepToolThenRuntimeOutputPolicyPrecedence|RunStepExposesUnifiedResultWindowAndRawHandle|RunStepAppliesRuntimeTransportBudgetToPipeExecution|RunStepAppliesRuntimeTransportBudgetToDirectPipeExecutor)$'`
- 结果：通过
- `go test ./pkg/harness/runtime ./modules/shell ./pkg/harness/executor/shell`
- 结果：通过

### 审核记录
- Reviewer：`019d484b-3235-7ee2-b287-d4a6a98197ae` (`Mill`)
- Reviewer 状态：`closed`
- 开始时间：`2026-04-01T08:53:16Z`
- 累计等待时长：`<5min`
- 超时次数：`0`
- 审核轮次：`3`
- 审核结论：
- 第 1 轮 reviewer 因工具链未暴露回传句柄，无法继续追踪，未作为最终 reviewer 保留。
- Replacement reviewer 第 1 轮发现 `1` 个 medium：
- 直接注册 `shellexec.PipeExecutor{}` 的 runtime 路径仍未读取 `max_output_bytes`，导致 transport budget 只在 shell module wrapper 生效。
- 修复后复审结论：`无 findings`，`Item 2 approved`。
- Replacement Reviewer：`019d484b-3235-7ee2-b287-d4a6a98197ae` (`Mill`)
- 关闭状态：reviewer 已正常关闭
- 关闭原因：review 完成
- 备注：旧 reviewer 因句柄缺失无法复用，已由 replacement reviewer 完成最终审批并关闭。

## Item 3 - 更新文档、完成全量回归与强审收口
### 约束
- 依赖事项：1, 2
- 冲突事项：无
- 风险等级：中
- 并行状态：串行

### 计划
- 更新 runtime/API/embedding 文档，明确新 runtime policy、planner projection、result window/raw handle 契约。
- 运行 focused + full verification，并把结果写回 checklist。
- 为本事项启动独立 reviewer，若有问题则循环修复直到 reviewer 明确通过，再关闭 reviewer 并勾选事项。
- 目标文档：
- `docs/API.md`
- `docs/EMBEDDING.md`
- `docs/RUNTIME.md`
- 验证目标：
- `go test ./...`
- 必要时对失败项做最小修复后重跑，直到 Item 3 reviewer 明确通过。

### 实施记录
- 更新公开文档，明确 runtime 和上层 product 的边界：
- `docs/RUNTIME.md` 补充 `RuntimePolicy`、`PlannerProjectionPolicy`、`ResultWindow` / `RawResultHandle` 与 raw-first consumer 规则。
- `docs/API.md`、`docs/EMBEDDING.md` 同步新的 runtime 配置入口、planner projection 显式要求，以及 artifact reread / raw handle 的使用方式。
- 完成 `modules/shell` 兼容性收口，避免 companion module 对当前 dev root 新增字段产生编译期硬依赖：
- `modules/shell/pty_manager.go` 将 PTY read window / raw handle 切换为本地兼容类型。
- `modules/shell/interactive_controller.go` 通过反射为 `hruntime.InteractiveViewResult` 可选写入 `Window` / `RawHandle`，兼容旧 root API。
- `modules/shell/module.go` 通过反射可选下发 `shellexec.Request.MaxOutputBytes`，避免 `modules` 对 v1.0.2 root 直接引用不存在字段。
- 修正受兼容层影响的夹具与示例：
- `modules/shell/pty_test.go`、`pkg/harness/runtime/program_test.go`、`examples/platform-reference/main.go` 改为使用 `shellmodule.ResultWindow`。
- 根据 Item 3 replacement reviewer 反馈补充兼容性修复：
- `modules/shell/pty_manager.go` 为 `PTYReadResult` 恢复 `NextOffset` / `OriginalBytes` / `ReturnedBytes` / `Truncated` / `HasMore` / `RawRef` 旧平铺字段别名，并保证与 `Window` / `RawHandle` 同步，避免 companion module 对外 public API 无意破坏。
- `modules/shell/interactive_controller.go` 为旧 root 版本的 `hruntime.InteractiveViewResult` 兼容回填 `Truncated` / `OriginalBytes` / `ReturnedBytes` / `HasMore` / `NextOffset` / `RawRef`，修复旧 root 兼容路径上的 runtime 行为回退。
- 新增 `modules/shell/compat_legacy_fields_test.go`，锁定 legacy preview 字段兼容行为。
- `release/companion_external_test.go` 的 `modules-shell-package` 外部消费者用例新增 `PTYReadResult` 旧字段访问，防止未来仅验证 import/build 而漏掉字段级 public API 破坏。
- 为了让 release 外部消费者构建测试在当前环境稳定执行，`release/companion_external_test.go` 改为仅让 `github.com/yiiilin/harness-core` 走 `GONOPROXY` 直连/本地快照，公共依赖恢复使用 `GOPROXY=https://proxy.golang.org,direct`，不再把 `github.com/lib/pq` 等公共模块强制走 `git ls-remote`。

### 验证记录
- `go test ./modules/shell ./pkg/harness/executor/shell ./pkg/harness/runtime`
- 结果：通过
- `go test ./release -run TestCompanionModulesTrackCommittedCompatibilityMatrix -count=1`
- 结果：通过
- `go test ./release -run TestExternalConsumersBuildAgainstSnapshotRepo/root-kernel-package -count=1`
- 结果：通过
- `go test ./release -run TestExternalConsumersBuildAgainstSnapshotRepo/modules-shell-package -count=1`
- 结果：通过
- `go test ./modules/shell -run 'Test(PTYReadResultKeepsLegacyPreviewFields|SetInteractiveViewLegacyPreviewFieldsBackfillsLegacyShape|InteractiveViewExposesUnifiedWindowAndRawHandleContract)' -count=1`
- 结果：通过
- `go test ./...`
- 结果：通过
- `git diff --check`
- 结果：通过

### 审核记录
- Reviewer：`019d488e-dabe-7733-a69e-3c777a1ef480` (`Lorentz`)
- Reviewer 状态：`replaced`
- 开始时间：`2026-04-01T10:20:48Z`
- 累计等待时长：`15min`
- 超时次数：`3`
- 审核轮次：`4`
- 审核结论：
- 第 1 轮 replacement reviewer 发现 `2` 个 medium：
- `PlannerProjectionInline` 依赖随机 map 遍历且按 byte 而非 rune 处理 `MaxChars`，会导致 planner preview 不稳定且可能产生无效 UTF-8。
- `RawHandle.Ref` 默认 reread 仍需调用方知道内部 schema/path，`pipe` preview `NextOffset` 也没有和统一 reread 路径对齐。
- 第 2 轮 replacement reviewer 发现 `2` 个 medium：
- `Action.Window` 的 payload 计量仍然和默认 `RawHandle` reread JSON 流不一致。
- `RuntimePolicy.Output.Transport` 还没有真正接管 `shell.exec` pipe execution 的 transport budget。
- 第 3 轮 replacement reviewer 发现 `1` 个 medium：
- 直接注册 `shellexec.PipeExecutor{}` 的 runtime 路径仍未读取 `max_output_bytes`，导致 transport budget 只在 shell module wrapper 生效。
- 最终复审结论：`无 findings`
- 第 4 轮 reviewer 针对本轮 Item 3 收口补丁重新审查：
- 审查范围：
- `modules/shell/module.go`
- `modules/shell/interactive_controller.go`
- `modules/shell/pty_manager.go`
- `modules/shell/pty_test.go`
- `pkg/harness/runtime/program_test.go`
- `examples/platform-reference/main.go`
- `release/companion_external_test.go`
- `docs/API.md`
- `docs/EMBEDDING.md`
- `docs/RUNTIME.md`
- 当前结论：
- replacement reviewer 首轮给出 `1 high + 1 medium + 1 low`：
- `modules/shell/pty_manager.go` 将 `PTYReadResult` 从平铺字段直接切到 `Window` / `RawHandle`，会破坏现有 companion-module 消费者的字段访问编译兼容性。
- `modules/shell/interactive_controller.go` 没有给旧 root 版本的 `InteractiveViewResult` 回填 legacy preview 字段，导致兼容路径上 `Truncated` 等值会退化为零值。
- checklist 审核记录对 replacement reviewer 句柄状态的描述前后不一致。
- 上述问题已修复，并已追加回归：
- `modules/shell/compat_legacy_fields_test.go`
- `release/companion_external_test.go` 的 `modules-shell-package` 字段访问覆盖
- 同一 replacement reviewer 复审结论：`无 findings`。
- Replacement Reviewer：`019d489f-0114-72a1-b22f-3e83d47dea2e` (`Popper`)
- Replacement Reviewer 状态：`closed`
- Replacement Reviewer 开始时间：`2026-04-01T10:38:36Z`
- Replacement Reviewer 累计等待时长：`<10min`
- Replacement Reviewer 超时次数：`1`
- 最终复审结论：`无 findings`
- 关闭状态：reviewer 已正常关闭
- 关闭原因：review 完成
