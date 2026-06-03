# Admin Gift Balance User Select Design

## Goal

管理员用户列表中的余额展示包含可用赠送余额；推广管理中的手动发放和用户配置不再要求手输用户 ID，而是通过邮箱搜索选择用户。

## Requirements

- 用户列表余额主展示值为 `balance + gift_balance_remaining`。
- 用户列表仍保留原始账户余额与赠送余额拆分，避免误解后台资金口径。
- 推广管理「发放余额」tab 改名为「发放赠送余额」。
- 发放赠送余额 tab 顶部展示提示：发放余额为赠送余额，仅用于兜底操作，也会新增审计日志。
- 单个发放赠送余额通过邮箱搜索下拉选择用户，提交时继续向后端发送 `user_id`。
- 用户配置通过邮箱搜索下拉选择用户，再加载/保存该用户配置。

## Architecture

后端在管理员用户列表 DTO 上新增只读字段 `gift_balance_remaining`，由 `UserHandler.List` 使用已注入的 `GiftBalanceRepository` 批量补充。现有 `balance` 字段保持原义，不改变扣费、充值、排序或编辑逻辑。

前端复用现有 `UserSearchSelect` 组件替换数字输入。用户列表余额列使用本地计算函数展示合计，并在同一单元格中显示余额拆分。

## Data Flow

`GET /api/v1/admin/users` 返回用户基础信息与 `gift_balance_remaining`。用户列表读取 `row.balance` 和 `row.gift_balance_remaining` 计算展示值。推广管理的两个选择器只更新本地用户 ID，后续调用仍使用现有推广管理 API。

## Error Handling

赠送余额查询失败时后端不阻断用户列表，字段按 `0` 返回，并记录服务端日志。用户搜索失败沿用现有 `UserSearchSelect` 空结果/错误处理。

## Testing

- 后端单元测试覆盖管理员用户列表会返回 `gift_balance_remaining`。
- 前端用户列表测试覆盖余额展示会把基础余额和赠送余额相加。
- 前端推广管理测试覆盖 tab 文案、兜底提示、邮箱搜索选择用户后提交发放与加载用户配置。
