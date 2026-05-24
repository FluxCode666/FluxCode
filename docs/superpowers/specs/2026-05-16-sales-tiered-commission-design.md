# 销售梯度提成功能设计文档

> 日期：2026-05-16
> 状态：设计已确认，等待规格评审
> 范围：在现有销售佣金体系上增加按自然月累计销售额的梯度提成能力，并保持现有“下线实际消耗额度后再逐步解锁佣金”的结算模型

## 1. 背景

当前销售佣金逻辑只支持为销售用户配置一个固定提成比例 `sales_commission_rate`。当其下线用户完成充值后，系统按订单实付金额生成冻结佣金记录；当下线后续实际消耗普通余额时，系统再按消耗比例逐步解锁该笔佣金。

新的业务需求是在保留这套解锁模型的前提下，支持销售用户按“自然月累计实付金额”使用梯度提成规则计算佣金，并支持配置一个可选的最低月销售额门槛。达到门槛后，佣金不是只从超出门槛的部分开始算，而是对当月全部销售额按阶梯累进规则计算。

## 2. 已确认规则

- 梯度提成仅针对销售用户生效，非销售用户不受影响。
- 累计口径按“某销售名下全部下线用户在同一自然月内的累计实付金额”计算。
- 自然月按固定业务时区 `Asia/Shanghai` 计算，次月 1 日重新归零。
- 梯度规则采用“阶梯累进”，不是“整档覆盖”。
- 最低月销售额门槛为可选项；未配置时视为 `0`。
- 当月累计实付金额达到门槛后，对当月**全部**销售额按阶梯累进规则计算佣金，而不是只对门槛以上部分计算。
- 佣金仍沿用现有模型：只有下线实际消耗了对应充值得到的普通余额，销售佣金才逐步解锁。
- 赠送余额消耗不触发销售佣金解锁，仍然只有普通余额扣费触发解锁。
- 现有推广奖励、手动完成推广、销售佣金结算页面继续保留。
- 手动完成推广产生的销售佣金也需要纳入当月累计销售额，并走同一套梯度规则。

## 3. 方案选择

本次采用方案 A：**月度规则快照 + 复用现有销售佣金记录、解锁、结算链路**。

### 3.1 采用原因

1. 现有系统已经有完整的 `sales_commission_records`、FIFO 解锁和手工结算链路。
2. 新需求的核心变化是“佣金总额怎么计算”，而不是“佣金如何解锁、如何结算”。
3. 继续复用订单级佣金记录，可以最大程度保留现有对账方式、列表接口和结算逻辑。
4. 通过“月度规则快照 + 月内重算”，可以同时满足：
   - 自然月累计
   - 阶梯累进
   - 达到门槛后追溯本月全部销售额
   - 下线先消费、销售后跨门槛时，对历史记录立即补解锁

### 3.2 不采用的方案

- 不采用“每笔订单拆成多条梯度佣金记录”，因为会直接冲击现有 `payment_order_id` 唯一约束、明细列表和结算分摊逻辑。
- 不采用“按月聚合而非按订单记账”的方案，因为它会破坏现有“按下线消耗订单额度逐步解锁佣金”的模型。

## 4. 目标与非目标

### 4.1 目标

1. 保留固定比例佣金模式，兼容现有销售用户。
2. 为销售用户新增梯度提成模式。
3. 支持为梯度模式配置最低月销售额门槛。
4. 支持按自然月固化销售提成规则快照，避免月中改配置导致当月历史结果漂移。
5. 在月累计跨过门槛时，补算本月更早订单的佣金总额和已解锁金额。
6. 手动完成推广生成的销售佣金记录也纳入同一自然月累计。

### 4.2 非目标

1. 本次不改动销售佣金的提现方式，仍然只支持管理员人工结算。
2. 本次不实现跨月结转或滚动累计。
3. 本次不实现退款后自动追缴已结算佣金。
4. 本次不把梯度提成扩展到非销售推广奖励。

## 5. 数据模型

## 5.1 users 表扩展

保留现有字段：

- `is_sales`
- `sales_commission_rate`

新增字段：

