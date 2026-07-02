# OpenAI 与 Claude 分组兜底设计

## 背景

当前分组已有两个兜底相关字段：

- `fallback_group_id`：主要用于 `claude_code_only` 分组。非 Claude Code 请求命中该分组时，会降级到此字段指向的分组。
- `fallback_group_id_on_invalid_request`：用于 prompt too long 等无效请求兜底，运行时只覆盖部分 Anthropic/Antigravity 路径。

OpenAI 分组目前只有分组内账号 failover。当前分组选不到账号，或同分组账号上游可重试错误耗尽后，会直接返回错误，不会切到备用分组。Claude 分组也缺少通用的上游失败分组兜底。

## 目标

- 复用 `fallback_group_id` 作为通用分组兜底字段。
- 保留现有 Claude `claude_code_only` 降级逻辑。
- 为 Claude 与 OpenAI HTTP 请求增加分组级兜底。
- 兜底分组必须显式启用后才能被选择。
- 兜底分组不能被用户或管理员直接绑定到 API Key。
- 废弃 `fallback_group_id_on_invalid_request` 的运行时语义，并标记下个版本移除。

## 非目标

- 不覆盖 OpenAI WebSocket 入口。
- 不支持链式兜底。
- 不支持跨平台兜底。
- 不删除 `fallback_group_id_on_invalid_request` 数据库字段，本版本仅停用运行时逻辑并加废弃标记。

## 数据模型

新增 `groups.is_fallback_group` 布尔字段，默认 `false`。

`is_fallback_group=true` 表示该分组可作为其他分组的兜底目标。一个兜底分组可以被多个入口分组引用，不做一对一限制。

分组约束：

- 只有 Claude/Anthropic 标准分组和 OpenAI 标准分组可以启用 `is_fallback_group`。
- 启用 `is_fallback_group` 的分组不能配置自己的 `fallback_group_id`。
- 启用 `is_fallback_group` 时，如果已有 API Key 直接绑定该分组，则拒绝保存，提示先迁移或解绑。
- 入口分组不能把自己设为兜底分组。
- Claude/Anthropic 入口分组只能选择 Claude/Anthropic 兜底分组。
- OpenAI 入口分组只能选择 OpenAI 兜底分组。
- 兜底目标必须是 active、standard、`is_fallback_group=true`。

## 运行时流程

运行时最多切换一次兜底分组。切到兜底分组后不再读取兜底分组自己的 `fallback_group_id`。

### Claude HTTP

覆盖 `/v1/messages` 的 Claude/Anthropic HTTP 网关路径。

保留现有 `claude_code_only` 行为：

- 如果入口分组启用 `claude_code_only`，且请求不是 Claude Code 客户端，则仍按现有逻辑改用 `fallback_group_id` 指向的分组。
- 如果未配置 `fallback_group_id`，继续返回 `ErrClaudeCodeOnly`。

新增通用兜底触发：

- 当前分组选不到可用账号时，如果入口分组配置了有效 `fallback_group_id`，切到兜底分组重新调度一次。
- 当前分组内账号 failover 耗尽，且最后错误是可重试上游错误时，切到兜底分组重试一次。
- 可重试上游错误包括 403（例如上游账号余额不足或账号级访问受限）、429、5xx、连接失败、超时等现有 `UpstreamFailoverError` 覆盖的错误。
- 如果流式响应已经写入客户端，则不再兜底，避免拼接两个上游流。
- 400 请求校验失败、401 认证失败、beta policy block 等不触发兜底。

### OpenAI HTTP

覆盖以下入口：

- `/openai/v1/responses`
- `/v1/chat/completions`
- `/v1/images/generations`
- `/v1/images/edits`
- OpenAI 分组承接的 `/v1/messages`

触发条件与 Claude 通用兜底一致：

- 当前 OpenAI 分组选不到可用账号时，切到 OpenAI 兜底分组重试一次。
- 当前分组账号 failover 耗尽，且最后错误是可重试上游错误时，切到 OpenAI 兜底分组重试一次。
- 已经开始写流时不兜底。
- WebSocket 入口不变。

## 兜底后的上下文

切到兜底分组后，系统按“直接请求兜底分组”处理：

