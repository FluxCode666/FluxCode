# GPT-5.6 模型支持迁移设计

## 背景

当前分支包含 FluxCode 的本地改动，尚未同步上游 `Wei-Shaw/sub2api` 中 GPT-5.6 模型支持相关逻辑。拉取 `upstream/main` 后，确认相关上游提交为 `6cea1c35b`（`feat: 适配 OpenAI 新模型 gpt-5.6-sol/terra/luna`）。

该上游提交涉及 8 个文件：

- `backend/internal/pkg/openai/constants.go`
- `backend/internal/service/billing_service.go`
- `backend/internal/service/openai_codex_transform.go`
- `backend/internal/service/openai_model_alias.go`
- `backend/internal/service/pricing_service.go`
- `backend/resources/model-pricing/model_prices_and_context_window.json`
- `frontend/src/components/keys/UseKeyModal.vue`
- `frontend/src/composables/useModelWhitelist.ts`

当前分支没有 `backend/internal/service/openai_model_alias.go`，模型归一化仍位于 `backend/internal/service/openai_codex_transform.go` 的 `normalizeCodexModel` 中。因此本次迁移应采用语义手工移植，而不是直接 cherry-pick 上游提交。

## 目标

为以下三个 OpenAI GPT-5.6 模型增加支持：

- `gpt-5.6-sol`
- `gpt-5.6-terra`
- `gpt-5.6-luna`

支持范围包括：

- 三个模型出现在默认 OpenAI 模型列表中。
- Codex/OpenAI 模型归一化保留各自 GPT-5.6 变体，不再回退到 `gpt-5.1`。
- OpenAI OAuth/Codex 转发可以路由精确模型 ID。
- 动态定价数据包含三个模型条目。
- 动态定价缺失时，静态计费 fallback 仍可用，并沿用 GPT-5.4 的价格与长上下文策略。
- 前端模型选择、预设映射快捷项、OpenCode 配置生成均暴露这三个模型。

## 方案

采用语义手工移植。不直接 cherry-pick `6cea1c35b`，也不把上游较新的 `openai_model_alias.go` 结构引入当前分支。

这样可以让补丁保持小范围，并贴合当前分支较旧的模型归一化架构。

## 后端设计

### 默认 OpenAI 模型

在 `backend/internal/pkg/openai/constants.go` 中，将三个 GPT-5.6 模型加入 `DefaultModels`，位置放在 `gpt-5.5` 之前。

使用上游元数据：

- `Created`: `1780876800`
- `OwnedBy`: `openai`
- `Type`: `model`
- 展示名：`GPT-5.6 Sol`、`GPT-5.6 Terra`、`GPT-5.6 Luna`

不修改 `DefaultTestModel`。当前分支仍使用 `gpt-5.1-codex` 作为测试和账号探测默认模型。

### Codex 模型映射

在 `backend/internal/service/openai_codex_transform.go` 中，为 `codexModelMap` 增加精确映射：

- `gpt-5.6-sol` -> `gpt-5.6-sol`
- `gpt-5.6-terra` -> `gpt-5.6-terra`
- `gpt-5.6-luna` -> `gpt-5.6-luna`

更新 `normalizeCodexModel`，让它在更宽泛的 `gpt-5.5`、`gpt-5.4`、`gpt-5` 判断之前识别三个 GPT-5.6 模型。需要支持当前分支已有的别名风格：

- 带连字符的名称，例如 `gpt-5.6-sol-high`
- 空格分隔的名称，例如 `gpt 5.6 sol`
- 带 provider 前缀的名称，例如 `openai/gpt-5.6-sol`

每个 GPT-5.6 变体都应归一化到自身基础模型，不应落到 `gpt-5.1`、`gpt-5.4` 或 `gpt-5.5`。

### 计费 fallback

在 `backend/internal/service/billing_service.go` 中，将 GPT-5.6 的 fallback 价格指向现有 GPT-5.4 fallback：

- `gpt-5.6-sol`
- `gpt-5.6-terra`
- `gpt-5.6-luna`

在 `getFallbackPricing` 中，`normalizeCodexModel` 解析出这些模型后，应返回对应 fallback 条目。

在 `isOpenAIGPT54Model` 中纳入三个 GPT-5.6 模型，确保定价数据缺少长上下文倍数时仍会应用 GPT-5.4 的长上下文计费策略。

