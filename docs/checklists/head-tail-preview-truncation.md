# Head-Tail Preview Truncation

## 模式
- 强审开发模式

## 审核设置
- 审核模型目标：gpt-5.4
- 推理强度目标：xhigh
- 活跃事项上限：1
- 首次等待窗口：5min
- 二次探测窗口：5-10min
- 硬超时门槛：15min

## 当前执行状态
- 当前状态：已完成
- 当前活跃事项：无
- 当前活跃 reviewer：无
- 当前阻塞原因：无
- 下一动作：无

## Checklist
- [x] 1. 锁定 head-tail preview 行为并补齐失败测试
- [x] 2. 实现统一 head-tail preview helper，并接入 pipe executor / runtime action result trimming
- [x] 3. 接入 planner inline projection 与文档，明确 exact window 契约不变
- [x] 4. 完成验证、强审 reviewer、收口并勾选

## Item 1 - 锁定 head-tail preview 行为并补齐失败测试
### 约束
- 依赖事项：无
- 冲突事项：2, 3, 4
- 风险等级：高
- 并行状态：串行
- 当前状态：进行中
- 阻塞原因：无
- 下一动作：新增失败测试，覆盖 preview middle-elision 与 exact window 不变

### 计划
- 盘点当前仍为前缀截断的 3 个路径：
- `pkg/harness/executor/shell/pipe.go`
- `pkg/harness/runtime/runner.go`
- `pkg/harness/runtime/planner_projection.go`
- 先补失败测试，覆盖：
- pipe preview 超限时保留头尾并在中间插入截断标记；
- runtime `action.Result` 的 inline trimming 对字符串字段使用 head-tail preview，而不是纯前缀；
- planner `inline` projection 超限时保留头尾，而不是纯前缀；
- `ArtifactRead` / PTY exact window 语义不变，不引入 head-tail preview。
- 只新增测试与必要测试辅助，不写生产实现。

### 实施记录
- 已新增失败测试：
- `pkg/harness/executor/shell/pipe_test.go`
- `TestPipeExecutorUsesHeadTailPreviewWhenOutputIsTruncated`
- `TestPipeExecutorReportsHeadTailPreviewMetadataWhenOutputIsTruncated`
- `TestPipeExecutorHeadTailPreviewRemainsUTF8Safe`
- `pkg/harness/runtime/runtime_policy_cleanup_test.go`
- `TestProjectPlannerContextInlineProjectionUsesHeadTailPreview`
- `TestRunStepAppliesHeadTailPreviewToInlineResultStrings`
- `TestRunStepAppliesHeadTailPreviewToInlineMetaAndErrorStrings`
- `TestRunStepKeepsUTF8ValidWhenInlinePreviewTruncatesMetaAndErrorStrings`
- 第 2 轮 reviewer 补充问题后，继续加严约束：
- pipe UTF-8 preview 现在同时锁定 byte budget、middle-elision 和 UTF-8 head/tail；
- runtime UTF-8 preview 现在同时覆盖 `Data["stdout"]` / `Meta["note"]` / `Error.Message`；
- runtime UTF-8 preview 现在同时锁定 middle-elision 与 `Inline.MaxChars` 预算。
- planner UTF-8 inline projection 现在也新增独立失败测试，锁定 valid UTF-8、middle-elision、首/尾 rune 保留与 `MaxChars` 预算。
- exact window 不新增新实现测试，沿用已有回归作为防护：
- `pkg/harness/runtime/execution_records_test.go`
- `modules/shell/pty_test.go`

