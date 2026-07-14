# GPT-5.6 定价元数据上游对齐设计

## 背景

当前分支已经完成 GPT-5.6 运行时计费语义的移植，包括 Sol、Terra、Luna 官方标准价、cache write、priority service tier、长上下文倍率、usage 多字段解析，以及渠道或区间显式 cache write 价格保护。

本次获取 `upstream/main` 后，上游引用更新到 `551e2570d`。对本地 `origin/main` 与该引用中的三个 GPT-5.6 定价对象执行规范化逐字段比较，发现运行时使用的标准价、priority 价格和长上下文倍率已经一致，但原始定价资源仍有以下偏差：

- 本地缺少三个模型的 batch/flex cache write 字段。
- Terra 与 Luna 的 batch/flex 输入、输出和 cache read 价格与上游不一致。
- 本地保留了上游 GPT-5.6 对象中不存在的显式 `*_above_272k_tokens` 字段。

当前 `PricingService` 不解析 batch/flex 和显式 `above_272k` 字段，`BillingService` 通过标准价与 tier 倍率计算当前请求费用，因此这些偏差暂未改变现有运行时计费结果。但是，错误的原始元数据会给未来字段消费、定价导出或后续同步带来风险，仍应精确收口。

## 目标

以 `upstream/main@551e2570d` 为基线，语义对齐以下三个对象的定价元数据：

- `gpt-5.6-sol`
- `gpt-5.6-terra`
- `gpt-5.6-luna`

对齐完成后：

- 三个模型具备完整、正确的 batch/flex cache write 字段。
- Terra 与 Luna 的 batch/flex 定价不再错误复用或偏离官方 tier 价格。
- 长上下文只通过阈值与倍率字段表达，与上游保持一致。
- 现有标准、priority、flex 和长上下文运行时计费路径保持不变。
- 自动化测试能够阻止这些原始字段再次漂移。

## 方案选择

评估过三种方案：

1. 精准对齐三个 GPT-5.6 定价对象并增加原始字段回归测试。
2. 保持现状，只依赖当前运行时 tier 倍率得到正确金额。
3. 全量移植上游通用计费子系统及其无关变化。

采用方案 1。它能消除已经确认的元数据偏差，同时避免把 `TokenPricingAbsent`、图片计费保护和其他非 GPT-5.6 变化带入本次工作。

## 定价元数据

以下数值单位均为 USD/token。

| 价格字段 | Sol | Terra | Luna |
| --- | ---: | ---: | ---: |
| 标准输入 | `5e-6` | `2.5e-6` | `1e-6` |
| 标准输出 | `30e-6` | `15e-6` | `6e-6` |
| 标准 cache write | `6.25e-6` | `3.125e-6` | `1.25e-6` |
| 标准 cache read | `0.5e-6` | `0.25e-6` | `0.1e-6` |
| batch/flex 输入 | `2.5e-6` | `1.25e-6` | `0.5e-6` |
| batch/flex 输出 | `15e-6` | `7.5e-6` | `3e-6` |
| batch/flex cache write | `3.125e-6` | `1.5625e-6` | `0.625e-6` |
| flex cache read | `0.25e-6` | `0.125e-6` | `0.05e-6` |
| priority 输入 | `10e-6` | `5e-6` | `2e-6` |
| priority 输出 | `60e-6` | `30e-6` | `12e-6` |
| priority cache write | `12.5e-6` | `6.25e-6` | `2.5e-6` |
| priority cache read | `1e-6` | `0.5e-6` | `0.2e-6` |

三个模型继续保留相同的长上下文策略：

- `long_context_input_token_threshold`: `272000`
- `long_context_input_cost_multiplier`: `2`
- `long_context_output_cost_multiplier`: `1.5`

## 资源文件修改

修改 `backend/resources/model-pricing/model_prices_and_context_window.json` 中三个 GPT-5.6 对象：

- 增加 `cache_creation_input_token_cost_batches`。
- 增加 `cache_creation_input_token_cost_flex`。
- 将 `input_cost_per_token_batches`、`input_cost_per_token_flex`、`output_cost_per_token_batches`、`output_cost_per_token_flex` 和 `cache_read_input_token_cost_flex` 对齐到上游值。
- 删除 `input_cost_per_token_above_272k_tokens`、`output_cost_per_token_above_272k_tokens` 和 `cache_read_input_token_cost_above_272k_tokens`。
- 保留标准价、priority 价格、模型能力、上下文窗口、支持端点和长上下文倍率等已经一致的字段。

