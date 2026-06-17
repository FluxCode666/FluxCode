# 促销活动剩余次数提示设计

## 背景

用户充值页面的促销活动列表已经通过 `/payment/promotions/available` 获取当前用户可用活动。接口返回的 `AvailablePromotion` 包含 `used_count` 与 `max_uses`，其中 `max_uses = 0` 表示不限次，`max_uses > 0` 表示当前用户仍可使用的个人次数上限。

## 目标

在充值页的促销活动区域中，如果某个促销活动存在次数限制，需要在每一项右侧显示剩余次数；订阅确认页中的促销活动列表同步展示相同提示。

## 方案

采用前端展示现有字段的轻量方案，不改后端接口、数据库、促销解析或订单创建链路。

- 当 `promo.max_uses > 0` 时，显示 `max(promo.max_uses - promo.used_count, 0)`。
- 当 `promo.max_uses <= 0` 时，不显示剩余次数提示。
- 文案使用已有国际化键 `payment.promoRemaining`，中文为“剩余 {n} 次”，英文为“{n} left”。
- 充值活动列表和订阅活动列表复用相同的计算函数，避免两处逻辑不一致。

## UI

在每个促销活动项的右侧增加一个小型 badge。左侧保留当前的选中圆点、活动名称、活动类型标签和描述；右侧 badge 使用收缩固定宽度的布局，不参与活动名称换行。长名称或窄屏下，名称区域可换行，右侧提示仍保持在该活动项右侧。

## 数据流

1. `PaymentView.vue` 调用 `paymentAPI.getAvailablePromotions` 获取充值或订阅可用活动。
2. 活动列表保存到 `availablePromotions` 或 `subAvailablePromotions`。
3. 模板渲染每个 `promo` 时调用剩余次数计算函数。
4. 仅限次活动显示提示，不限次活动保持当前 UI。

## 测试

新增或补充 `PaymentView` 组件测试：

- 充值促销列表中，`max_uses: 3`、`used_count: 1` 显示“剩余 2 次”。
- 订阅促销列表中，`max_uses: 5`、`used_count: 3` 显示“剩余 2 次”。
- `max_uses: 0` 的不限次活动不显示剩余次数提示。

## 非目标

- 不新增后端字段。
- 不改变促销活动可用性判断。
- 不改变订单预览、创建、履约和限次复查逻辑。
