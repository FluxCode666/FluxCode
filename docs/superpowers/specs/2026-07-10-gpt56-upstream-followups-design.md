# GPT-5.6 上游后续补丁精准移植设计

## 背景

当前工作树位于 `c88a8bb5a`，已包含首轮 GPT-5.6 支持：`gpt-5.6-sol`、`gpt-5.6-terra`、`gpt-5.6-luna` 已进入模型列表、基础归一化、静态 fallback、前端白名单和 OpenCode 配置。

本次拉取 `upstream/main` 后，上游最新引用为 `5260a42a0`。在首轮提交 `6cea1c35b` 之后，上游又合入了多条 GPT-5.6 相关修复：

- `4a2b10c94`：支持 GPT-5.6 cache write 计费。
- `383f61d0e`：将 GPT-5.6 对齐官方定价。
- `062af81fb`：保留渠道显式配置的 GPT-5.6 cache write 价格。
- `657c4f97d`：升级 Codex 客户端版本到 `0.144.1`，修复 `gpt-5.6-luna` 404。
- `80b3d4c1f`：兼容 GPT-5.6 `max` 推理强度。
- `c3ae5fc3c`：用模型候选列表提取 reasoning effort，避免后缀模型元数据丢失。
- `de28eba3c`：加固 GPT-5.6 billing 与 usage，补裸 `gpt-5.6`、cache usage 字段解析和前端 `max` 变体。

当前分支和上游结构差异较大。直接合并 `upstream/main` 会带入大量无关网关、后台、前端和迁移改动，因此本次采用语义级精准移植。

## 目标

精准补齐 GPT-5.6 后续行为：

- GPT-5.6 三个变体使用官方 Sol/Terra/Luna 定价，不再静态回退到 GPT-5.4。
- 动态定价资源包含 GPT-5.6 cache write、priority cache write、长上下文阈值与倍数。
- billing 使用 cache write token，且能识别上游多种 usage 字段命名。
- 渠道或区间显式配置的 `CacheWritePrice` 不被 GPT-5.6 自动 cache write 策略覆盖。
- `gpt-5.6` 裸模型作为 Sol 别名处理。
- GPT-5.6 支持 `max` reasoning effort；OpenAI OAuth compact 子请求仍按上游兼容要求将 `max` 降级为 `xhigh`。
- reasoning effort 元数据从模型候选列表提取，避免映射后模型剥掉后缀导致统计缺失。
- Codex CLI 默认 User-Agent 和 compact `Version` 升级到上游修复版本。
- 前端白名单、预设映射和 OpenCode 生成配置暴露裸 `gpt-5.6` 与 GPT-5.6 `max` 变体。

## 非目标

- 不合并整个 `upstream/main`。
- 不引入上游无关的大文件拆分或网关重构。
- 不改变 Claude、Gemini、Grok、图片生成、支付、推广、账号管理等无关行为。
- 不修改当前分支的默认测试模型。
- 不让未知 `gpt-5.6-*` 任意型号自动套用 GPT-5.6 或 GPT-5.4 静态价格。

## 方案

采用“精准语义移植”：

1. 以当前分支现有文件结构为主，把上游 GPT-5.6 行为移植进已有模块。
2. 新增一个窄 helper 文件 `backend/internal/service/openai_gpt56_alias.go` 承载 GPT-5.6 判定与别名归一化，不引入完整上游 `openai_model_alias.go` 重构。
3. 对所有触达行为补定向回归测试。
4. 验证通过后再进入业务代码提交。

## 后端设计

### 模型归一化

当前分支的模型归一化主要在 `backend/internal/service/openai_codex_transform.go` 的 `normalizeCodexModel`。实现时新增 `backend/internal/service/openai_gpt56_alias.go`，让 `normalizeCodexModel`、PricingService、BillingService 和 reasoning effort 共享同一套 GPT-5.6 判定规则：