### 验证记录
- `cd /usr/local/src/project/harness-core && go test ./pkg/harness/executor/shell ./pkg/harness/runtime -run 'Test(PipeExecutorUsesHeadTailPreviewWhenOutputIsTruncated|ProjectPlannerContextInlineProjectionUsesHeadTailPreview|RunStepAppliesHeadTailPreviewToInlineResultStrings)$'`
- 结果：失败，且失败点符合预期：
- `PipeExecutorUsesHeadTailPreviewWhenOutputIsTruncated`: 当前仍返回前缀 `"abcdefghijklm"`
- `ProjectPlannerContextInlineProjectionUsesHeadTailPreview`: 当前仍返回前缀 `"abcdefghijklm"`
- `RunStepAppliesHeadTailPreviewToInlineResultStrings`: 当前 action preview 仍返回前缀 `"abcdefghijklm"`
- `cd /usr/local/src/project/harness-core && go test ./pkg/harness/executor/shell ./pkg/harness/runtime -run 'Test(PipeExecutorUsesHeadTailPreviewWhenOutputIsTruncated|PipeExecutorReportsHeadTailPreviewMetadataWhenOutputIsTruncated|PipeExecutorHeadTailPreviewRemainsUTF8Safe|ProjectPlannerContextInlineProjectionUsesHeadTailPreview|RunStepAppliesHeadTailPreviewToInlineResultStrings|RunStepAppliesHeadTailPreviewToInlineMetaAndErrorStrings|RunStepKeepsUTF8ValidWhenInlinePreviewTruncatesMetaAndErrorStrings)$'`
- 结果：失败，且失败点符合预期：
- pipe preview 仍返回前缀，不保留尾部；
- pipe preview metadata 仍是旧的 prefix-window 形状，没有 `preview_mode/head_bytes/tail_bytes/elided_bytes`；
- pipe UTF-8 case 没有 middle elision；
- planner inline projection 仍返回前缀；
- runtime action `Data.stdout` / `Meta.note` / `Error.Message` 仍返回前缀；
- runtime action 对非 ASCII `Meta.note` 仍然发生无效 UTF-8 截断（当前值为 `"世界\\xe4"`）。
- `cd /usr/local/src/project/harness-core && go test ./pkg/harness/executor/shell ./pkg/harness/runtime -run 'Test(PipeExecutorUsesHeadTailPreviewWhenOutputIsTruncated|PipeExecutorReportsHeadTailPreviewMetadataWhenOutputIsTruncated|PipeExecutorHeadTailPreviewRemainsUTF8Safe|ProjectPlannerContextInlineProjectionUsesHeadTailPreview|RunStepAppliesHeadTailPreviewToInlineResultStrings|RunStepAppliesHeadTailPreviewToInlineMetaAndErrorStrings|RunStepKeepsUTF8ValidWhenInlinePreviewTruncatesMetaAndErrorStrings)$'`
- 结果：失败，且失败点符合更新后的预期：
- pipe UTF-8 preview 仍未使用 middle-elision，不能满足新的 UTF-8 head/tail + byte budget 断言；
- planner/runtime ASCII preview 仍保持旧的前缀截断行为；
- runtime UTF-8 `Data["stdout"]` / `Meta["note"]` / `Error.Message` 仍未使用 head-tail preview，错误值仍表现为前缀截断与非法 UTF-8（当前为 `"世界\\xe4"`）。
- planner UTF-8 inline projection 新增后，当前实现仍会表现为 prefix-only / 非 head-tail，符合预期红测方向。

### 审核记录
- Reviewer：`019d491e-1c57-72b1-b523-598c279c563a` (`Gibbs`)
- Reviewer 状态：`completed`
- 开始时间：`2026-04-01T13:02:58Z`
- 累计等待时长：`5min+`
- 超时次数：`1`
- 审核轮次：`1`
- 审核结论：
- 第 1 轮 reviewer 发现 `1` 个 high + `2` 个 medium：
- `pipe_test.go` 只锁定了渲染字符串，没有锁定 preview metadata 新契约，后续实现会被旧绿测反卡；
- runtime 路径只覆盖 `Data["stdout"]`，没有覆盖 `Meta` / `Error.Message`；
- ASCII-only 用例不足以锁定 pipe/runtime 的 UTF-8 安全要求。
- 已根据 reviewer 结论补充对应失败测试，并发起第 2 轮复审。
- 第 2 轮 reviewer 发现 `1` 个 high + `2` 个 medium：
- runtime UTF-8 case 仍未证明发生 middle-elision 且未超 `Inline.MaxChars` 预算；
- pipe UTF-8 case 仍未锁定 byte budget；
- runtime UTF-8 `Data["stdout"]` 仍未覆盖。
- 已根据 reviewer 结论继续补充失败测试，待第 3 轮复审。
- 第 3 轮 reviewer 发现 `1` 个 medium：
- planner UTF-8 inline projection 仍未锁定 head-tail 行为，现有 planner UTF-8 测试仍只覆盖 prefix-only 小预算路径。
- 已根据 reviewer 结论补充 planner UTF-8 失败测试，待第 4 轮复审。
- 第 4 轮 reviewer 结论：`Item 1 approved`，无新增问题。
- Replacement Reviewer：待填写
- 关闭状态：已关闭
- 关闭原因：Item 1 通过后按强审规则关闭 reviewer

## Item 2 - 实现统一 head-tail preview helper，并接入 pipe executor / runtime action result trimming
### 约束
- 依赖事项：1
- 冲突事项：3, 4
- 风险等级：高
- 并行状态：串行
- 当前状态：进行中
- 阻塞原因：无
- 下一动作：发起 Item 2 reviewer，确认 helper/pipe/runtime 接入没有回归

