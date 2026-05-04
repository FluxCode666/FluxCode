# 销售佣金功能设计文档

> 日期：2026-05-05
> 状态：设计已确认，等待实现计划
> 范围：在现有推广邀请、充值、用量扣费链路上增加销售佣金冻结、解冻和人工结算能力

## 1. 目标

允许管理员把某些用户标记为销售人员，并为每个销售设置独立提成比例。销售通过现有推广码邀请用户注册后，被邀请人充值会为销售生成一笔人民币佣金。该佣金初始冻结，只有被邀请人实际消耗其充值得到的普通站内余额时，才按消耗比例逐步解冻。解冻后的金额由管理员手动结算；第一版不提供销售自助提现。

示例：

- 被邀请人实际支付人民币 10 元，到账站内余额 10 刀。
- 销售提成比例为 10%。
- 充值成功后销售获得冻结佣金人民币 1 元。
- 被邀请人后续消耗普通余额 2 刀后，解冻 `2 / 10 * 1 = 0.2` 元。

## 2. 已确认规则

- 销售身份在管理端用户编辑中指定。
- 每个销售用户拥有独立提成比例，例如 10%。
- 佣金冻结金额按充值订单的实际支付人民币 `payment_orders.pay_amount` 计算。
- 佣金解冻进度按充值订单的实际到账站内额度 `payment_orders.amount` 计算。
- 销售佣金与现有推广奖励同时生效，互不替代。
- 现有推广奖励仍使用 `gift_balance_records` 赠送余额。
- 销售佣金使用独立账本，不复用赠送余额。
- 第一版只做管理端人工结算，不做销售自助提现。
- 销售用户可以查看自己的佣金汇总和明细；非销售用户不展示入口。
- 赠送余额消耗不触发销售佣金解冻。只有实际扣除普通余额的用量成本触发解冻。
- 多笔充值佣金按 FIFO 归因：先解冻最早一笔未完全归因的销售佣金记录。

## 3. 方案选择

采用方案 A：独立销售佣金账本，挂在现有邀请关系和充值订单上。

不选择扩展 `referrals` 表，因为一条邀请关系会对应多次充值、多次解冻和多次结算，仅靠累计字段无法满足明细和对账。

不选择复用 `gift_balance_records`，因为赠送余额是站内额度，销售佣金是人民币可结算金额，两者的冻结、解冻、结算语义不同。

## 4. 数据模型

### 4.1 users 表扩展

新增字段：

```sql
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS is_sales BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS sales_commission_rate DECIMAL(8,4) NOT NULL DEFAULT 0;
```

字段语义：

- `is_sales`：是否为销售人员。
- `sales_commission_rate`：销售提成比例，单位是百分比。例如 `10` 表示 10%。

校验规则：

- `sales_commission_rate` 必须在 `[0, 100]`。
- 当 `is_sales = true` 时，管理端 UI 要求比例大于 0。
- 修改比例只影响后续新充值订单；已生成的佣金记录保留创建时的比例快照。
- 关闭销售身份不影响已有佣金记录继续解冻和结算，但不会再生成新的销售佣金。

### 4.2 sales_commission_records 表

新增销售佣金主账本：

```sql
CREATE TABLE IF NOT EXISTS sales_commission_records (
    id BIGSERIAL PRIMARY KEY,
    sales_user_id BIGINT NOT NULL REFERENCES users(id),
    referee_user_id BIGINT NOT NULL REFERENCES users(id),
    referral_id BIGINT NOT NULL REFERENCES referrals(id),
    payment_order_id BIGINT NOT NULL REFERENCES payment_orders(id),

    order_pay_amount_cny DECIMAL(20,2) NOT NULL,
    order_credited_amount DECIMAL(20,8) NOT NULL,
    commission_rate DECIMAL(8,4) NOT NULL,
    commission_total_cny DECIMAL(20,2) NOT NULL,

    credited_used_amount DECIMAL(20,8) NOT NULL DEFAULT 0,
    unlocked_cny DECIMAL(20,2) NOT NULL DEFAULT 0,
    settled_cny DECIMAL(20,2) NOT NULL DEFAULT 0,

    status VARCHAR(32) NOT NULL DEFAULT 'frozen',
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_sales_commission_order UNIQUE (payment_order_id)
);

CREATE INDEX IF NOT EXISTS idx_sales_commission_sales_user
    ON sales_commission_records (sales_user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_sales_commission_referee
    ON sales_commission_records (referee_user_id, id);

CREATE INDEX IF NOT EXISTS idx_sales_commission_status
    ON sales_commission_records (status);
```