```sql
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS sales_commission_mode VARCHAR(16) NOT NULL DEFAULT 'fixed',
    ADD COLUMN IF NOT EXISTS sales_commission_min_monthly_sales DECIMAL(20,2) NOT NULL DEFAULT 0;
```

字段语义：

- `sales_commission_mode`
  - `fixed`：固定比例模式，沿用当前行为。
  - `tiered`：梯度提成模式。
- `sales_commission_min_monthly_sales`
  - 仅在 `tiered` 模式下生效。
  - 单位为人民币，对应“至少达到多少月销售额才开始计算提成”。
  - 默认 `0` 表示无门槛。

校验规则：

- `sales_commission_mode` 只能为 `fixed` 或 `tiered`。
- `sales_commission_min_monthly_sales >= 0`。
- 当 `is_sales = true` 且 `sales_commission_mode = fixed` 时，`sales_commission_rate` 必须大于 0 且不超过 100。
- 当 `is_sales = true` 且 `sales_commission_mode = tiered` 时，允许 `sales_commission_rate = 0`，但必须至少存在一条有效梯度规则。

## 5.2 sales_commission_tiers 表

新增销售用户实时梯度配置表：

```sql
CREATE TABLE IF NOT EXISTS sales_commission_tiers (
    id BIGSERIAL PRIMARY KEY,
    sales_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    month_sales_from_cny DECIMAL(20,2) NOT NULL,
    month_sales_to_cny DECIMAL(20,2),
    commission_rate DECIMAL(8,4) NOT NULL,
    sort_order INT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sales_commission_tiers_sales_user
    ON sales_commission_tiers (sales_user_id, sort_order, id);
```

字段语义：

- `month_sales_from_cny`：本档起始累计销售额，含起点。
- `month_sales_to_cny`：本档结束累计销售额，不含终点；`NULL` 表示无上限。
- `commission_rate`：该区间对应提成比例，单位为百分比。
- `sort_order`：前端编辑顺序和后端校验顺序。

校验规则：

- `month_sales_from_cny >= 0`
- `month_sales_to_cny > month_sales_from_cny`，或为 `NULL`
- `commission_rate > 0 AND commission_rate <= 100`
- 同一销售用户的梯度区间必须从 `0` 开始，严格升序、首尾连续，且不得重叠或留空档
- 最后一档允许无上限，其他档必须显式给出 `month_sales_to_cny`

示例：

```text
0      ~ 10000   => 5%
10000  ~ 20000   => 8%
20000  ~ NULL    => 10%
```

## 5.3 sales_commission_monthly_snapshots 表

新增月度规则快照表：

```sql
CREATE TABLE IF NOT EXISTS sales_commission_monthly_snapshots (
    id BIGSERIAL PRIMARY KEY,
    sales_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    commission_month DATE NOT NULL,
    timezone VARCHAR(64) NOT NULL DEFAULT 'Asia/Shanghai',
    commission_mode VARCHAR(16) NOT NULL,
    fixed_commission_rate DECIMAL(8,4) NOT NULL DEFAULT 0,
    min_monthly_sales_cny DECIMAL(20,2) NOT NULL DEFAULT 0,
    tiers_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_sales_commission_monthly_snapshots UNIQUE (sales_user_id, commission_month)
);
```

说明：

- `commission_month` 固定存储为该自然月第一天，例如 `2026-05-01`。
- `tiers_json` 存快照时的完整梯度区间数组，不再依赖后续实时配置。
- 当某销售在某自然月第一次产生销售佣金事件时，系统创建该月快照；该月后续所有佣金记录都引用这份快照。
- 月中修改销售提成模式、固定比例、门槛或梯度配置，只影响**下一个尚未生成快照的月份**。

## 5.4 sales_commission_records 表扩展

在现有销售佣金记录表基础上新增字段：

```sql
ALTER TABLE sales_commission_records
    ADD COLUMN IF NOT EXISTS commission_month DATE,
    ADD COLUMN IF NOT EXISTS snapshot_id BIGINT REFERENCES sales_commission_monthly_snapshots(id),
    ADD COLUMN IF NOT EXISTS commission_mode VARCHAR(16) NOT NULL DEFAULT 'fixed',
    ADD COLUMN IF NOT EXISTS commission_event_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS monthly_sales_before_cny DECIMAL(20,2) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS monthly_sales_after_cny DECIMAL(20,2) NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_sales_commission_records_month
    ON sales_commission_records (sales_user_id, commission_month, commission_event_at, id);
```

