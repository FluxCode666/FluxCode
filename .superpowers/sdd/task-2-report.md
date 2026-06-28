# Task 2 报告：校验兜底分组配置

## 任务概述

按 TDD 收紧了 admin group fallback 配置校验，覆盖以下规则：

- `is_fallback_group=true` 仅允许 `openai` / `anthropic` 且必须是 `standard`
- fallback target 必须 `active`、`standard`、`is_fallback_group=true`、同平台、非链式
- 保留并继续校验 `claude_code_only` 目标不可作为通用 fallback
- 已有 API Key 直接绑定的分组不能启用为 fallback group
- 多个入口分组可以共用同一个 fallback target
- `fallback_group_id_on_invalid_request` 保持 legacy compatibility，未改运行时行为

## RED

### 新增/调整测试

文件：

- `backend/internal/service/admin_service_group_test.go`

新增/调整的关键测试：

- `TestAdminService_CreateGroup_FallbackGroupFlagRejectsUnsupportedPlatform`
- `TestAdminService_CreateGroup_FallbackGroupFlagRejectsSubscription`
- `TestAdminService_CreateGroup_FallbackGroupFlagRejectsChainedFallback`
- `TestAdminService_CreateGroup_FallbackTargetRejectsInactive`
- `TestAdminService_CreateGroup_FallbackTargetRejectsSubscription`
- `TestAdminService_CreateGroup_FallbackTargetRejectsChainedFallback`
- `TestAdminService_CreateGroup_FallbackTargetRejectsClaudeCodeOnly`
- `TestAdminService_CreateGroup_FallbackTargetRequiresEnabledFlag`
- `TestAdminService_CreateGroup_FallbackTargetMustMatchPlatform`
- `TestAdminService_CreateGroup_MultipleGroupsMayShareFallbackTarget`
- `TestAdminService_UpdateGroup_FallbackGroupFlagRejectsBoundAPIKeys`
- `TestAdminService_UpdateGroup_FallbackGroupFlagRejectsExistingFallbackTarget`
- `TestAdminService_ValidateFallbackGroup_RejectsChainedTarget`

### RED 命令

```bash
cd backend
go test -tags unit ./internal/service -run 'TestAdminService_(CreateGroup_(FallbackGroupFlag|FallbackTarget|MultipleGroupsMayShareFallbackTarget)|UpdateGroup_FallbackGroupFlagRejects(BoundAPIKeys|ExistingFallbackTarget))' -count=1
```

### RED 结果

首次运行先暴露了测试常量问题：`StatusInactive` 未定义，已修正为现有常量 `StatusDisabled`。

修正后再次运行，测试按预期失败，核心失败证据包括：

- `TestAdminService_CreateGroup_FallbackGroupFlagRejectsUnsupportedPlatform`: `An error is expected but got nil`
- `TestAdminService_CreateGroup_FallbackGroupFlagRejectsSubscription`: `An error is expected but got nil`
- `TestAdminService_CreateGroup_FallbackTargetRejectsInactive`: `An error is expected but got nil`
- `TestAdminService_CreateGroup_FallbackTargetRequiresEnabledFlag`: `An error is expected but got nil`
- `TestAdminService_UpdateGroup_FallbackGroupFlagRejectsBoundAPIKeys`: `An error is expected but got nil`

这说明缺口集中在新的 fallback 校验逻辑，而不是测试本身。

## GREEN

### 实现文件

- `backend/internal/service/admin_service.go`

### 主要实现

- 新增 `validateFallbackGroupFlag`
  - 校验 fallback group 平台范围
  - 校验 billing type 必须为 `standard`
  - 校验 fallback group 自身不能再配置 `fallback_group_id`
  - 对已有分组启用 fallback group 时，用 `apiKeyRepo.CountByGroupID` 拒绝已有 API Key 绑定的情况
- 重写 `validateFallbackGroup`
  - 变更为基于入口平台的通用 fallback target 校验
  - 校验 target 必须存在、`active`、`standard`、`is_fallback_group=true`
  - 校验 target 与入口组同平台
  - 校验 target 不能链式 fallback
  - 保留 `claude_code_only` 目标拒绝逻辑
