# Task 1 报告：后端模型目录与 Codex 归一化

## 任务范围

严格按照 brief，仅修改以下后端文件：

- `backend/internal/pkg/openai/constants.go`
- `backend/internal/pkg/openai/constants_test.go`
- `backend/internal/service/openai_codex_transform.go`
- `backend/internal/service/openai_codex_transform_test.go`
- `backend/internal/service/gpt55_support_test.go`

未修改、未删除、未暂存 `frontend/pnpm-workspace.yaml`。

## TDD 执行记录

### 1. 先写失败测试

- 新增 `backend/internal/pkg/openai/constants_test.go`
  - `TestDefaultModelsIncludeGPT56Variants`
- 扩展 `backend/internal/service/openai_codex_transform_test.go`
  - 在现有 `TestNormalizeCodexModel_Gpt53` 中加入 `gpt-5.6-sol`、`gpt-5.6-terra`、`gpt-5.6-luna` 及其别名覆盖
- 扩展 `backend/internal/service/gpt55_support_test.go`
  - 新增 `TestGPT56Support_NormalizeCodexModel`

### 2. 验证红灯

运行：

```bash
cd backend
go test -tags unit ./internal/pkg/openai ./internal/service -run 'TestDefaultModelsIncludeGPT56Variants|TestNormalizeCodexModel_Gpt53|TestGPT56Support_NormalizeCodexModel'
```

首次结果：`FAIL`

关键失败点：

- `DefaultModelIDs()` 不包含 `gpt-5.6-sol`
- `normalizeCodexModel("gpt-5.6-sol")` 返回 `gpt-5.1`

过程中还发现 `openai_codex_transform_test.go` 里已有同名 `TestNormalizeCodexModel_Gpt53`，已将新增断言合并进现有测试，避免重复定义导致编译失败。这属于测试落点对齐，不改变 brief 要求的覆盖范围。

### 3. 最小实现

在 `backend/internal/pkg/openai/constants.go`：

- 将 `gpt-5.6-sol`
- `gpt-5.6-terra`
- `gpt-5.6-luna`

加入 `DefaultModels`，位置位于 `gpt-5.5` 之前。

在 `backend/internal/service/openai_codex_transform.go`：

- 将上述 3 个模型加入 `codexModelMap`
- 在 `normalizeCodexModel` 中新增对以下形式的归一化支持：
  - `gpt-5.6-sol` / `gpt 5.6 sol`
  - `gpt-5.6-terra` / `gpt 5.6 terra`
  - `gpt-5.6-luna` / `gpt 5.6 luna`

### 4. 格式化并验证绿灯

运行：

```bash
cd backend
gofmt -w internal/pkg/openai/constants.go internal/pkg/openai/constants_test.go internal/service/openai_codex_transform.go internal/service/openai_codex_transform_test.go internal/service/gpt55_support_test.go
go test -tags unit ./internal/pkg/openai ./internal/service -run 'TestDefaultModelsIncludeGPT56Variants|TestNormalizeCodexModel_Gpt53|TestGPT56Support_NormalizeCodexModel'
```

结果：`PASS`

## 变更摘要

- 后端默认模型目录新增 3 个 GPT-5.6 基础模型
- Codex 模型归一化支持 3 个 GPT-5.6 变体及常见别名
- 为默认模型列表和归一化行为补充单元测试

## 风险与备注

- 本次仅实现 Task 1 brief 要求内容
- 未涉及计费、前端、定价 JSON
- 未扩大测试范围到 brief 之外的 Go 测试集

## 提交信息

提交完成后可通过 Git 查看：

- 提交标题：`feat(openai): add gpt-5.6 model normalization`