字段语义：

- `commission_month`：该笔佣金归属月份，按 `Asia/Shanghai` 月份边界计算。
- `snapshot_id`：指向当月规则快照。
- `commission_mode`：记录创建时所属快照的模式，便于排查。
- `commission_event_at`：该笔销售事件在业务上的发生时间，用于确定月内累计顺序。
- `monthly_sales_before_cny`：重算后，该笔记录前的月累计销售额。
- `monthly_sales_after_cny`：重算后，该笔记录后的月累计销售额。

保留现有字段语义：

- `order_pay_amount_cny`：该笔销售事件的实付人民币。
- `order_credited_amount`：该笔事件对应的到账普通额度。
- `commission_total_cny`：该笔记录最终应得佣金总额。
- `credited_used_amount`：该笔到账普通额度中已被消耗并归因到佣金解锁的数量。
- `unlocked_cny`：已解锁佣金。
- `settled_cny`：已结算佣金。

兼容说明：

- `commission_rate` 字段继续保留。
- 在固定比例模式下，`commission_rate` 仍然是固定比例。
- 在梯度模式下，`commission_rate` 改为该条记录的**加权有效比例**，即：

```text
commission_rate = round(commission_total_cny / order_pay_amount_cny * 100, 4)
```

这样可以兼容现有列表与报表展示，不需要立刻新增专门的“有效比例”字段。

## 6. 核心业务流程

## 6.1 事件归属月份

销售佣金事件的归属月份按 `Asia/Shanghai` 计算：

- 普通充值订单：优先取 `payment_orders.paid_at`，为空时回退到 `completed_at`，再回退到当前时间。
- 推广手动完成：取管理员执行手动完成的时间。

系统将该时间转换到 `Asia/Shanghai`，再截断到当月第一天，写入 `commission_month`。
同一个时间也写入 `commission_event_at`，作为月内阶梯累计顺序的排序基准。

## 6.2 快照创建与读取

当销售佣金事件发生时：

1. 判断邀请人是否为销售用户。
2. 若 `sales_commission_mode = fixed`，继续支持现有逻辑。
3. 若 `sales_commission_mode = tiered`，先根据归属月份读取当月快照。
4. 若快照不存在，则从 `users` 和 `sales_commission_tiers` 实时配置创建快照。
5. 该月后续所有订单和手动完成都复用同一快照。

## 6.3 固定比例模式

固定比例模式不改变现有行为：

```text
commission_total_cny = round(order_pay_amount_cny * sales_commission_rate / 100, 2)
```

新增字段处理：

- `commission_month` 写入归属月份
- `snapshot_id` 指向当月快照
- `commission_mode = fixed`
- `monthly_sales_before_cny` 和 `monthly_sales_after_cny` 仍可顺手写入当月累计值，便于统一查询和后续扩展，但它们不影响固定模式计算

## 6.4 梯度模式下的新记录创建

梯度模式下，单笔销售事件仍然只创建**一条** `sales_commission_record`，但创建后会触发该月全量重算。

原因：

- 若某销售在本月前几笔订单时尚未达到门槛，这些订单最初佣金应为 `0`
- 当本月后续订单让累计销售额跨过门槛后，前几笔订单也必须被补算出佣金
- 这些补算佣金仍需挂回原来的订单记录，才能继续沿用现有“按对应订单到账额度解锁”的模型

因此，梯度模式下的处理顺序为：

1. 在事务内插入当前销售佣金记录，先写入订单、月份、快照、额度等基础信息
2. 读取该销售该月份所有 `sales_commission_records` 并 `FOR UPDATE`
3. 按记录顺序执行整月重算
4. 回写每条记录的 `monthly_sales_before_cny`、`monthly_sales_after_cny`、`commission_total_cny`、`commission_rate`、`unlocked_cny`、`status`

记录顺序定义为：

