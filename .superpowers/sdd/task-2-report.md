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

---

## 2026-07-07 reviewer Important finding 修复补充

### 问题确认

- reviewer 指出 `backend/internal/service/pricing_service.go` 的 `PricingService.matchOpenAIModel()` 使用 `strings.HasPrefix(model, "gpt-5.6")` 作为静态回退条件
- 该实现会把 `gpt-5.6-foo`、`gpt-5.60` 以及未来任意 `gpt-5.6-*` 未知型号错误映射到 `openAIGPT54FallbackPricing`
- 这与 Task 2 只允许 `gpt-5.6-sol|terra|luna` 在动态定价缺失时回退 GPT-5.4 的要求不一致

### 修复内容

修改文件：

- `backend/internal/service/pricing_service.go`
- `backend/internal/service/pricing_service_test.go`

实现细节：

- 新增 `isOpenAIGPT56StaticFallbackModel(model string) bool`
- 仅当模型名满足以下任一条件时才允许静态回退到 GPT-5.4：
  - 精确等于 `gpt-5.6-sol`
  - 精确等于 `gpt-5.6-terra`
  - 精确等于 `gpt-5.6-luna`
  - 以上三者再追加 `-...` 后缀的规范化变体，例如日期版或能力后缀版
- 未知型号如 `gpt-5.6-foo` 不再命中 GPT-5.4 静态价，而是保持当前代码既有的后续回退路径

### 新增/更新测试

- 保留并验证正例：
  - `TestGetModelPricing_Gpt56UsesGpt54StaticFallbackWhenRemoteMissing`
  - `TestGPT56Support_PricingServiceStaticFallback`
- 新增负例：
  - `TestGetModelPricing_Gpt56UnknownModelDoesNotUseGpt54StaticFallback`

负例构造方式：

- 仅注入 `pricingData["gpt-5.1-codex"]`
- 调用 `svc.GetModelPricing("gpt-5.6-foo")`
- 断言返回的是默认模型定价对象，而不是 `openAIGPT54FallbackPricing`

### 验证

执行：

```bash
cd backend
go test -tags unit ./internal/service -run 'TestGetModelPricing_Gpt56UsesGpt54StaticFallbackWhenRemoteMissing|TestGPT56Support_PricingServiceStaticFallback|TestGetModelPricing_Gpt56UnknownModelDoesNotUseGpt54StaticFallback'
```

结果：

```text
ok  	github.com/Wei-Shaw/sub2api/internal/service	0.067s
```

### 范围控制

- 未修改任何前端文件
- 未处理或加入未跟踪文件 `frontend/pnpm-workspace.yaml`