- `gpt-5.6` 归一到 `gpt-5.6-sol`。
- `gpt-5.6-max`、`gpt-5.6-low`、`gpt-5.6-medium`、`gpt-5.6-high`、`gpt-5.6-xhigh` 等已知 effort 后缀归一到 `gpt-5.6-sol`。
- `gpt-5.6-sol-*`、`gpt-5.6-terra-*`、`gpt-5.6-luna-*` 归一到对应基础模型。
- `openai/gpt-5.6-sol`、空格分隔和下划线分隔写法继续兼容。
- 未知 `gpt-5.6-foo` 不应被错误归一到 Sol。

该 helper 的范围只包含 GPT-5.6 和现有 GPT-5/Codex 归一化复用，不迁入上游完整 alias 层。

### 默认模型列表

在 `backend/internal/pkg/openai/constants.go` 中补裸模型：

- `gpt-5.6`，显示名 `GPT-5.6 (Sol)`。

保留当前已有的：

- `gpt-5.6-sol`
- `gpt-5.6-terra`
- `gpt-5.6-luna`

### Codex 客户端版本

将当前默认 Codex 客户端标识升级到上游修复版本：

- Codex CLI User-Agent：使用 `0.144.1` 版本字符串，并保留真实客户端形态中的 OS、架构和终端后缀。
- compact `Version`：升级为 `0.144.1`。
- 账号使用量探测版本常量同步到 `0.144.1`。

如果当前分支存在 DB 可配置项，默认值升级不应覆盖管理员已保存的自定义配置。

### 动态定价资源

更新 `backend/resources/model-pricing/model_prices_and_context_window.json` 中三个 GPT-5.6 条目：

- `gpt-5.6-sol`
  - input: `5e-6`
  - output: `30e-6`
  - cache write: `6.25e-6`
  - cache read: `0.5e-6`
  - priority input/output/cache write/cache read: `10e-6`、`60e-6`、`12.5e-6`、`1e-6`
- `gpt-5.6-terra`
  - input: `2.5e-6`
  - output: `15e-6`
  - cache write: `3.125e-6`
  - cache read: `0.25e-6`
  - priority input/output/cache write/cache read: `5e-6`、`30e-6`、`6.25e-6`、`0.5e-6`
- `gpt-5.6-luna`
  - input: `1e-6`
  - output: `6e-6`
  - cache write: `1.25e-6`
  - cache read: `0.1e-6`
  - priority input/output/cache write/cache read: `2e-6`、`12e-6`、`2.5e-6`、`0.2e-6`

三个条目都包含：

- `long_context_input_token_threshold`: `272000`
- `long_context_input_cost_multiplier`: `2`
- `long_context_output_cost_multiplier`: `1.5`
- `supports_service_tier`: `true`
- `supports_prompt_caching`: `true`

裸 `gpt-5.6` 不需要新增资源条目；定价查询通过归一化命中 Sol。

### PricingService

在 `backend/internal/service/pricing_service.go` 中补齐结构字段和 fallback：

- `LiteLLMModelPricing` 与 `LiteLLMRawEntry` 增加 `cache_creation_input_token_cost_priority`。
- 解析 JSON 时读取 priority cache write 字段。
- 静态 fallback 新增 Sol/Terra/Luna 三个官方价格对象，不再复用 `openAIGPT54FallbackPricing`。
- `matchOpenAIModel` 对 `gpt-5.6-sol*`、`gpt-5.6-terra*`、`gpt-5.6-luna*` 返回各自 fallback。
- 裸 `gpt-5.6` 和已知 effort 后缀应通过归一化走 Sol。
- 未知 `gpt-5.6-foo` 返回 `nil` 或继续走现有错误路径，不命中 GPT-5.4。

### BillingService

在 `backend/internal/service/billing_service.go` 中补齐 token 计费能力：

