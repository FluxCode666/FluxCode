# GPT-5.6 Pricing Metadata Alignment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将内置 GPT-5.6 Sol、Terra、Luna 定价元数据精准对齐到 `upstream/main@551e2570d`，并用离线回归测试阻止 batch/flex 字段再次漂移。

**Architecture:** 保持现有 `PricingService` 与 `BillingService` 数据流不变，只修正内置 JSON 中三个 GPT-5.6 对象的 tier 元数据。测试直接读取原始 JSON，以便覆盖当前运行时解析器尚未消费的 batch/flex 字段及废弃字段不存在性。

**Tech Stack:** Go 1.26、标准库 `encoding/json`、`testify/require`、JSON、`jq`、Git。

## Global Constraints

- 对齐基线固定为 `upstream/main@551e2570d`。
- 只修改 `gpt-5.6-sol`、`gpt-5.6-terra`、`gpt-5.6-luna` 三个定价对象。
- 不修改 `BillingService`、`PricingService`、usage 解析、模型别名或前端代码。
- 不扩展 `LiteLLMModelPricing` 或 `LiteLLMRawEntry` 去消费 batch/flex 字段。
- 保留长上下文阈值 `272000`、输入倍率 `2`、输出倍率 `1.5`。
- 不引入 `TokenPricingAbsent`、图片计费保护或其他非 GPT-5.6 上游变化。
- 自动化测试不得依赖网络或实时上游仓库。

## File Structure

- `backend/resources/model-pricing/model_prices_and_context_window.json`：保存内置模型定价元数据；只更新三个 GPT-5.6 对象。
- `backend/internal/service/pricing_service_test.go`：读取内置资源并断言 GPT-5.6 标准、batch、flex、priority 与长上下文字段。

---

### Task 1: 对齐 GPT-5.6 tier 定价元数据

**Files:**
- Modify: `backend/internal/service/pricing_service_test.go`
- Modify: `backend/resources/model-pricing/model_prices_and_context_window.json`

**Interfaces:**
- Consumes: 内置 JSON 对象 `gpt-5.6-sol`、`gpt-5.6-terra`、`gpt-5.6-luna`。
- Produces: 测试 `TestDefaultPricingGPT56TierMetadataMatchesUpstream()`；不新增生产代码接口。

- [ ] **Step 1: 写入会失败的原始定价元数据测试**

在 `backend/internal/service/pricing_service_test.go` 中加入以下测试。现有 import 已包含 `encoding/json`、`os`、`path/filepath`、`testing` 和 `require`，无需修改 import。

