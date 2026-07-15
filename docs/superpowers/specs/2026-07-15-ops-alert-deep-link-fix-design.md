# 运维告警相关日志深链修复设计

## 背景

运维监控页的告警事件详情包含“查看相关日志”入口。该入口会打开带有 `open_error_details=1` 等查询参数的 `/admin/ops` 深链。当前页面在相关错误详情弹窗状态初始化前调用 `applyRouteQueryToState()`，深链解析访问仍处于 JavaScript 暂时性死区的变量，导致页面初始化失败，无法查看相关日志。

上游仓库已通过提交 `e46d2c2112b9d232c945231954f7fdc1a972909c` 修复同一问题。

## 目标

- 将首次调用 `applyRouteQueryToState()` 移到错误详情与告警规则弹窗状态声明之后。
- 保持现有深链参数、弹窗行为和日志筛选语义不变。
- 增加前端回归测试，证明带 `open_error_details=1` 的初始路由可以正常渲染并打开错误详情弹窗。

## 非目标

- 不修改告警与日志的关联条件。
- 不增加告警业务样本。
- 不接入飞书或其他通知渠道。
- 不移植上游其他运维监控改动。

## 实现设计

在 `frontend/src/views/admin/ops/OpsDashboard.vue` 中保留 `applyRouteQueryToState()` 的实现不变，仅调整首次调用位置：先声明 `showErrorDetails`、`errorDetailsType`、`showAlertRulesCard` 等深链处理依赖的响应式状态，再执行首次路由查询解析。运行期间监听 `route.query` 的现有调用保持不变。

该调整直接消除初始化顺序错误，不引入新的状态、接口或数据迁移。

## 测试设计

新增或扩展 `OpsDashboard` 组件测试：

1. 初始路由携带 `open_error_details=1&error_type=request`。
2. 挂载页面时不抛出 `ReferenceError`。
3. 错误详情弹窗收到打开状态及 `request` 类型。

随后运行该组件专项测试与前端类型检查，确认没有回归。

## 验收标准

- 从告警详情点击“查看相关日志”后，运维页面正常加载并打开请求错误详情弹窗。
- 浏览器控制台不再出现深链状态变量初始化相关的 `ReferenceError`。
- 普通 `/admin/ops` 页面及告警规则深链行为保持正常。
- 代码差异仅包含调用顺序调整与对应回归测试。