### 计划
- 新增统一 preview helper，默认策略为：
- 头 40%
- 尾 60%
- 中间明确省略标记
- helper 必须满足：
- UTF-8 安全，不能切坏 rune；
- 有统一 preview metadata，可表达 head/tail/elided，而不是伪装成连续前缀窗口；
- 当原文未超限时保持原样。
- 接入：
- `pkg/harness/executor/shell/pipe.go`
- `pkg/harness/runtime/action_result_channels.go`
- `pkg/harness/runtime/runner.go`
- 保证 raw/raw-handle/re-read 语义不退化。
- 实施中发现 planner inline projection 与 pipe/runtime 共用同一预览策略边界；为避免 helper 落地后再二次拆改，允许将 planner 代码接入前移到本项实现，Item 3 主要收口文档与契约说明。

### 实施记录
- 新增 `pkg/harness/preview/head_tail.go`：
- 提供 `TruncateHeadTailBytes` / `TruncateHeadTailChars` 两个统一 helper；
- 使用 `...` 作为 middle-elision marker；
- 默认按 4:6 分配 head/tail 预算；
- 保证 UTF-8 rune-safe；预算过小无法稳定表达 head-tail 时回退为 rune-safe prefix fallback。
- 更新 `pkg/harness/executor/shell/pipe.go`：
- pipe preview 改为统一 byte-budget head-tail helper；
- `stdout_preview` / `stderr_preview` metadata 新增 `preview_mode/head_bytes/tail_bytes/elided_bytes`；
- 保留 `returned_bytes/next_offset/has_more` 以兼容现有 raw reread 恢复路径。
- 更新 `pkg/harness/runtime/runner.go`：
- inline `Data` / `Meta` / `Error.Message` 全部改走 char-budget head-tail helper；
- 消除了对 `Error.Message` 的 byte-slice 截断。
- 提前接入 `pkg/harness/runtime/planner_projection.go`：
- planner inline projection 改为同一 head-tail helper；
- 超预算仍按 planner 语义耗尽 remaining，不改变 exact window reread 契约。
- 同步修正仍断言 prefix-only 的旧测试：
- `pkg/harness/executor/shell/pipe_test.go`
- `pkg/harness/runtime/execution_records_test.go`
- `pkg/harness/runtime/runtime_policy_cleanup_test.go`

### 验证记录
- `cd /usr/local/src/project/harness-core && go test ./pkg/harness/executor/shell ./pkg/harness/runtime -run 'Test(PipeExecutorTruncatesOutputAndReportsMetadata|PipeExecutorPreservesRawOutputAndReportsRecoverablePreviewMetadata|PipeExecutorUsesHeadTailPreviewWhenOutputIsTruncated|PipeExecutorReportsHeadTailPreviewMetadataWhenOutputIsTruncated|PipeExecutorHeadTailPreviewRemainsUTF8Safe|ProjectPlannerContextInlineProjectionIsDeterministicAndRuneSafe|ProjectPlannerContextInlineProjectionUsesHeadTailPreview|ProjectPlannerContextInlineProjectionUsesHeadTailPreviewForUTF8|RunStepAppliesHeadTailPreviewToInlineResultStrings|RunStepAppliesHeadTailPreviewToInlineMetaAndErrorStrings|RunStepKeepsUTF8ValidWhenInlinePreviewTruncatesMetaAndErrorStrings)$'`
- 结果：通过。
- `cd /usr/local/src/project/harness-core && go test ./pkg/harness/executor/shell ./pkg/harness/runtime ./modules/shell`
- 结果：通过。
- 根据 reviewer 意见新增/收紧 metadata 契约后再次验证：
- `cd /usr/local/src/project/harness-core && go test ./pkg/harness/executor/shell -run 'Test(PipeExecutorPreservesRawOutputAndReportsRecoverablePreviewMetadata|PipeExecutorReportsHeadTailPreviewMetadataWhenOutputIsTruncated|PipeExecutorReportsPrefixPreviewMetadataWhenTailCannotBePreserved)$'`
- 结果：通过。
- `cd /usr/local/src/project/harness-core && go test ./pkg/harness/executor/shell ./pkg/harness/runtime ./modules/shell`
- 结果：通过。

### 审核记录
- Reviewer：`019d493c-c82d-7cf1-a6c5-2eb5ecd862e7` (`Carson`)
- Reviewer 状态：`completed`
- 开始时间：`2026-04-01T15:40:00Z`
- 累计等待时长：`5min+`
- 超时次数：`0`
- 审核轮次：`1`
- 审核结论：
- 第 1 轮 reviewer 发现 `2` 个 medium：
- head-tail pipe preview 仍暴露了易误导为连续前缀窗口的 `next_offset`；
- 极小预算 fallback 虽然已退化为 prefix preview，但 metadata 仍错误标记为 `head_tail`。
- 已新增对应失败测试并修复：
- head-tail preview metadata 现在不再暴露 `next_offset`；
- fallback preview metadata 现在明确标记为 `preview_mode=prefix`，并仅在该模式下保留 `next_offset`。
- 已发起同 reviewer 第 2 轮复审。
- 第 2 轮 reviewer 结论：`Item 2 approved`。
- Replacement Reviewer：待填写
- 关闭状态：已关闭
- 关闭原因：Item 2 通过后按强审规则关闭 reviewer