金额字段语义：

- 冻结金额不单独存储，计算为 `commission_total_cny - unlocked_cny`。
- 可结算金额计算为 `unlocked_cny - settled_cny`，但仅当关联订单状态仍为 `completed` 时计入。
- `credited_used_amount` 表示该充值订单到账额度中已经被佣金系统归因消耗的数量。

状态枚举：

- `frozen`：尚未解冻。
- `partial_unlocked`：部分解冻。
- `unlocked`：全部解冻但未全部结算。
- `settled`：已全部结算。
- `settlement_blocked`：关联订单处于退款相关状态，暂不计入可结算。

### 4.3 sales_commission_settlements 表

新增结算批次表：

```sql
CREATE TABLE IF NOT EXISTS sales_commission_settlements (
    id BIGSERIAL PRIMARY KEY,
    sales_user_id BIGINT NOT NULL REFERENCES users(id),
    amount_cny DECIMAL(20,2) NOT NULL,
    note TEXT NOT NULL DEFAULT '',
    created_by BIGINT REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sales_commission_settlements_sales_user
    ON sales_commission_settlements (sales_user_id, created_at DESC);
```

### 4.4 sales_commission_settlement_items 表

新增结算明细表，用于把一次人工结算分摊到具体佣金记录：

```sql
CREATE TABLE IF NOT EXISTS sales_commission_settlement_items (
    id BIGSERIAL PRIMARY KEY,
    settlement_id BIGINT NOT NULL REFERENCES sales_commission_settlements(id) ON DELETE CASCADE,
    commission_record_id BIGINT NOT NULL REFERENCES sales_commission_records(id),
    amount_cny DECIMAL(20,2) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sales_commission_settlement_items_record
    ON sales_commission_settlement_items (commission_record_id);
```

## 5. 核心流程

### 5.1 管理员指定销售

管理端用户编辑接口增加：

- `is_sales`
- `sales_commission_rate`

管理端用户列表和详情返回同名字段。用户编辑弹窗增加“销售人员”开关和“提成比例”输入框。

### 5.2 充值成功创建冻结佣金

触点：`backend/internal/service/payment_fulfillment.go` 中余额充值履约成功后。

流程：

1. 余额充值订单完成现有 `Redeem` 和 `markCompleted`。
2. 现有推广奖励继续执行。
3. 查询被充值用户是否存在 `referrals` 记录。
4. 查询 `referrals.referrer_id` 对应用户是否 `is_sales = true` 且 `sales_commission_rate > 0`。
5. 若满足条件，创建 `sales_commission_records`。
6. `payment_order_id` 唯一约束保证订单级幂等。

佣金计算：

```text
commission_total_cny = round(payment_order.pay_amount * sales_commission_rate / 100, 2)
order_credited_amount = payment_order.amount
```

如果 `payment_order.amount <= 0` 或 `commission_total_cny <= 0`，不创建记录。

### 5.3 用量扣费触发佣金解冻

触点：`backend/internal/repository/usage_billing_repo.go` 中 `deductUsageBillingBalance`。

现有扣费顺序是先扣赠送余额，再扣普通余额。销售佣金只根据本次实际扣除的普通余额 `remainingCost` 解冻。

流程：

