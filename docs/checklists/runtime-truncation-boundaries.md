# Runtime Truncation Boundaries And Recoverable Raw Outputs

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
- [x] 1. 锁定当前截断边界并补齐失败测试
- [x] 2. 实现统一的 raw/inline/re-read/truncation metadata 合约
- [x] 3. 更新文档与回归验证并完成强审收口

## Item 1 - 锁定当前截断边界并补齐失败测试
### 约束
- 依赖事项：无
- 冲突事项：2, 3
- 风险等级：高
- 并行状态：串行

### 计划
- 盘点当前 pipe executor、runtime inline trimming、program/fanout binding、verifier、interactive/PTy view 的截断和读取路径。
- 按需求补齐失败测试，覆盖：
- pipe 结果在 inline budget 之外仍可恢复 raw/full 输出；
- downstream runtime consumer 在 correctness 路径上优先消费 raw；
- interactive / PTY read surface 暴露统一的 truncation metadata 与 continuation 信息。
- 先只新增测试和必要的测试夹具，不修改生产实现。

### 实施记录
- 已盘点当前链路：
- `pkg/harness/executor/shell/pipe.go` 仍在 executor 层直接截断 `stdout/stderr`，只有局部 metadata，没有统一 raw reread 合约。
- runtime 单步与 fanout 已有 raw-first 基础，但尚未为调用方暴露稳定 `raw_ref -> reread` 公共 API。
- `modules/shell` / `pkg/harness/runtime` 的 interactive/PTy view 仍只返回 `truncated + next_offset`，没有统一的 original/returned/has_more/raw_ref 形状。
- 引用文档 `docs/runbooks/truncation-map.md` 与 `docs/issue/harness-core-runtime-raw-vs-inline-results.md` 在当前仓库中不存在，改为以用户提供的说明和现有代码为准。

### 验证记录
- `go test ./pkg/harness/executor/shell -run 'TestPipeExecutor(PreservesRawOutputAndReportsRecoverablePreviewMetadata|TruncatesOutputAndReportsMetadata)$'`
- 结果：失败；`TestPipeExecutorPreservesRawOutputAndReportsRecoverablePreviewMetadata` 证明 `PipeExecutor.Execute` 仍只返回 inline `stdout="hello"`，`result.Raw == nil`，且没有标准化 `stdout_preview` metadata。
- `go test ./pkg/harness/runtime -run 'TestRunStepExposesRawArtifactReferenceAndRereadWindows$'`
- 结果：构建失败；当前 `action.Result` 缺少 `RawRef`，`runtime.Service` 缺少 `GetArtifact` / `ReadArtifact`，`ArtifactReadRequest` 也尚未定义，说明公共 raw reread 合约尚未暴露。
- `go test ./modules/shell -run 'TestInteractiveViewExposesRecoverablePreviewMetadata$'`
- 结果：构建失败；当前 `runtime.InteractiveViewResult` 缺少 `OriginalBytes`、`ReturnedBytes`、`HasMore`、`NextOffset`、`RawRef`，说明 PTY/interactive surface 还没有统一的 preview/reread metadata 合约。

### 审核记录
- Reviewer：`019d4777-3ea4-74b2-ad52-bcca46d122b0` (`Jason`)
- Reviewer 状态：`closed`
- 开始时间：`2026-04-01T05:20:11Z`
- 累计等待时长：`15min`
- 超时次数：`3`
- 审核轮次：`1`
- 审核结论：首次 reviewer 未返回结果，未形成审查结论。
- Replacement Reviewer：`019d4786-8f18-7c81-925d-dd0dc505ef3c` (`Maxwell`)
- 关闭状态：全部 reviewer 已关闭
- 关闭原因：原 reviewer 连续 3 次 `wait_agent` 超时后替换；replacement reviewer 审核通过后已正常关闭。
- 备注：replacement reviewer 结论：`无 findings`，确认当前红测与 checklist 记录足够支撑进入 Item 2，并明确 `Item 1 approved`。

## Item 2 - 实现统一的 raw/inline/re-read/truncation metadata 合约
### 约束
- 依赖事项：1
- 冲突事项：3
- 风险等级：高
- 并行状态：串行

### 计划
- 在 runtime/action/executor 层明确区分 raw/full 与 inline/preview 结果，并补充稳定的 truncation metadata。
- 修正 pipe execution 的早截断边界，保证 raw capture 先于 inline trimming。
- 为 correctness-sensitive runtime consumer 统一切换为 raw-first。
- 为大输出暴露稳定 raw reference / artifact id，并补齐 offset/cursor continuation 信息在可流式 surface 上的对齐。