- 先按 `commission_event_at ASC`
- 若时间相同，再按 `id ASC`

## 6.5 梯度模式的整月重算算法

### 6.5.1 总体思路

定义：

- `M`：该销售该月全部销售佣金记录的 `order_pay_amount_cny` 之和
- `threshold`：快照中的最低月销售额门槛
- `P(x)`：按该月快照规则，对累计销售额 `x` 计算出的“应得佣金总额”

当 `M < threshold` 时：

- 本月所有记录的 `commission_total_cny = 0`

当 `M >= threshold` 时：

- 第 `i` 条记录的佣金总额为：

```text
commission_i = P(monthly_after_i) - P(monthly_before_i)
```

其中：

- `monthly_before_i`：该记录前的月累计销售额
- `monthly_after_i`：该记录后的月累计销售额

### 6.5.2 阶梯累进函数

假设快照梯度为：

```text
0      ~ 10000   => 5%
10000  ~ 20000   => 8%
20000  ~ NULL    => 10%
```

则：

- `P(9000) = 9000 * 5% = 450`
- `P(12000) = 10000 * 5% + 2000 * 8% = 660`
- `P(25000) = 10000 * 5% + 10000 * 8% + 5000 * 10% = 1800`

如果该月前两笔订单分别为 `9000` 和 `3000`，且门槛为 `10000`，则：

- 月总额 `12000` 已达到门槛
- 第 1 笔记录佣金：`P(9000) - P(0) = 450`
- 第 2 笔记录佣金：`P(12000) - P(9000) = 210`

这满足“达到门槛后，对当月全部销售额计算提成”的业务要求，同时仍然把佣金总额挂回原始订单。

### 6.5.3 精度规则

- `order_pay_amount_cny` 保持现有人民币精度
- `commission_total_cny` 按人民币保留两位小数
- `commission_rate` 保留四位小数
- 累计销售额区间比较按两位小数处理

## 6.6 梯度模式下的解锁补算

现有逻辑中，普通充值订单的佣金解锁仍由下线后续真实消耗普通余额触发，已经有：

- `credited_used_amount`
- `unlocked_cny`

梯度模式下，整月重算后需要同步重算每条记录的 `unlocked_cny`：

```text
unlocked_cny = round(credited_used_amount / order_credited_amount * commission_total_cny, 2)
```

若：

```text
credited_used_amount >= order_credited_amount
```

则直接校正为：

```text
unlocked_cny = commission_total_cny
```

这样可以正确覆盖以下场景：

1. 月初订单当时未达门槛，因此佣金总额为 `0`
2. 下线在月中已经消耗了部分甚至全部这笔订单对应的普通余额
3. 月后续订单让总销售额跨过门槛
4. 系统重算该月全部记录时，月初那笔订单会立刻补出应得佣金，并按照已消耗比例同步补出 `unlocked_cny`

这正是本次需求与现有解锁模型最容易出错的部分，必须在同一笔重算里一起完成。

## 6.7 使用扣费阶段的解锁逻辑

`backend/internal/repository/usage_billing_repo.go` 中的 FIFO 解锁逻辑继续保留。

变化点只有两点：

1. 它解锁的仍然是 `sales_commission_records`
2. 这些记录的 `commission_total_cny` 可能来自固定比例，也可能来自梯度月度重算

因此：

- 赠送余额优先扣减不变
- 普通余额扣费后按 FIFO 解锁佣金不变
- `unlockSalesCommissionFIFO` 只需要继续基于当前记录的 `commission_total_cny` 和 `order_credited_amount` 做比例解锁

整月重算与用量解锁的关系为：

- **新订单触发时**：可能重算该月所有记录的佣金总额和已解锁金额
- **后续扣费触发时**：继续只增量推进 `credited_used_amount` 和 `unlocked_cny`

## 6.8 手动完成推广与梯度模式

当前手动完成推广会创建没有 `payment_order_id` 的销售佣金记录，并立即可解锁。

本次要求：

1. 手动完成产生的 `order_pay_amount_cny` 也要参与当月累计销售额
2. 该记录同样归属某个 `commission_month`
3. 若销售模式为 `tiered`，这条记录也参与该月整月重算

