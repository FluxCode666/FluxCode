# Task 3 报告：BillingService Cache Write And Explicit Overrides

## 状态

已完成并提交。

- 提交：`23d65b448 feat: bill gpt-5.6 cache write tokens`
- 提交文件仅包含任务允许的四个 BillingService/Resolver 源码与测试文件。

## 实现内容

1. `ModelPricing` 新增 `CacheCreationPricePerTokenPriority` 与 `CacheCreationPriceExplicit`。
2. BillingService 从 `LiteLLMModelPricing` 映射 priority cache write 价格，并将其用于 `priority` service tier 的缓存创建计费。
3. GPT-5.6 Sol、Terra、Luna 的 BillingService fallback 改为 Task 2 定义的官方价格；均包含 priority cache write 与固定长上下文参数 `272000 / 2 / 1.5`。
4. GPT-5.6 动态价格缺失 cache write 时，按输入价格的 `1.25` 倍补齐普通及 priority cache write；渠道或区间显式 `CacheWritePrice`（包括 `0`）不会被此策略覆盖。
5. 长上下文判断改为计入输入、cache write 和 cache read token。
6. 对未知 `gpt-5.6-*` 型号，BillingService 不再继续落入通用 GPT/Codex fallback。
7. 渠道平铺价格与区间价格在设置 `CacheWritePrice` 时均标记显式覆盖。

## RED 证据

先加入 brief 要求的失败测试，再执行：

```bash
cd backend && go test -tags unit ./internal/service -run 'TestBillingService_GPT56|TestCalculateCostWithServiceTier_GPT56|TestModelPricingResolver_GPT56|TestIntervalToModelPricing_CacheWriteZero' -count=1
```

结果：失败。编译器报告 `ModelPricing` 缺少 `CacheCreationPricePerTokenPriority` 和 `CacheCreationPriceExplicit`，证明新增测试覆盖的是尚未实现的接口契约。

## GREEN 证据

实现并格式化后执行 brief 指定验证：

```bash
cd backend && go test -tags unit ./internal/service -run 'TestBillingService_GPT56|TestCalculateCostWithServiceTier_GPT56|TestModelPricingResolver_GPT56|TestIntervalToModelPricing_CacheWriteZero|TestGetModelPricingWithChannel_CacheWritePrice' -count=1
```

结果：通过，`ok github.com/Wei-Shaw/sub2api/internal/service 0.071s`。

另已执行 `git diff --check` 与暂存区 `git diff --cached --check`，均无格式或空白错误。

## 文件变更

- `backend/internal/service/billing_service.go`
- `backend/internal/service/model_pricing_resolver.go`
- `backend/internal/service/billing_service_test.go`
- `backend/internal/service/model_pricing_resolver_test.go`

## 自审结论

实现遵守任务边界，未合并 upstream/main，未改默认测试模型，未修改 `docs/superpowers/plans` 或 `.superpowers/sdd/progress.md`。显式零值 cache write 由布尔标记保护，priority cache write 仅在该费率存在时替换缓存创建费率，普通 tier 与 flex tier 继续沿用既有倍率逻辑。

## 疑虑

无功能疑虑。按 brief 执行了目标服务测试；未执行整个 `./internal/service` 全量测试。工作区中原有的 `.superpowers/sdd/task-2-report.md` 改动及 `docs/superpowers/plans/2026-07-10-gpt56-upstream-followups.md` 未跟踪文件未被修改或提交。

---

## Review 修复：GPT-5.6 Billing Safeguards

### 修复内容

1. `BillingService.getFallbackPricing` 现在通过 `normalizeGPT56ModelID` 识别 GPT-5.6 型号后再执行未知型号守卫。因此 `openai/gpt-5.6-foo` 与前后带空白的 `gpt-5.6-foo` 都会返回 `pricing not found`，不再进入 `normalizeCodexModel` 的 `gpt-5.1` fallback。
2. priority 计费仅在 cache write 并非渠道/区间显式覆盖时采用 `CacheCreationPricePerTokenPriority`。显式 `CacheWritePrice`（包括 `0`）会保留原值，避免旧 priority 字段覆盖。
3. 将 `gpt55_support_test.go` 中 GPT-5.6 BillingService 与 PricingService 静态 fallback 断言更新为 GPT-5.6 官方 Sol 价格，并将 BillingService 测试更名为 `TestGPT56Support_BillingFallbackUsesOfficialPrices`。

### RED 证据

新增回归测试后执行：

```bash
cd backend && go test -tags unit ./internal/service -run 'TestBillingService_GPT56|TestCalculateCostWithServiceTier_GPT56|TestModelPricingResolver_GPT56|TestIntervalToModelPricing_CacheWriteZero|TestGetModelPricingWithChannel_CacheWritePrice|TestGPT56Support' -count=1
```

结果：失败。未知 provider/空白别名均返回 GPT-5.1 fallback；priority 下显式 `CacheWritePrice=0` 产生 `0.0005` cache creation cost。

### GREEN 证据

实现后重新执行同一命令：通过，`ok github.com/Wei-Shaw/sub2api/internal/service 0.072s`。

Reviewer 最小复现命令按新测试名更新并通过：

```bash
cd backend && go test -tags unit ./internal/service -run '^TestGPT56Support_BillingFallbackUsesOfficialPrices$' -count=1
```

结果：通过，`ok github.com/Wei-Shaw/sub2api/internal/service 0.031s`。

还执行了 `go test -tags unit ./internal/service -count=1`。该全量包测试未通过，原因与本次文件无关：`TestAdminService_BulkUpdateAccounts_MixedChannelPreCheckBlocksOnExistingConflict` 的错误文本断言失败、`TestAdminService_CreateUser_Success` 缺少 group repository，以及 SettingService 预热调用测试 stub 的 `GetMultiple` 时 panic。目标回归集通过。

---

## 第二轮 Review 收口：GPT-5.6 测试价格预期

### 修复内容

1. `TestGetModelPricing_OpenAIGPT56Fallbacks` 分别断言 Sol、Terra、Luna 的官方 input、output、cache write 和 cache read 价格。
2. `TestCalculateCost_OpenAIGPT56LongContextAppliesWholeSessionMultipliers` 改用 Sol 官方 input `5e-6`、output `30e-6`，并继续验证长上下文 input `*2`、output `*1.5`。

### RED 证据

```bash
cd backend && go test -tags unit ./internal/service -run 'TestGetModelPricing_OpenAIGPT56Fallbacks|TestCalculateCost_OpenAIGPT56LongContextAppliesWholeSessionMultipliers|TestBillingService_GPT56|TestCalculateCostWithServiceTier_GPT56|TestGPT56Support' -count=1
```

结果：失败，原测试对 Sol/Luna 使用 GPT-5.4 价格，并对 GPT-5.6 长上下文使用旧 input/output 价格。

### GREEN 证据

同一命令在更新测试预期后通过。

## 本轮疑虑

无功能疑虑。报告文件按要求保留为工作区修改，未加入提交。