- `ModelPricing` 增加 `CacheCreationPricePerTokenPriority`。
- `ModelPricing` 增加 `CacheCreationPriceExplicit`，用于标记渠道或区间定价显式设置过 cache write。
- `usePriorityServiceTierPricing` 将 priority cache write 纳入判定。
- `GetModelPricing` 从 LiteLLM 数据构造 `ModelPricing` 时带上 priority cache write。
- GPT-5.6 fallback 改为官方价格对象，包含 cache write、priority cache write 和长上下文策略。
- `applyModelSpecificPricingPolicy` 对 GPT-5.6 缺失 cache write 的动态价格按输入价 `1.25x` 补齐，但只在 `CacheCreationPriceExplicit == false` 时生效。
- 长上下文阈值判断包含 `InputTokens + CacheCreationTokens + CacheReadTokens`。
- priority 计费下 cache write 使用 priority cache write 单价。

渠道和区间定价覆盖时：

- 如果 `CacheWritePrice` 非 nil，设置 `CacheCreationPriceExplicit = true`。
- 即使显式值为 `0`，也不能被 GPT-5.6 自动补成 `input * 1.25`。

### Usage 解析

当前分支的 usage 解析只读取 `cached_tokens` 和部分基础字段。实现时应让 Responses、Chat Completions、WS V2 等路径统一识别：

- `cache_creation_input_tokens`
- `cache_write_input_tokens`
- `cache_creation_tokens`
- `cache_write_tokens`
- `input_tokens_details.cache_creation_tokens`
- `input_tokens_details.cache_write_tokens`
- `prompt_tokens_details.cache_creation_tokens`
- `prompt_tokens_details.cache_write_tokens`

读取优先级应优先使用明确的 nested `cache_write_tokens` / `cache_creation_tokens` 字段，缺失时再看顶层兼容字段。负值按 `0` 处理。

`backend/internal/pkg/apicompat/types.go` 中的 Responses/Chat usage 类型也应补 cache write 字段，保证协议转换路径不会丢字段。

### Reasoning Effort

补齐 GPT-5.6 `max` 兼容：

- `normalizeOpenAIReasoningEffortForModel(raw, model)`：当 `raw == "max"` 且模型属于 GPT-5.6 时返回 `max`；其他模型仍沿用现有 normalization。
- OpenAI OAuth compact 路径中，如果有效模型属于 GPT-5.6 且请求 `reasoning.effort == "max"`，发送上游前降级为 `xhigh`。
- 非 compact、API Key、普通 Responses 和其他平台不做该降级。

补齐模型候选提取：

- `extractOpenAIReasoningEffortFromBody` 与 `extractOpenAIReasoningEffort` 接收模型候选列表，例如 `upstreamModel, billingModel, originalModel`。
- 显式 effort 的 `max` 判定使用第一个非空候选。
- body 未携带 effort 时，依次从候选模型后缀推导，避免映射后模型剥掉 `-xhigh`、`-max` 等后缀导致 usage 元数据为空。

## 前端设计

### 模型白名单

在 `frontend/src/composables/useModelWhitelist.ts` 中：

- OpenAI 模型列表增加 `gpt-5.6`。
- OpenAI 预设映射增加 `GPT-5.6`，自映射到 `gpt-5.6`。
- 保留已有 Sol/Terra/Luna 三个预设。

### UseKeyModal

在 `frontend/src/components/keys/UseKeyModal.vue` 的 OpenCode 模型配置中：

- 增加 `gpt-5.6`，显示名 `GPT-5.6 (Sol)`。
- GPT-5.6 系列变体均增加 `max`。
- Sol/Terra/Luna 的 context 和 output 继续使用 `1050000` 与 `128000`。
- `store` 继续为 `false`。

## 数据流

请求进入网关后：

1. 原始模型名先经过账号映射，得到 billing/effective model。
2. OpenAI/Codex 模型归一化将裸 `gpt-5.6` 和已知 GPT-5.6 suffix 映射到正确基础模型。
3. 上游请求使用归一化后的模型和兼容后的 reasoning effort。
4. 上游响应 usage 解析为 `OpenAIUsage`，包括 cache write tokens。
5. Billing 根据动态定价、渠道定价或静态 fallback 计算 input/output/cache write/cache read 费用。
6. Usage log 记录 requested/upstream/billing 模型、tokens、reasoning effort 和费用。