- 克隆当前 API Key 的运行时上下文，将 `GroupID` 和 `Group` 改为兜底分组。
- 重新执行 billing eligibility 检查。
- 重新解析渠道模型映射和渠道定价。
- 使用兜底分组的倍率、定价、订阅/额度规则。
- 用量日志 `group_id` 记录兜底分组。
- 用量日志 `original_group_id` 记录触发运行时兜底前的入口原分组；未发生分组兜底时保持 `NULL`。
- 使用记录分组列展示为原分组在上、兜底分组在下，并通过缩进、连接线和“兜底”标识表达链路关系；计费和统计仍以 `group_id` 为准。
- 日志记录原始分组 ID、兜底分组 ID、触发原因和入口类型。

## API Key 可选性

用户创建或编辑 API Key 时：

- 前端分组选项过滤掉 `is_fallback_group=true` 的分组。
- 后端 `APIKeyService.Create` 和 `APIKeyService.Update` 拒绝绑定兜底分组。

管理员改绑 API Key 时：

- `AdminUpdateAPIKeyGroupID` 拒绝绑定兜底分组。
- 管理员用户允许分组配置中，过滤或拒绝兜底分组，避免用户通过 allowed groups 间接选择。

API Key 列表筛选可以保留所有分组，包括兜底分组，用于排查旧数据或迁移中状态。

## 管理端交互

分组管理新增 `启用为兜底分组` 开关：

- 仅 Claude/Anthropic 与 OpenAI 的 standard 分组可启用。
- 启用后，列表显示“兜底分组”标签。
- 启用后，该分组不能作为 API Key 分组选项。

入口分组的 `fallback_group_id` 选择器：

- Claude/Anthropic 入口只展示已启用兜底的 Claude/Anthropic standard active 分组。
- OpenAI 入口只展示已启用兜底的 OpenAI standard active 分组。
- 排除自身。
- 排除配置了自己 `fallback_group_id` 的分组。

隐藏旧的“无效请求兜底分组”配置控件。

## 旧字段废弃

`fallback_group_id_on_invalid_request` 在本版本停止参与运行时逻辑。

处理方式：

- prompt too long 等旧 invalid request 兜底统一走 `fallback_group_id`。
- 后端 DTO、service 类型、repository 映射保留字段以兼容旧数据。
- 相关字段注释标记：`Deprecated: will be removed in next version`。
- 中文注释说明：下个版本移除，不再参与运行时逻辑。
- 文档提示管理员迁移到 `fallback_group_id`，并将目标分组启用为兜底分组。

## 测试计划

后端测试：

- 只有 Claude/Anthropic 与 OpenAI 的 standard 分组可以启用 `is_fallback_group`。
- 有 API Key 直接绑定时不能启用 `is_fallback_group`。
- 入口分组只能选择同平台、active、standard、`is_fallback_group=true` 的兜底分组。
- 兜底目标不能配置自己的 `fallback_group_id`。
- 多个入口分组可以共享同一个兜底分组。
- 用户创建、用户更新、管理员改绑 API Key 时不能绑定兜底分组。
- Claude 保留 `claude_code_only` 非 Claude Code 降级。
- Claude 当前分组不可用时切到兜底分组。
- Claude 当前分组账号 failover 耗尽后切到兜底分组。
- OpenAI Responses、ChatCompletions、Images、OpenAI Messages 在选不到账号或 failover 耗尽时切到兜底分组。
- 已开始写流时不触发兜底。
- `fallback_group_id_on_invalid_request` 不再触发 prompt too long 兜底。

前端测试：

- 分组表单仅在 Claude/Anthropic 与 OpenAI standard 分组上展示兜底开关。
- 入口分组兜底选择器只展示同平台且已启用兜底的 standard active 分组。
- 用户创建/编辑 API Key 时不展示兜底分组。
- 旧“无效请求兜底分组”控件隐藏。

## 验收标准

- 管理员可以创建一个 OpenAI 兜底分组，并让多个 OpenAI 入口分组引用它。
- OpenAI 入口分组账号不可用或上游可重试错误耗尽后，自动使用兜底分组完成请求。
- Claude 支持通用分组兜底，同时原有 `claude_code_only` 降级不回退。
- 兜底请求的用量日志、倍率、渠道定价都归兜底分组。
- 用户和管理员都不能直接把 API Key 绑定到兜底分组。
- `fallback_group_id_on_invalid_request` 被标记为下个版本移除，且不再影响运行时。