1. 在 usage billing 事务内完成现有扣费。
2. 如果 `remainingCost <= 0`，不解冻佣金。
3. 查询该用户作为被邀请人的未完全归因销售佣金记录。
4. 按 `id ASC` FIFO 顺序分摊本次普通余额消耗。
5. 对每条记录计算可归因额度：

```text
available_credit = order_credited_amount - credited_used_amount
allocated_credit = min(remainingCost_left, available_credit)
unlock_delta = round(allocated_credit / order_credited_amount * commission_total_cny, 2)
```

6. 更新 `credited_used_amount` 和 `unlocked_cny`。
7. 如果记录已经完全归因，即 `credited_used_amount >= order_credited_amount`，把 `unlocked_cny` 校正为 `commission_total_cny`，避免分次四舍五入留下尾差。
8. 根据金额更新状态。

幂等性：

- `usage_billing_repo.Apply` 已通过 `usage_billing_dedup` 对请求计费做幂等。
- 佣金解冻放在同一个事务里，只有实际应用计费时才执行。

### 5.4 管理员人工结算

管理端提交销售用户、结算金额和备注。

流程：

1. 计算销售的可结算金额：`SUM(unlocked_cny - settled_cny)`，仅包含关联订单状态为 `completed` 的记录。
2. 如果结算金额大于可结算金额，拒绝。
3. 创建 `sales_commission_settlements`。
4. 按佣金记录 FIFO 分摊结算金额。
5. 写入 `sales_commission_settlement_items`。
6. 更新每条佣金记录的 `settled_cny` 和状态。

第一版不接入外部打款渠道，不做销售自助提现。

### 5.5 退款边界

第一版采用保守结算策略：

- 关联订单状态不是 `completed` 时，该佣金记录不计入可结算金额。
- 管理端和销售端明细展示关联订单状态。
- 已经人工结算后的订单如果后续退款，本期不自动追缴，也不自动生成负佣金。
- 后续可以单独增加退款冲销能力。

## 6. 接口设计

### 6.1 管理端用户接口扩展

现有用户 DTO 增加：

```json
{
  "is_sales": true,
  "sales_commission_rate": 10
}
```

现有用户更新请求支持这两个字段。

### 6.2 管理端销售佣金接口

新增接口组：`/api/v1/admin/sales-commissions`

- `GET /summary`：销售汇总列表，支持搜索销售用户、分页。
- `GET /records`：佣金明细列表，支持按销售、被邀请人、订单、状态筛选。
- `GET /settlements`：结算记录列表。
- `POST /settlements`：人工结算。

汇总字段：

- `sales_user_id`
- `sales_email`
- `sales_username`
- `frozen_cny`
- `unlocked_cny`
- `settleable_cny`
- `settled_cny`
- `total_commission_cny`

明细字段：

- `id`
- `sales_user_id`
- `referee_user_id`
- `payment_order_id`
- `order_pay_amount_cny`
- `order_credited_amount`
- `credited_used_amount`
- `commission_rate`
- `commission_total_cny`
- `frozen_cny`
- `unlocked_cny`
- `settled_cny`
- `settleable_cny`
- `status`
- `payment_order_status`
- `created_at`
- `updated_at`

### 6.3 用户端销售佣金接口

新增接口组：`/api/v1/sales-commissions`

- `GET /summary`：当前销售用户的佣金汇总。
- `GET /records`：当前销售用户的佣金明细。

非销售用户访问返回空汇总和空列表，前端默认隐藏入口。

## 7. 前端设计

### 7.1 管理端用户编辑

在 `frontend/src/components/admin/user/UserEditModal.vue` 增加：

- “销售人员”开关。
- “提成比例”数字输入，单位 `%`。
- 当销售人员开关开启时，提成比例必填且大于 0。

用户列表角色徽标旁可增加轻量销售标记，便于识别。

### 7.2 管理端销售佣金页面

新增页面放在推广管理附近，侧边栏名称为“销售佣金”。

页面包含：