## Item 3 - 接入 planner inline projection 与文档，明确 exact window 契约不变
### 约束
- 依赖事项：2
- 冲突事项：4
- 风险等级：中
- 并行状态：串行
- 当前状态：进行中
- 阻塞原因：无
- 下一动作：待 Item 2 通过后整理本项实施/验证并发起独立 reviewer

### 计划
- Item 2 已提前完成 planner `inline` projection 代码接入；本项聚焦文档与契约澄清。
- 更新文档，明确区分：
- preview truncation：head-tail middle elision
- exact window reread：精确 offset/line window，不做 head-tail preview
- 目标文档：
- `docs/RUNTIME.md`
- `docs/API.md`
- `modules/shell/README.md`

### 实施记录
- 已更新文档：
- `docs/RUNTIME.md`
- `docs/API.md`
- `modules/shell/README.md`
- 文档现在明确：
- preview truncation 采用 head-tail middle-elision；
- head-tail preview 不再暴露 prefix-style `next_offset`；
- `ReadArtifact` / `ViewInteractive` / verifier offset 仍然是 exact-window reread 契约。

### 验证记录
- 文档变更与当前实现一致性已人工对照：
- `docs/RUNTIME.md`
- `docs/API.md`
- `modules/shell/README.md`
- 对照结果：
- head-tail preview 与 prefix fallback 的 metadata 行为说明已与 `pipe.go` 当前实现对齐；
- exact-window reread 与 `ReadArtifact` / `ViewInteractive` 契约说明未被误写成 preview 语义。

### 审核记录
- Reviewer：`019d494f-4fec-7ed0-85d6-415fda86a8b7` (`Zeno`)
- Reviewer 状态：`completed`
- 开始时间：`2026-04-01T15:55:00Z`
- 累计等待时长：`10min+`
- 超时次数：`1`
- 审核轮次：`1`
- 审核结论：`Item 3 approved`
- Replacement Reviewer：待填写
- 关闭状态：已关闭
- 关闭原因：Item 3 通过后按强审规则关闭 reviewer

## Item 4 - 完成验证、强审 reviewer、收口并勾选
### 约束
- 依赖事项：1, 2, 3
- 冲突事项：无
- 风险等级：中
- 并行状态：串行
- 当前状态：进行中
- 阻塞原因：无
- 下一动作：发起 Item 4 reviewer，完成最终收口并勾选

### 计划
- 运行 focused tests 和相关 package 全量测试。
- 为每个事项分别发起 reviewer 审核；若有问题则修复并复审，直到 reviewer 明确通过。
- 关闭所有 reviewer 后再逐项勾选，并更新当前执行状态。
- 目标验证：
- `go test ./pkg/harness/executor/shell ./pkg/harness/runtime ./modules/shell`
- 必要时补跑 `go test ./...`
- `git diff --check`

### 实施记录
- 汇总并确认本次改动最终范围：
- 统一 preview helper：`pkg/harness/preview/head_tail.go`
- pipe preview 接入与 metadata 契约修正：`pkg/harness/executor/shell/pipe.go`
- runtime inline preview 接入：`pkg/harness/runtime/runner.go`
- planner inline projection 接入：`pkg/harness/runtime/planner_projection.go`
- 测试与文档同步更新：
- `pkg/harness/executor/shell/pipe_test.go`
- `pkg/harness/runtime/execution_records_test.go`
- `pkg/harness/runtime/runtime_policy_cleanup_test.go`
- `docs/RUNTIME.md`
- `docs/API.md`
- `modules/shell/README.md`
- `docs/checklists/head-tail-preview-truncation.md`

### 验证记录
- `cd /usr/local/src/project/harness-core && go test ./pkg/harness/preview ./pkg/harness/executor/shell ./pkg/harness/runtime ./modules/shell`
- 结果：通过。
- `cd /usr/local/src/project/harness-core && git diff --check`
- 结果：通过。

### 审核记录
- Reviewer：`019d4957-ac6f-7042-984f-741e825a5fda` (`Kepler`)
- Reviewer 状态：`completed`
- 开始时间：`2026-04-01T16:10:00Z`
- 累计等待时长：`10min+`
- 超时次数：`1`
- 审核轮次：`1`
- 审核结论：`Item 4 approved`
- Replacement Reviewer：待填写
- 关闭状态：已关闭
- 关闭原因：Item 4 通过后按强审规则关闭 reviewer