- 调整 `CreateGroup` / `UpdateGroup`
  - 先完成 group 字段归一化
  - 再执行 fallback group flag 校验
  - 最后执行 fallback target 校验
- 保持 `fallback_group_id_on_invalid_request` 旧字段只做兼容校验，未改运行时

## 验证命令与结果

### 1. 聚焦测试

```bash
cd backend
go test -tags unit ./internal/service -run 'TestAdminService_(CreateGroup_(FallbackGroupFlag|FallbackTarget|MultipleGroupsMayShareFallbackTarget)|UpdateGroup_FallbackGroupFlagRejects(BoundAPIKeys|ExistingFallbackTarget)|ValidateFallbackGroup_RejectsChainedTarget)' -count=1
```

结果：

```text
ok  	github.com/Wei-Shaw/sub2api/internal/service	0.304s
```

### 2. service 编译基线

```bash
cd backend
go test -tags unit ./internal/service -run '^$' -count=1
```

结果：

```text
ok  	github.com/Wei-Shaw/sub2api/internal/service	0.304s [no tests to run]
```

### 3. 格式化

```bash
gofmt -w backend/internal/service/admin_service.go backend/internal/service/admin_service_group_test.go
```

结果：成功，无额外输出。

## 变更文件

- `backend/internal/service/admin_service.go`
- `backend/internal/service/admin_service_group_test.go`

## 自审

- 已确认未重复改动 Task 1 已存在的 `CreateGroupInput` / `UpdateGroupInput` 字段
- 已复用现有 `APIKeyRepository.CountByGroupID`，未新增 repository 接口或实现
- 未触碰 Task 3 范围内“禁止绑定兜底组”的用户/API Key 绑定逻辑
- 未修改 `fallback_group_id_on_invalid_request` 的运行时使用面，只保留兼容校验
- 未将 `.superpowers/sdd/*` 纳入 git 变更

## 疑虑 / 后续关注

- 当前 `validateFallbackGroup` 现在将通用 fallback 限制为 `openai` / `anthropic` 入口组；这符合 brief 与 global constraints，但如果后续产品希望 `antigravity` 或其他平台也复用该字段，需要单独扩展规则与测试。

## Reviewer 追修（2026-06-29）

### 修复点

- 收窄 `validateFallbackGroupFlag` 的 API Key 绑定检查：只有在 `UpdateGroup` 将 `IsFallbackGroup` 从 `false` 启用为 `true` 时才调用 `apiKeyRepo.CountByGroupID`
- `CreateGroup` 仍保留兜底分组创建校验，但因为新分组 `ID=0`，不会触发已有 API Key 绑定检查
- 未修改 Task 3 范围内的 API Key 解绑逻辑，也未改运行时兜底行为

### 追加测试

- `TestAdminService_UpdateGroup_FallbackGroupFlagAllowsNonEnablementEdits`
  - 已是 fallback group 且 `apiKeyRepo.CountByGroupID` 返回 `> 0` 时，改 `Name` / 保持 `StatusActive`，`IsFallbackGroup` 为空或显式 `true`，均应成功

### 验证命令与结果

```bash
cd backend
go test -tags unit ./internal/service -run 'TestAdminService_(CreateGroup_(FallbackGroupFlag|FallbackTarget|MultipleGroupsMayShareFallbackTarget)|UpdateGroup_FallbackGroupFlag|ValidateFallbackGroup_RejectsChainedTarget)' -count=1
```

结果：

```text
ok  	github.com/Wei-Shaw/sub2api/internal/service	0.154s
```

```bash
cd backend
go test -tags unit ./internal/service -run '^$' -count=1
```

结果：

```text
ok  	github.com/Wei-Shaw/sub2api/internal/service	0.051s [no tests to run]
```

```bash
gofmt -w backend/internal/service/admin_service.go backend/internal/service/admin_service_group_test.go
```

结果：成功，无额外输出。
