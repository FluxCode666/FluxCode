# Task 2: PricingService GPT-5.6 Official Prices 报告

## 实现内容

- 为 `LiteLLMModelPricing` 与 `LiteLLMRawEntry` 增加 `CacheCreationInputTokenCostPriority`，并在 `parsePricingData` 中保留该字段。
- 新增 GPT-5.6 Sol、Terra、Luna 的官方静态回退价格，包含 priority、cache write/read、服务层级、提示缓存和固定长上下文参数。
- `buildModelLookupCandidates` 先插入 `normalizeGPT56ModelAlias` 的结果，因此裸 `gpt-5.6` 在动态资源存在时优先命中 `gpt-5.6-sol`。
- `matchOpenAIModel` 仅为已知 GPT-5.6 别名使用对应官方静态价格；未知 `gpt-5.6-*` 直接返回 `nil`，不会落入 Sol、Terra、Luna、GPT-5.4 或默认测试模型。
- 更新 bundled JSON 中 Sol、Terra、Luna 的官方字段。三者长上下文参数均为阈值 `272000`、输入倍率 `2`、输出倍率 `1.5`。

## RED 证据

先更新 `backend/internal/service/pricing_service_test.go`，再执行：

```bash
cd backend && go test -tags unit ./internal/service -run 'TestPricingService_GPT56|TestParsePricingData_ReadsPriorityCacheWrite' -count=1
```

结果为失败（退出码 1）：`LiteLLMModelPricing` 不存在 `CacheCreationInputTokenCostPriority`。失败原因是任务所需生产接口尚未实现。

## GREEN 证据

实现后，以相同命令重新执行，结果通过：

```text
ok github.com/Wei-Shaw/sub2api/internal/service 0.053s
```

附加验证：

- `jq empty backend/resources/model-pricing/model_prices_and_context_window.json` 通过。
- 三条 `jq -e` 精确字段断言通过，覆盖 Sol、Terra、Luna 的 13 个指定字段。
- `git diff --check` 通过。

## 文件变更

- `backend/internal/service/pricing_service.go`
- `backend/internal/service/pricing_service_test.go`
- `backend/resources/model-pricing/model_prices_and_context_window.json`

## 自审结论

实现限制在任务要求的 PricingService、测试与定价资源范围内；没有修改默认测试模型，没有合并 `upstream/main`，也没有引入无关的网关重构或大文件拆分。裸 `gpt-5.6` 的动态 Sol 优先级和未知 GPT-5.6 型号拒绝回退均已在代码路径中落实。

## 疑虑

完整命令 `cd backend && go test -tags unit ./internal/service -count=1` 仍失败于与本任务无关的既有测试：两个 AdminService 测试的预期不一致，以及 SettingService 后台预热协程调用未预期的 stub `GetMultiple`。任务 brief 指定的 GPT-5.6 测试通过。

## Review 修复（2026-07-10）

### 根因与修复

- `LiteLLMRawEntry` 未声明三个 `long_context_*` 字段，导致 JSON 中的 GPT-5.6 长上下文参数在 `parsePricingData` 后归零。已增加指针字段并复制到 `LiteLLMModelPricing`。
- `matchOpenAIModel` 先枚举通用 variants，未知 `gpt-5.6-foo` 会降级为 `gpt-5.6` 并命中动态裸键。现在 GPT-5.6 在 variants 前单独分流：已知别名返回官方静态回退，未知型号直接返回 `nil`。
- 已统一 bundled JSON 的 `*_above_272k_tokens`：Sol 保持 `1e-5/4.5e-5/1e-6`，Terra 为 `5e-6/22.5e-6/0.5e-6`，Luna 为 `2e-6/9e-6/0.2e-6`（输入/输出/cache read）。

### 回归覆盖与验证

- RED：在定价数据包含裸 `gpt-5.6` 时，`TestPricingService_GPT56UnknownDoesNotFallback` 失败并错误返回该动态定价；`TestParsePricingData_ReadsPriorityCacheWrite` 失败并显示长上下文阈值为 `0`。
- GREEN：`TestPricingService_GPT56BareUsesDynamicSolPricing` 断言动态路径保留 `272000/2/1.5`；解析测试也断言三个字段；未知型号测试覆盖裸键存在时仍返回 `nil`。
- `cd backend && go test -tags unit ./internal/service -run 'TestPricingService_GPT56|TestParsePricingData_ReadsPriorityCacheWrite' -count=1` 通过。
- `jq empty backend/resources/model-pricing/model_prices_and_context_window.json` 通过；Sol、Terra、Luna 的九项 `above_272k_tokens` 精确断言通过。