这里追求字段和值的语义一致，不要求 JSON 数字格式或对象行号与上游字节级一致。

## 运行时边界与数据流

数据流保持为：

1. 内置 JSON 由 `PricingService` 加载。
2. `PricingService` 继续解析标准、priority、cache 和长上下文倍率字段。
3. `BillingService` 继续使用显式 priority 价格，或按现有规则对 flex 应用 tier 倍率。
4. 长上下文继续按 `InputTokens + CacheCreationTokens + CacheReadTokens > 272000` 触发输入侧 `2x` 和输出侧 `1.5x`。

本次不扩展 `LiteLLMModelPricing` 或 `LiteLLMRawEntry` 去消费 batch/flex 字段，也不改变 service tier 选择和费用计算。这样可以让资源与上游对齐，同时保持当前已经验证过的运行时行为。

## 错误处理

- JSON 语法错误继续由现有价格加载逻辑返回错误。
- 新增测试把字段缺失、值不一致或废弃字段重新出现视为失败。
- 本次不增加运行时 fallback，也不改变价格缺失时的现有错误路径。
- 不从网络实时读取上游价格，避免测试结果受外部仓库状态影响。

## 测试设计

在 `backend/internal/service/pricing_service_test.go` 增加原始定价资源回归测试。测试直接读取内置 JSON，并使用只覆盖本次字段的窄结构或原始键映射进行断言。

测试覆盖：

- 三个模型的 batch/flex 输入与输出价格。
- 三个模型的 batch/flex cache write 价格。
- 三个模型的 flex cache read 价格。
- 标准与 priority 关键价格仍保持官方值。
- 长上下文阈值和倍率保持 `272000`、`2`、`1.5`。
- 三个对象均不再包含 `input_cost_per_token_above_272k_tokens`、`output_cost_per_token_above_272k_tokens` 和 `cache_read_input_token_cost_above_272k_tokens`。

保留并执行现有 GPT-5.6 测试，继续覆盖：

- 官方 fallback 价格。
- priority cache write 计费。
- 动态价格缺失 cache write 时的 `1.25x` 策略。
- 显式 cache write 零价格保护。
- 长上下文 token 合计和阈值边界。
- usage 中 cache write token 的提取与计费。

建议验证命令：

```bash
cd backend
go test -tags unit ./internal/service -run 'GPT56|DefaultPricing.*GPT56|PricingMetadata'
```

另外对三个对象执行本地与固定上游引用的规范化比较，作为实施期间的人工验证。该比较不进入自动化测试。

## 验收标准

- 三个 GPT-5.6 定价对象与 `upstream/main@551e2570d` 在目标字段和值上语义一致。
- Terra 与 Luna 的 batch/flex 输入、输出、cache write 和 cache read 价格符合上游。
- 三个对象不再携带上游已移除的显式 `above_272k` 字段。
- 现有运行时计费代码无改动。
- 新增元数据测试和现有 GPT-5.6 定向测试通过。
- 实现 diff 只包含定价 JSON、对应后端测试以及本流程生成的设计和计划文档。

## 非目标

- 不全量合并 `upstream/main`。
- 不修改 `BillingService`、`PricingService`、usage 解析或模型别名逻辑。
- 不让运行时直接消费 batch/flex 原始字段。
- 不引入 `TokenPricingAbsent` 或图片计费相关通用改动。
- 不修改其他 OpenAI、Claude、Gemini、Grok 或图片模型价格。
- 不修改前端模型配置或前端测试。

## 风险与缓解

- 风险：手工录入小数时出现数量级错误。缓解：直接依据固定上游引用，并用表驱动测试逐字段断言。
- 风险：误改 GPT-5.6 之外的大型 JSON 内容。缓解：实现后检查目标对象规范化 diff 和 Git 文件级 diff。
- 风险：把元数据对齐误扩展为运行时重构。缓解：明确禁止修改计费服务结构和数据流，仅触达资源与测试。
- 风险：本地首次测试受 Go 依赖下载阻塞。缓解：保留明确的定向命令，区分依赖环境失败与测试断言失败，并在依赖可用后重新执行。