### 动态定价 fallback

在 `backend/internal/service/pricing_service.go` 中更新 `matchOpenAIModel`：当模型以 `gpt-5.6` 开头，且动态定价没有精确命中或变体命中时，回退到 `openAIGPT54FallbackPricing`。

该行为与 `gpt-5.5` 保持一致，同时保留现有 GPT-5.4 mini/nano 的特殊 fallback。

### 定价资源

在 `backend/resources/model-pricing/model_prices_and_context_window.json` 中，将三个上游模型对象加入 `gpt-5.5` 之前。

使用上游字段，包括：

- `max_input_tokens`: `1050000`
- `max_output_tokens`: `128000`
- `supported_endpoints`: `/v1/chat/completions`、`/v1/batch`、`/v1/responses`
- 支持文本和图片输入
- 支持 prompt caching、reasoning、service tier、tool choice、vision、web search
- 包含超过 `272k` tokens 的长上下文字段

## 前端设计

### 模型白名单

在 `frontend/src/composables/useModelWhitelist.ts` 中，将三个 GPT-5.6 模型加入 OpenAI 模型列表，位置靠近现有 GPT-5.5/GPT-5.4 条目。

增加 OpenAI 预设映射按钮：

- `GPT-5.6 Sol`
- `GPT-5.6 Terra`
- `GPT-5.6 Luna`

每个预设都映射到自身模型。颜色使用已有 Tailwind 色系，优先沿用上游选择，除非与本地样式冲突。

### Use Key Modal

在 `frontend/src/components/keys/UseKeyModal.vue` 中，将 GPT-5.6 条目加入生成 OpenCode 配置的 `openaiModels` 对象。

每个模型使用：

- `context`: `1050000`
- `output`: `128000`
- `store`: `false`
- `variants`: `low`、`medium`、`high`、`xhigh`

不修改生成的 Codex `config.toml` 默认模型行为。该弹窗已经通过 `props.openaiUseKeyModelId` 解析模型；本次迁移只让 GPT-5.6 在被配置或选择时可用。

## 错误处理

未知 OpenAI 模型继续走现有定价错误路径。本次 GPT-5.6 迁移不应扩大 fallback 匹配范围到任意 `gpt-*` 模型。

如果动态定价数据不可用，或其中不包含 GPT-5.6，计费路径仍应通过 GPT-5.4 fallback 正常计算费用。如果动态定价中包含 GPT-5.6，则优先使用动态定价。

## 测试

后端测试：

- 扩展归一化测试，覆盖 `gpt-5.6-sol`、`gpt-5.6-terra`、`gpt-5.6-luna`，并至少覆盖一个 reasoning suffix 和一个空格分隔别名。
- 新增或扩展计费 fallback 测试，验证 GPT-5.6 使用 GPT-5.4 fallback 价格。
- 新增或扩展长上下文计费测试，验证 GPT-5.6 继承 GPT-5.4 倍数。
- 新增或扩展 `PricingService` fallback 测试，验证动态定价缺失时回退到 `openAIGPT54FallbackPricing`。

前端测试：

- 扩展 `useModelWhitelist` 测试，断言 OpenAI 模型列表包含三个 GPT-5.6 模型。
- 扩展模型映射测试，确保 whitelist 模式可以为 GPT-5.6 生成自身映射。
- 扩展 `UseKeyModal` 测试，断言 OpenCode 生成配置包含 GPT-5.6 模型元数据。

验证命令聚焦本次触达区域：

- 在 `backend/` 目录执行：`go test -tags unit ./internal/service ./internal/pkg/openai`
- 在仓库根目录执行：`pnpm --dir frontend test -- useModelWhitelist UseKeyModal`

如果本地仓库的实际测试命令不同，使用 package scripts 中最接近的定向命令。

## 非目标

- 不同步整个 `upstream/main`。
- 不引入 `openai_model_alias.go`。
- 不修改 `DefaultTestModel`。
- 不修改管理端默认 `openai_use_key_model_id`。
- 不修改无关 GPT、Claude、Gemini、xAI 或图片模型的价格。
- 不重构前端模型白名单结构。

## 实现边界

实现应是当前分支上的小范围补丁。代码应把上游语义适配到本地结构，并包含定向测试。账号导入、风控、批量图片、新认证流程等更大范围上游变更均不属于本次迁移。