```go
func TestDefaultPricingGPT56TierMetadataMatchesUpstream(t *testing.T) {
	type tierMetadata struct {
		Input                       float64  `json:"input_cost_per_token"`
		InputBatch                  float64  `json:"input_cost_per_token_batches"`
		InputFlex                   float64  `json:"input_cost_per_token_flex"`
		InputPriority               float64  `json:"input_cost_per_token_priority"`
		InputAbove272K              *float64 `json:"input_cost_per_token_above_272k_tokens"`
		Output                      float64  `json:"output_cost_per_token"`
		OutputBatch                 float64  `json:"output_cost_per_token_batches"`
		OutputFlex                  float64  `json:"output_cost_per_token_flex"`
		OutputPriority              float64  `json:"output_cost_per_token_priority"`
		OutputAbove272K             *float64 `json:"output_cost_per_token_above_272k_tokens"`
		CacheWrite                  float64  `json:"cache_creation_input_token_cost"`
		CacheWriteBatch             float64  `json:"cache_creation_input_token_cost_batches"`
		CacheWriteFlex              float64  `json:"cache_creation_input_token_cost_flex"`
		CacheWritePriority          float64  `json:"cache_creation_input_token_cost_priority"`
		CacheRead                   float64  `json:"cache_read_input_token_cost"`
		CacheReadFlex               float64  `json:"cache_read_input_token_cost_flex"`
		CacheReadPriority           float64  `json:"cache_read_input_token_cost_priority"`
		CacheReadAbove272K          *float64 `json:"cache_read_input_token_cost_above_272k_tokens"`
		LongContextThreshold        int      `json:"long_context_input_token_threshold"`
		LongContextInputMultiplier  float64  `json:"long_context_input_cost_multiplier"`
		LongContextOutputMultiplier float64  `json:"long_context_output_cost_multiplier"`
	}

	data, err := os.ReadFile(filepath.Join("..", "..", "resources", "model-pricing", "model_prices_and_context_window.json"))
	require.NoError(t, err)

	var pricing map[string]tierMetadata
	require.NoError(t, json.Unmarshal(data, &pricing))

	tests := []struct {
		model                                                             string
		input, output, cacheWrite, cacheRead                              float64
		inputBatch, outputBatch, cacheWriteBatch                          float64
		inputFlex, outputFlex, cacheWriteFlex, cacheReadFlex              float64
		inputPriority, outputPriority, cacheWritePriority, cacheReadPriority float64
	}{
		{
			model: "gpt-5.6-sol",
			input: 5e-6, output: 30e-6, cacheWrite: 6.25e-6, cacheRead: 0.5e-6,
			inputBatch: 2.5e-6, outputBatch: 15e-6, cacheWriteBatch: 3.125e-6,
			inputFlex: 2.5e-6, outputFlex: 15e-6, cacheWriteFlex: 3.125e-6, cacheReadFlex: 0.25e-6,
			inputPriority: 10e-6, outputPriority: 60e-6, cacheWritePriority: 12.5e-6, cacheReadPriority: 1e-6,
		},
		{
			model: "gpt-5.6-terra",
			input: 2.5e-6, output: 15e-6, cacheWrite: 3.125e-6, cacheRead: 0.25e-6,
			inputBatch: 1.25e-6, outputBatch: 7.5e-6, cacheWriteBatch: 1.5625e-6,
			inputFlex: 1.25e-6, outputFlex: 7.5e-6, cacheWriteFlex: 1.5625e-6, cacheReadFlex: 0.125e-6,
			inputPriority: 5e-6, outputPriority: 30e-6, cacheWritePriority: 6.25e-6, cacheReadPriority: 0.5e-6,
		},
		{
			model: "gpt-5.6-luna",
			input: 1e-6, output: 6e-6, cacheWrite: 1.25e-6, cacheRead: 0.1e-6,
			inputBatch: 0.5e-6, outputBatch: 3e-6, cacheWriteBatch: 0.625e-6,
			inputFlex: 0.5e-6, outputFlex: 3e-6, cacheWriteFlex: 0.625e-6, cacheReadFlex: 0.05e-6,
			inputPriority: 2e-6, outputPriority: 12e-6, cacheWritePriority: 2.5e-6, cacheReadPriority: 0.2e-6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got, ok := pricing[tt.model]
			require.True(t, ok, "missing pricing object %s", tt.model)

			require.InDelta(t, tt.input, got.Input, 1e-12)
			require.InDelta(t, tt.output, got.Output, 1e-12)
			require.InDelta(t, tt.cacheWrite, got.CacheWrite, 1e-12)
			require.InDelta(t, tt.cacheRead, got.CacheRead, 1e-12)
			require.InDelta(t, tt.inputBatch, got.InputBatch, 1e-12)
			require.InDelta(t, tt.outputBatch, got.OutputBatch, 1e-12)
			require.InDelta(t, tt.cacheWriteBatch, got.CacheWriteBatch, 1e-12)
			require.InDelta(t, tt.inputFlex, got.InputFlex, 1e-12)
			require.InDelta(t, tt.outputFlex, got.OutputFlex, 1e-12)
			require.InDelta(t, tt.cacheWriteFlex, got.CacheWriteFlex, 1e-12)
			require.InDelta(t, tt.cacheReadFlex, got.CacheReadFlex, 1e-12)
			require.InDelta(t, tt.inputPriority, got.InputPriority, 1e-12)
			require.InDelta(t, tt.outputPriority, got.OutputPriority, 1e-12)
			require.InDelta(t, tt.cacheWritePriority, got.CacheWritePriority, 1e-12)
			require.InDelta(t, tt.cacheReadPriority, got.CacheReadPriority, 1e-12)

			require.Equal(t, 272000, got.LongContextThreshold)
			require.InDelta(t, 2.0, got.LongContextInputMultiplier, 1e-12)
			require.InDelta(t, 1.5, got.LongContextOutputMultiplier, 1e-12)
			require.Nil(t, got.InputAbove272K)
			require.Nil(t, got.OutputAbove272K)
			require.Nil(t, got.CacheReadAbove272K)
		})
	}
}
```

- [ ] **Step 2: 运行新测试，确认它因现有元数据漂移而失败**

Run:

```bash
cd backend
go test -tags unit ./internal/service -run '^TestDefaultPricingGPT56TierMetadataMatchesUpstream$' -count=1
```

Expected: FAIL。第一个失败点应为 Sol 缺少 `cache_creation_input_token_cost_batches`，或 Terra/Luna 的 batch/flex 数值与预期不一致；不得接受编译失败作为预期失败。

- [ ] **Step 3: 将三个 GPT-5.6 对象的定价字段改为固定上游值**

在 `backend/resources/model-pricing/model_prices_and_context_window.json` 的对应对象中，将定价字段调整为以下内容。对象中的模型能力、上下文窗口、端点列表等其他字段保持不变。

`gpt-5.6-sol`：

```json
"cache_creation_input_token_cost": 6.25e-06,
"cache_creation_input_token_cost_batches": 3.125e-06,
"cache_creation_input_token_cost_flex": 3.125e-06,
"cache_creation_input_token_cost_priority": 1.25e-05,
"cache_read_input_token_cost": 5e-07,
"cache_read_input_token_cost_flex": 2.5e-07,
"cache_read_input_token_cost_priority": 1e-06,
"input_cost_per_token": 5e-06,
"input_cost_per_token_batches": 2.5e-06,
"input_cost_per_token_flex": 2.5e-06,
"input_cost_per_token_priority": 1e-05,
"long_context_input_token_threshold": 272000,
"long_context_input_cost_multiplier": 2.0,
"long_context_output_cost_multiplier": 1.5,
"output_cost_per_token": 3e-05,
"output_cost_per_token_batches": 1.5e-05,
"output_cost_per_token_flex": 1.5e-05,
"output_cost_per_token_priority": 6e-05
```