对于手动完成记录：

- `order_credited_amount` 继续沿用当前逻辑传入
- 创建时把 `credited_used_amount` 初始化为 `order_credited_amount`
- 因为它本身已经视作立即可解锁，所以重算后：

```text
unlocked_cny = commission_total_cny
```

如果这条手动完成记录发生在门槛尚未达成之前，则它初始可能为 `0`；等到该月后续销售额跨过门槛后，它也会被补算并立即变成全额解锁。

## 7. 管理端配置与接口

## 7.1 用户编辑配置

在现有管理员用户编辑弹窗的“销售佣金”区域扩展以下字段：

- `is_sales`
- `sales_commission_mode`
- `sales_commission_rate`（仅固定模式显示）
- `sales_commission_min_monthly_sales`（仅梯度模式显示）
- `sales_commission_tiers`（仅梯度模式显示）

交互建议：

- 用分段控件或单选切换 `固定比例` / `梯度提成`
- 梯度模式下用表格式编辑器维护区间与比例
- 支持新增、删除、调整顺序

## 7.2 管理端接口

优先复用现有用户更新接口：

```http
PUT /api/v1/admin/users/:id
```

新增请求字段：

```json
{
  "is_sales": true,
  "sales_commission_mode": "tiered",
  "sales_commission_rate": 0,
  "sales_commission_min_monthly_sales": 10000,
  "sales_commission_tiers": [
    {
      "month_sales_from_cny": 0,
      "month_sales_to_cny": 10000,
      "commission_rate": 5
    },
    {
      "month_sales_from_cny": 10000,
      "month_sales_to_cny": 20000,
      "commission_rate": 8
    },
    {
      "month_sales_from_cny": 20000,
      "month_sales_to_cny": null,
      "commission_rate": 10
    }
  ]
}
```

后端处理原则：

1. 用户基本字段更新与梯度配置同步放在同一事务内完成
2. `fixed` 模式下删除该销售的实时梯度配置
3. `tiered` 模式下严格校验梯度区间合法性后再全量同步
4. 本月已存在快照时，不回写历史快照，只影响后续月份

## 7.3 查询接口与 DTO

管理端需要能回显销售提成配置，因此用户详情或用户列表 DTO 需要扩展：

- `sales_commission_mode`
- `sales_commission_min_monthly_sales`
- `sales_commission_tiers`

为了减少列表负载，优先建议在“打开编辑弹窗时读取用户详情”或新增单独的配置读取接口；如果前端当前强依赖列表行数据，也可以把这些字段加入管理员用户列表 DTO。具体以实现阶段对现有前端结构的评估为准，但后端数据模型必须支持完整回显。

## 8. 服务层与仓储层改造点

## 8.1 服务层

重点改造：

- `backend/internal/service/sales_commission_service.go`
- `backend/internal/service/admin_service.go`
- `backend/internal/service/referral_service.go`

新增职责：

1. 解析销售提成模式与配置
2. 获取或创建月度快照
3. 创建梯度模式下的基础佣金记录
4. 整月重算指定销售、指定月份的佣金记录

建议新增内部能力：

- `GetOrCreateMonthlySnapshot`
- `RepriceMonthlyCommissionRecords`
- `BuildTieredCommissionCurve`
- `RecomputeUnlockedAmount`

## 8.2 仓储层

重点改造：

- `backend/internal/repository/sales_commission_repo.go`
- 新增 `sales_commission_tier_repo.go`，或把 tier CRUD 合并进现有 sales commission repository

需要新增的仓储能力：

1. 读取/同步销售实时梯度配置
2. 获取或插入月度快照
3. 锁定某销售某月份的全部佣金记录
4. 批量回写重算结果

## 8.3 兼容现有结算与报表

现有汇总与明细查询主要依赖：

- `commission_total_cny`
- `unlocked_cny`
- `settled_cny`
- `status`

只要整月重算把这些字段维护正确，现有：

- 销售佣金汇总
- 销售佣金明细
- 管理员手动结算

都可以继续复用，不需要重做整套报表接口。

## 9. 状态与幂等

## 9.1 幂等要求

