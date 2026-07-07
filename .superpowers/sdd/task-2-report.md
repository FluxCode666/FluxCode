# Task 2 报告

## 任务范围

- 仅实现 Task 2：后端计费、`PricingService` 动态定价 fallback、`backend/resources/model-pricing/model_prices_and_context_window.json`
- 未修改前端白名单、`UseKeyModal` 或其他非 brief 范围内容
- 未触碰未跟踪本地文件 `frontend/pnpm-workspace.yaml`

## 执行流程

### 1. 读取唯一需求来源

- 已读取 `/Users/duegin/.codex/worktrees/fe18/FluxCode/.superpowers/sdd/task-2-brief.md`

### 2. 按 TDD 先补失败测试

新增测试：

- `backend/internal/service/billing_service_test.go`
  - `TestGetModelPricing_OpenAIGPT56Fallbacks`
  - `TestCalculateCost_OpenAIGPT56LongContextAppliesWholeSessionMultipliers`
- `backend/internal/service/pricing_service_test.go`
  - `TestGetModelPricing_Gpt56UsesGpt54StaticFallbackWhenRemoteMissing`
- `backend/internal/service/gpt55_support_test.go`
  - `TestGPT56Support_BillingFallbackMatchesGPT54`
  - `TestGPT56Support_PricingServiceStaticFallback`

### 3. 先跑红灯验证失败

执行：

```bash
cd backend
go test -tags unit ./internal/service -run 'TestGetModelPricing_OpenAIGPT56Fallbacks|TestCalculateCost_OpenAIGPT56LongContextAppliesWholeSessionMultipliers|TestGPT56Support_BillingFallbackMatchesGPT54|TestGetModelPricing_Gpt56UsesGpt54StaticFallbackWhenRemoteMissing|TestGPT56Support_PricingServiceStaticFallback'
```

结果符合预期：

- `BillingService` 对 `gpt-5.6-sol|terra|luna` 返回 `pricing not found`
- `PricingService` 对 `gpt-5.6-*` 错误回退到了 `gpt-5.1-codex`

## 实现内容

### BillingService

修改文件：

- `backend/internal/service/billing_service.go`

变更：

- 将 `gpt-5.6-sol`
- `gpt-5.6-terra`
- `gpt-5.6-luna`

全部加入本地静态 fallback 表，并统一指向 `gpt-5.4`

- 在 `getFallbackPricing` 的 OpenAI 分支中增加三种 `gpt-5.6-*` 命中逻辑
- 扩展 `isOpenAIGPT54Model`，让 `gpt-5.6-*` 共享 GPT-5.4 的长上下文倍率策略

### PricingService

修改文件：

- `backend/internal/service/pricing_service.go`

变更：

- 在 `matchOpenAIModel` 中为 `gpt-5.6*` 增加静态回退
- 当动态定价缺失时，`gpt-5.6-*` 统一返回 `openAIGPT54FallbackPricing`

### 定价资源

修改文件：

- `backend/resources/model-pricing/model_prices_and_context_window.json`

新增资源对象：

- `gpt-5.6-sol`
- `gpt-5.6-terra`
- `gpt-5.6-luna`

## 验证

执行：

```bash
cd backend
gofmt -w internal/service/billing_service.go internal/service/billing_service_test.go internal/service/pricing_service.go internal/service/pricing_service_test.go internal/service/gpt55_support_test.go
python3 -m json.tool resources/model-pricing/model_prices_and_context_window.json >/tmp/gpt56-pricing.json
go test -tags unit ./internal/service -run 'TestGetModelPricing_OpenAIGPT56Fallbacks|TestCalculateCost_OpenAIGPT56LongContextAppliesWholeSessionMultipliers|TestGPT56Support_BillingFallbackMatchesGPT54|TestGetModelPricing_Gpt56UsesGpt54StaticFallbackWhenRemoteMissing|TestGPT56Support_PricingServiceStaticFallback'
```

结果：

- `gofmt` 成功
- JSON 校验成功
- 指定 Go 单测全部通过

测试输出摘要：

```text
ok  	github.com/Wei-Shaw/sub2api/internal/service	0.080s
```

## Git 结果

计划提交信息：

```bash
feat(openai): add gpt-5.6 pricing fallback
```

## 备注

- 当前工作区存在未跟踪文件 `frontend/pnpm-workspace.yaml`，已按要求保持不变且不会加入提交
