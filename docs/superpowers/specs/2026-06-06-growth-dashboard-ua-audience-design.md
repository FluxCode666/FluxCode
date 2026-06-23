# 运营大盘 UA 用户画像设计

## 背景

运营大盘已经接入核心经营指标、增长、留存、充值和功能使用。系统当前会在 `usage_logs.user_agent` 记录请求 `User-Agent`，管理端用量明细可查看或导出原始 UA，但运营大盘没有基于 UA 的画像聚合。

## 目标

在管理端「运营大盘」新增 UA 用户画像模块，用于按统计区间观察活跃请求用户的设备、系统、浏览器和客户端类型分布。

## 范围

- 新增 4 个独立接口：
  - `GET /api/v1/admin/growth/audience/devices`
  - `GET /api/v1/admin/growth/audience/os`
  - `GET /api/v1/admin/growth/audience/browsers`
  - `GET /api/v1/admin/growth/audience/clients`
- 复用现有查询参数：`start_date`、`end_date`、`granularity`。
- 统计时区固定 `Asia/Shanghai`。
- 响应统一为 `{ "items": [...] }`。
- 每项字段：`key`、`label`、`users`、`requests`、`user_ratio`。

## 分类口径

- 设备：`desktop`、`mobile`、`tablet`、`cli`、`api`、`unknown`。
- OS：`windows`、`macos`、`linux`、`ios`、`android`、`unknown`。
- 浏览器：`chrome`、`safari`、`edge`、`firefox`、`unknown`。
- 客户端：`browser`、`codex_cli`、`claude_code`、`gemini_cli`、`sdk`、`curl`、`unknown`。

分类在后端 SQL 聚合中完成，不把原始 UA 返回到运营大盘。无法识别或为空的 UA 进入 `unknown`。

## 数据流

前端 GrowthDashboardView 使用 4 个独立状态对象分别请求设备、OS、浏览器、客户端接口。任一接口失败只显示对应卡片错误，不影响其他图表。

后端沿用现有 Growth 层级：

- Handler 解析查询区间并返回响应 envelope。
- Service 处理 nil 默认值、比例四位小数归一化。
- Repository 从 `usage_logs` 聚合，JOIN `users` 过滤软删除用户。

## 隐私与合规

运营大盘不展示原始 UA，不暴露 IP、Prompt、AI 回复、上传文件内容、图片内容或聊天记录。接口只输出聚合分类、用户数、请求数和占比。

## 非目标

- 不做 IP/地域画像。
- 不做持久化 UA 解析结果表。
- 不新增前端行为埋点。
- 不做 UA 级留存或付费贡献，本次只做活跃画像分布。

## 验收

- 后端路由测试覆盖 4 个新增路径。
- Repository 测试覆盖聚合扫描。
- Service 测试覆盖比例归一化。
- 前端 API 测试覆盖 4 个独立 endpoint。
- 页面测试覆盖新增接口随整页加载，并验证单卡片失败不影响其他卡片。