- 普通充值订单仍然依赖 `payment_order_id` 唯一约束确保幂等
- 手动完成记录继续沿用当前“允许 `payment_order_id = NULL`”的设计
- 月度快照通过 `(sales_user_id, commission_month)` 唯一约束确保幂等

## 9.2 重算事务要求

梯度模式下，单笔销售事件处理必须在一个事务内完成：

1. 插入当前基础记录
2. 锁定该月全部相关记录
3. 读取快照
4. 完成整月重算
5. 回写金额与状态

避免留下“当前订单已插入，但当月历史记录还没补算”的中间状态。

## 10. 测试范围

本次必须补齐以下测试：

1. 固定比例模式回归测试，确保历史功能不变
2. 梯度模式下单月未达门槛，所有记录佣金为 `0`
3. 梯度模式下跨门槛后，前序记录被正确补算
4. 单笔订单跨越多个梯度区间时，佣金计算正确
5. 次月自然归零，新的月份重新累计
6. 下线先消耗额度、后跨门槛时，`unlocked_cny` 被正确补算
7. 手动完成推广生成的销售佣金记录也能纳入当月累计
8. 现有汇总、明细、结算逻辑在梯度模式下仍然正确
9. 用户编辑接口的梯度配置校验覆盖：
   - 区间重叠
   - 区间断裂
   - 最后一档无上限
   - 比例越界
   - 非销售用户提交梯度配置

## 11. 实现影响面

预计需要修改的主要文件：

- `backend/internal/service/sales_commission_service.go`
- `backend/internal/service/sales_commission.go`
- `backend/internal/service/admin_service.go`
- `backend/internal/repository/sales_commission_repo.go`
- `backend/internal/repository/usage_billing_repo.go`
- `backend/internal/handler/admin/user_handler.go`
- `backend/internal/handler/dto/types.go`
- `frontend/src/components/admin/user/UserEditModal.vue`
- `frontend/src/components/admin/user/__tests__/UserEditModalSales.spec.ts`
- `frontend/src/types/index.ts`
- 相关迁移文件与仓储/服务测试

## 12. 迁移策略

### 12.1 数据迁移

1. 为 `users` 增加新字段，默认 `sales_commission_mode = 'fixed'`
2. 为 `sales_commission_records` 增加月份、快照和累计字段
3. 新建实时梯度配置表和月度快照表
4. 为历史 `sales_commission_records` 回填：
   - `commission_mode = 'fixed'`
   - `commission_month` 按历史 `created_at` 转换到 `Asia/Shanghai` 后取当月第一天
   - `commission_event_at` 优先按关联 `payment_orders.paid_at / completed_at` 回填，手动记录回退到 `created_at`

### 12.2 行为兼容

- 所有历史销售用户默认仍是固定比例模式
- 未配置梯度的销售用户不会发生行为变化
- 梯度模式只对显式切换后的销售用户生效

## 13. 风险与控制

### 13.1 风险

1. 月内跨门槛重算会更新历史记录，容易影响已展示过的汇总数字
2. 已经部分解锁的记录在补算时，`unlocked_cny` 会跳增
3. 前端用户编辑弹窗需要处理更复杂的校验与交互

### 13.2 控制

1. 使用月度快照，防止月中改配置导致历史结果反复漂移
2. 把整月重算与当前事件创建放在同一事务内
3. 先以固定模式默认值迁移，避免影响存量销售
4. 通过测试覆盖“先消费后跨门槛”的补解锁场景

## 14. 结论

本次功能应当在**不推翻现有销售佣金账本和解锁逻辑**的前提下完成。核心做法是：

1. 为销售用户增加 `fixed / tiered` 两种提成模式
2. 为梯度模式增加最低月销售额门槛和阶梯配置
3. 按 `Asia/Shanghai` 自然月固化规则快照
4. 在梯度模式下，每次新增销售事件后对该月全部佣金记录执行重算
5. 重算时同时补算佣金总额与已解锁金额，确保与“下线实际消耗额度后解锁佣金”的现有模型兼容

这样可以满足业务方要求的“按自然月累计销售额做阶梯累进提成”，同时尽量让现有报表、结算、解锁逻辑继续成立。