- 汇总卡片：总冻结、总已解冻、总可结算、总已结算。
- 销售汇总表：按销售用户聚合。
- 佣金明细表：按订单查看冻结、解冻、结算进度。
- 结算操作弹窗：输入金额和备注，提交后刷新汇总。

### 7.3 用户端销售佣金页面

仅销售用户展示入口。

页面包含：

- 当前销售的冻结、已解冻、可结算、已结算金额。
- 佣金明细表。
- 不提供提现按钮。

## 8. 错误处理与一致性

- 创建佣金记录失败不影响充值履约，但必须记录错误日志；后续可以通过运维脚本补偿。
- 解冻佣金失败应使 usage billing 事务失败，因为扣费和解冻必须保持一致。
- 人工结算使用数据库事务，结算批次、结算明细和佣金记录更新必须同时成功。
- 所有金额计算使用 `shopspring/decimal`，不使用浮点直接做人民币金额计算。
- 人民币金额保留 2 位小数，站内额度保留 8 位小数。
- 解冻最终一步校正尾差，保证全额消耗后 `unlocked_cny = commission_total_cny`。

## 9. 测试计划

后端测试：

- 管理员可以设置销售身份和提成比例。
- 非销售邀请人充值不创建销售佣金。
- 销售邀请人充值创建冻结佣金。
- 销售佣金使用 `pay_amount` 计算冻结金额，使用 `amount` 计算解冻进度。
- 赠送余额消耗不触发销售佣金解冻。
- 普通余额消耗按比例解冻。
- 多笔充值按 FIFO 解冻。
- 重复支付履约不会重复创建佣金记录。
- 重复 usage billing 不会重复解冻。
- 管理员结算金额不能超过可结算金额。
- 结算按 FIFO 写入 settlement items 并更新 `settled_cny`。
- 退款相关订单不计入可结算金额。

前端测试：

- 用户编辑弹窗正确提交 `is_sales` 和 `sales_commission_rate`。
- 非销售用户不显示用户端“销售佣金”入口。
- 销售用户显示汇总和明细。
- 管理端销售佣金页面能筛选、分页、打开结算弹窗。

## 10. 不在第一版范围

- 销售自助提现。
- 外部打款渠道集成。
- 已结算佣金的退款自动追缴。
- 多级销售分佣。
- 按订阅额度消耗解冻销售佣金。
- 按赠送余额消耗解冻销售佣金。

## 11. 主要改动位置

后端：

- `backend/ent/schema/user.go`
- `backend/migrations/111_sales_commissions.sql`
- `backend/internal/service/user.go`
- `backend/internal/handler/dto/types.go`
- `backend/internal/handler/dto/mappers.go`
- `backend/internal/service/admin_service.go`
- `backend/internal/handler/admin/user_handler.go`
- `backend/internal/service/payment_fulfillment.go`
- `backend/internal/repository/usage_billing_repo.go`
- 新增 `backend/internal/service/sales_commission.go`
- 新增 `backend/internal/service/sales_commission_service.go`
- 新增 `backend/internal/repository/sales_commission_repo.go`
- 新增管理端和用户端 handler 与 routes

前端：

- `frontend/src/types/index.ts`
- `frontend/src/api/admin/users.ts`
- `frontend/src/components/admin/user/UserEditModal.vue`
- `frontend/src/views/admin/UsersView.vue`
- 新增管理端销售佣金 API 和页面
- 新增用户端销售佣金 API 和页面
- 路由和侧边栏菜单配置

## 12. 验收标准

- 管理员能把用户设置为销售，并保存独立提成比例。
- 销售邀请用户充值后，生成冻结人民币佣金。
- 被邀请人消耗普通余额后，销售佣金按充值到账额度比例解冻。
- 解冻后的可结算金额能被管理员手动结算。
- 销售用户能看到自己的佣金汇总和明细。
- 非销售用户看不到销售佣金入口。
- 现有推广奖励、充值、赠送余额、普通余额扣费逻辑保持兼容。