`gpt-5.6-terra`：

```json
"cache_creation_input_token_cost": 3.125e-06,
"cache_creation_input_token_cost_batches": 1.5625e-06,
"cache_creation_input_token_cost_flex": 1.5625e-06,
"cache_creation_input_token_cost_priority": 6.25e-06,
"cache_read_input_token_cost": 2.5e-07,
"cache_read_input_token_cost_flex": 1.25e-07,
"cache_read_input_token_cost_priority": 5e-07,
"input_cost_per_token": 2.5e-06,
"input_cost_per_token_batches": 1.25e-06,
"input_cost_per_token_flex": 1.25e-06,
"input_cost_per_token_priority": 5e-06,
"long_context_input_token_threshold": 272000,
"long_context_input_cost_multiplier": 2.0,
"long_context_output_cost_multiplier": 1.5,
"output_cost_per_token": 1.5e-05,
"output_cost_per_token_batches": 7.5e-06,
"output_cost_per_token_flex": 7.5e-06,
"output_cost_per_token_priority": 3e-05
```

`gpt-5.6-luna`：

```json
"cache_creation_input_token_cost": 1.25e-06,
"cache_creation_input_token_cost_batches": 6.25e-07,
"cache_creation_input_token_cost_flex": 6.25e-07,
"cache_creation_input_token_cost_priority": 2.5e-06,
"cache_read_input_token_cost": 1e-07,
"cache_read_input_token_cost_flex": 5e-08,
"cache_read_input_token_cost_priority": 2e-07,
"input_cost_per_token": 1e-06,
"input_cost_per_token_batches": 5e-07,
"input_cost_per_token_flex": 5e-07,
"input_cost_per_token_priority": 2e-06,
"long_context_input_token_threshold": 272000,
"long_context_input_cost_multiplier": 2.0,
"long_context_output_cost_multiplier": 1.5,
"output_cost_per_token": 6e-06,
"output_cost_per_token_batches": 3e-06,
"output_cost_per_token_flex": 3e-06,
"output_cost_per_token_priority": 1.2e-05
```

从三个对象中删除以下字段，不用 `0` 或 `null` 代替：

```text
input_cost_per_token_above_272k_tokens
output_cost_per_token_above_272k_tokens
cache_read_input_token_cost_above_272k_tokens
```

- [ ] **Step 4: 运行新测试，确认元数据对齐后通过**

Run:

```bash
cd backend
go test -tags unit ./internal/service -run '^TestDefaultPricingGPT56TierMetadataMatchesUpstream$' -count=1
```

Expected:

```text
ok  github.com/Wei-Shaw/sub2api/internal/service
```

- [ ] **Step 5: 对固定上游引用执行三个对象的规范化语义比较**

从仓库根目录运行：

```bash
tmp_local="$(mktemp)"
tmp_upstream="$(mktemp)"
jq -S 'with_entries(select(.key == "gpt-5.6-sol" or .key == "gpt-5.6-terra" or .key == "gpt-5.6-luna"))' \
  backend/resources/model-pricing/model_prices_and_context_window.json > "$tmp_local"
git show 551e2570d:backend/resources/model-pricing/model_prices_and_context_window.json | \
  jq -S 'with_entries(select(.key == "gpt-5.6-sol" or .key == "gpt-5.6-terra" or .key == "gpt-5.6-luna"))' > "$tmp_upstream"
diff -u "$tmp_upstream" "$tmp_local"
rm -f "$tmp_local" "$tmp_upstream"
```

Expected: `diff` 无输出并以状态码 `0` 结束。若只剩 JSON 数字表示差异，直接采用上游表示形式，不放宽比较规则。

- [ ] **Step 6: 运行现有 GPT-5.6 计费与 usage 回归测试**

Run:

```bash
cd backend
go test -tags unit ./internal/service ./internal/pkg/apicompat \
  -run 'GPT56|CacheWrite|CacheCreationUsage' -count=1
```

Expected: 两个包均输出 `ok`。如果首次运行停留在依赖下载，先解决依赖可用性后重新运行；不得把未进入测试执行的下载中断记为通过。

- [ ] **Step 7: 检查范围并提交实现**

从仓库根目录运行：

```bash
git diff --check
git diff -- backend/internal/service/pricing_service_test.go \
  backend/resources/model-pricing/model_prices_and_context_window.json
git status --short
```

Expected:

- `git diff --check` 无输出。
- 业务 diff 只有测试和三个 GPT-5.6 JSON 对象。
- `git status --short` 只列出上述两个实现文件；设计和计划文档已在此前提交中。

提交：

```bash
git add backend/internal/service/pricing_service_test.go \
  backend/resources/model-pricing/model_prices_and_context_window.json
git commit -m "fix(pricing): align gpt-5.6 tier metadata"
```

Expected: commit 成功，提交只包含两个文件。