### 实施记录
- `pkg/harness/action/spec.go` 为 `action.Result` 增加 `RawRef`，让 inline 结果在持久化后能指向 durable raw artifact。
- `pkg/harness/executor/shell/pipe.go` 改为先保留完整 `stdout/stderr`，再返回 preview；当 preview 被裁剪时，把完整 payload 放入 `Result.Raw`，并补充标准化 `stdout_preview` / `stderr_preview` metadata。
- `pkg/harness/runtime/runner.go` 与 `pkg/harness/runtime/fanout_scheduler.go` 在持久化 action-result artifact 后回填 `RawRef`，同时 artifact payload 继续使用 raw-first 内容。
- `pkg/harness/runtime/service.go`、`pkg/harness/runtime/service_reads.go`、`pkg/harness/runtime/artifact_reads.go` 新增 `GetArtifact` / `ReadArtifact` 公共 reread API，支持 `path + offset/max_bytes` 与 `path + line_offset/max_lines` 两种窗口读取，并返回统一 continuation metadata。
- `pkg/harness/runtime/interactive_control.go`、`modules/shell/pty_manager.go`、`modules/shell/interactive_controller.go` 为 interactive/PTy view 暴露 `original_bytes`、`returned_bytes`、`has_more`、`next_offset`、`raw_ref` 等 preview/reread metadata。
- `pkg/harness/runtime/program_interactive_actions.go` 把上述 interactive preview metadata 透传到 native interactive action 结果，避免 program 节点丢失 reread 线索。
- `pkg/harness/harness.go`、`pkg/harness/runtime/errors.go` 补充 facade re-export 与 artifact reread 错误类型，保证外部 embedding API 可直接使用。
- 回归过程中发现 `modules/shell` 需要继续兼容独立快照仓库 against 旧版 `pkg/harness/runtime` 的外部编译场景，因此 `interactive_controller.go` 保留反射式字段回填，而不是直接在 struct literal 中硬编码新字段。

### 验证记录
- `go test ./pkg/harness/executor/shell -run 'TestPipeExecutor(PreservesRawOutputAndReportsRecoverablePreviewMetadata|TruncatesOutputAndReportsMetadata)$'`
- `go test ./pkg/harness/runtime -run 'TestRunStep(ExposesRawArtifactReferenceAndRereadWindows|StoresRecoverableRawOutputWhenInlineTrimmed|VerificationUsesRawActionResultWhenInlineTrimmed)$'`
- `go test ./modules/shell -run 'TestInteractiveViewExposesRecoverablePreviewMetadata$'`
- `go test ./release -run 'TestExternalConsumersBuildAgainstSnapshotRepo/modules-shell-package$'`
- `go test ./pkg/harness/runtime`
- `go test ./modules/shell`
- `go test ./pkg/harness/executor/shell`
- 结果：全部通过。

### 审核记录
- Reviewer：`019d478c-2d34-7a90-8a31-6a3775ff75fa` (`Raman`)
- Reviewer 状态：`closed`
- 开始时间：`2026-04-01T05:37:57Z`
- 累计等待时长：`12min`
- 超时次数：`2`
- 审核轮次：`1`
- 审核结论：补充 verdict 后确认 `无 findings`，`Item 2 approved`。
- Replacement Reviewer：待填写
- 关闭状态：reviewer 已正常关闭
- 关闭原因：review 完成
- 备注：reviewer 首次回复未按要求给出显式 verdict；复问后补充 `无 findings` / `Item 2 approved`，因此本事项通过。

## Item 3 - 更新文档与回归验证并完成强审收口
### 约束
- 依赖事项：1, 2
- 冲突事项：无
- 风险等级：中
- 并行状态：串行

### 计划
- 更新运行时文档，明确 runtime 与 product semantic compaction 的边界、预算范围、以及 raw/inline 的公开契约。
- 运行 focused + full verification，并把结果写回 checklist。
- 对本事项启动独立 reviewer 做强审；若有问题则循环修复直到 reviewer 明确通过，再关闭 reviewer 并勾选事项。

### 实施记录
- `docs/RUNTIME.md` 增补 output boundary contract，明确 runtime 只负责 resource safety / preview budget / raw capture / reread，不负责 product-level semantic compaction。
- `docs/API.md`、`docs/EMBEDDING.md` 更新公开读 API 与 embedding 建议读取链路，补充 `GetArtifact` / `ReadArtifact`、`ActionResult.Raw` / `RawRef` 的使用边界。
- `docs/ROADMAP.md` 将 shell truncation 目标改写为 “bound preview + preserve raw”，避免文档继续暗示 destructive truncation。

### 验证记录
- `go test ./pkg/harness/executor/shell -run 'TestPipeExecutor(PreservesRawOutputAndReportsRecoverablePreviewMetadata|TruncatesOutputAndReportsMetadata)$'`
- `go test ./pkg/harness/runtime -run 'TestRunStep(ExposesRawArtifactReferenceAndRereadWindows|StoresRecoverableRawOutputWhenInlineTrimmed|VerificationUsesRawActionResultWhenInlineTrimmed)$'`
- `go test ./modules/shell -run 'TestInteractiveViewExposesRecoverablePreviewMetadata$'`
- `go test ./release -run 'TestExternalConsumersBuildAgainstSnapshotRepo/modules-shell-package$'`
- `go test ./release -run 'TestExternalConsumersBuildAgainstSnapshotRepo/(root-module|adapters-websocket-package)$'`
- `go test ./...`
- 结果：全部通过。

### 审核记录
- Reviewer：`019d4799-e42e-7d00-9878-58fa8934c36d` (`Aristotle`)
- Reviewer 状态：`closed`
- 开始时间：`2026-04-01T05:51:14Z`
- 累计等待时长：`<5min`
- 超时次数：`0`
- 审核轮次：`1`
- 审核结论：`无 findings`，`Item 3 approved`。
- Replacement Reviewer：待填写
- 关闭状态：reviewer 已正常关闭
- 关闭原因：review 完成