## 错误处理

- 动态定价缺失时，已知 GPT-5.6 模型使用对应官方静态 fallback。
- 未知 `gpt-5.6-*` 不自动回退到 Sol/Terra/Luna 或 GPT-5.4，避免误计价。
- usage 字段缺失时按 `0` 处理，不阻断请求。
- usage 字段为负数时按 `0` 处理。
- compact 路径的 `max -> xhigh` 转换失败时返回现有请求体序列化错误，避免静默发送不兼容 payload。
- 管理员显式配置的 cache write 价格优先于模型策略。

## 测试计划

后端测试：

- `backend/internal/pkg/openai/constants_test.go`：断言默认模型包含 `gpt-5.6` 和三个变体。
- `backend/internal/service/openai_codex_transform_test.go` 或相邻测试：覆盖裸 `gpt-5.6`、`gpt-5.6-max`、Sol/Terra/Luna suffix、provider 前缀、未知 `gpt-5.6-foo`。
- `backend/internal/service/pricing_service_test.go`：覆盖三模型官方 fallback、priority cache write、未知 GPT-5.6 不误回退。
- `backend/internal/service/billing_service_test.go`：覆盖官方价格、cache write 费用、priority cache write、长上下文 token 统计包含 cache write。
- `backend/internal/service/model_pricing_resolver_test.go`：覆盖显式 `CacheWritePrice=0` 和区间 `CacheWritePrice` 不被 GPT-5.6 策略覆盖。
- OpenAI gateway usage 测试：覆盖 nested 与 top-level cache write 字段解析。
- Reasoning effort 测试：覆盖 GPT-5.6 `max` 保留、compact 降级、模型候选后缀推导。
- Codex 版本测试：断言默认 UA/version 和探测版本为 `0.144.1`，同时保留配置覆盖行为。

前端测试：

- `frontend/src/composables/__tests__/useModelWhitelist.spec.ts`：断言包含 `gpt-5.6` 和三个变体，预设映射正确。
- `frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`：断言 OpenCode config 包含裸 `gpt-5.6`，GPT-5.6 系列含 `max` variant。

建议验证命令：

- `cd backend && go test -tags unit ./internal/pkg/openai ./internal/service ./internal/pkg/apicompat`
- `pnpm --dir frontend test -- useModelWhitelist UseKeyModal`

如果本地脚本或 build tags 与上述命令不完全一致，以仓库现有 package scripts 和 Go 测试约定为准，保持测试范围聚焦本次触达模块。

## 风险与缓解

- 价格字段来源有动态 JSON、静态 fallback、渠道定价三层。通过 `CacheCreationPriceExplicit` 和定向测试防止层间覆盖错误。
- 当前分支没有完整上游 alias 文件。通过小范围 helper 和归一化测试覆盖别名行为，避免引入上游大重构。
- usage 字段名存在多种上游形态。通过集中解析 helper 和多 fixture 测试降低漏计风险。
- Codex UA/version 可能已有管理员配置。只改默认值，不覆盖已存配置。

## 验收标准

- GPT-5.6 Sol/Terra/Luna 定价与上游官方 fallback 一致。
- GPT-5.6 cache write tokens 能被记录并计费。
- priority service tier 下 input/output/cache write/cache read 均使用 priority 价格。
- `gpt-5.6` 与 `gpt-5.6-max` 能按 Sol 路由，未知 `gpt-5.6-foo` 不误命中。
- GPT-5.6 `max` reasoning effort 在普通路径保留，在 OpenAI OAuth compact 路径按设计降级。
- 前端可选择和生成裸 `gpt-5.6` 及 GPT-5.6 `max` 配置。
- 定向后端和前端测试通过。
