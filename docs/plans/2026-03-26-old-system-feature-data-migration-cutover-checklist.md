# 旧系统数据迁移切换与验收清单（执行版）

> 目标：把旧系统（source）数据库的数据迁移到新系统（target），完成切流，并确保 Dashboard/定价/号池监控/叠卡等关键功能与历史数据可用。

## 0. 约定

- `source_dsn`：旧系统 Postgres DSN
- `target_dsn`：新系统 Postgres DSN
- 迁移器入口：`backend/cmd/data-migrator`
- phase（按依赖顺序）：
  - `core`: `groups`、`users`、`settings`
  - `access_control`: `user_allowed_groups`、`orphan_allowed_groups_audit`、`user_attribute_definitions`、`user_attribute_values`
  - `infrastructure`: `proxies`、`accounts`、`account_groups`
  - `api_keys`: `api_keys`
  - `subscriptions`: `user_subscriptions`、`subscription_grants`、`redeem_codes`
  - `pricing`: `pricing_plan_groups`、`pricing_plans`
  - `pool_monitor`: `account_pool_alert_configs`、`proxy_usage_metrics_hourly`
  - `usage`: `usage_logs`
  - `ops_metrics`: `ops_metrics_hourly`、`ops_metrics_daily`
  - `billing`: `billing_usage_entries`

## 1. 切换前检查（必须）

- [ ] 确认 source/target 的 schema 版本已完成迁移（新系统执行过 migrations）
- [ ] 对 source 数据库做一次完整备份（可回滚的事实源）
- [ ] 确认 target 数据库可被清空（或明确本次是否允许保留既有数据）
- [ ] 确认新系统后端启动参数与时区一致（建议 `TZ=Asia/Shanghai`）
- [ ] 确认前端 dev/prod 访问的后端地址与端口正确（dev: 3000 代理到 8080）

## 2. Dry-run 演练（建议至少 1 次）

生成对账报告（只读，不写入 target）：

```bash
cd backend
/opt/homebrew/bin/go run ./cmd/data-migrator \
  --source_dsn '...SOURCE...' \
  --target_dsn '...TARGET...' \
  --mode dry-run \
  --phase all \
  --report_file /tmp/fluxcode-migration-dry-run.json
```

也可以直接复用两套后端 `config.yaml`（避免手写 DSN）：

```bash
cd backend
/opt/homebrew/bin/go run ./cmd/data-migrator \
  --source_config /abs/path/to/old/config.yaml \
  --target_config /abs/path/to/new/config.yaml \
  --mode dry-run \
  --phase all \
  --report_file /tmp/fluxcode-migration-dry-run.json
```

- [ ] 检查 `/tmp/fluxcode-migration-dry-run.json`：各表 `source_rows/target_rows/diff`
- [ ] 若 target 非空：确认差异原因（历史残留 or 多次演练），再决定是否 `reset_target`

## 3. 正式切换执行（建议按阶段）

### 3.1 冻结旧系统写入（切流前窗口）

- [ ] 停止旧系统写入（维护窗口、只读模式或停机）
- [ ] 记录冻结时间点（用于验收口径与差异解释）

### 3.2 Apply 导入（写入 target）

全量导入（会对目标表执行 TRUNCATE；仅在确认 target 可清空时使用）：

```bash
cd backend
/opt/homebrew/bin/go run ./cmd/data-migrator \
  --source_dsn '...SOURCE...' \
  --target_dsn '...TARGET...' \
  --mode apply \
  --phase all \
  --reset_target \
  --report_file /tmp/fluxcode-migration-apply.json
```

若需要分阶段（推荐先不跑 `usage` 以缩短窗口）：

```bash
cd backend
/opt/homebrew/bin/go run ./cmd/data-migrator --source_dsn '...SOURCE...' --target_dsn '...TARGET...' --mode apply --phase core --reset_target
/opt/homebrew/bin/go run ./cmd/data-migrator --source_dsn '...SOURCE...' --target_dsn '...TARGET...' --mode apply --phase access_control --reset_target
/opt/homebrew/bin/go run ./cmd/data-migrator --source_dsn '...SOURCE...' --target_dsn '...TARGET...' --mode apply --phase infrastructure --reset_target
/opt/homebrew/bin/go run ./cmd/data-migrator --source_dsn '...SOURCE...' --target_dsn '...TARGET...' --mode apply --phase api_keys --reset_target
/opt/homebrew/bin/go run ./cmd/data-migrator --source_dsn '...SOURCE...' --target_dsn '...TARGET...' --mode apply --phase subscriptions --reset_target
/opt/homebrew/bin/go run ./cmd/data-migrator --source_dsn '...SOURCE...' --target_dsn '...TARGET...' --mode apply --phase pricing --reset_target
/opt/homebrew/bin/go run ./cmd/data-migrator --source_dsn '...SOURCE...' --target_dsn '...TARGET...' --mode apply --phase pool_monitor --reset_target
/opt/homebrew/bin/go run ./cmd/data-migrator --source_dsn '...SOURCE...' --target_dsn '...TARGET...' --mode apply --phase usage --reset_target
/opt/homebrew/bin/go run ./cmd/data-migrator --source_dsn '...SOURCE...' --target_dsn '...TARGET...' --mode apply --phase ops_metrics --reset_target
/opt/homebrew/bin/go run ./cmd/data-migrator --source_dsn '...SOURCE...' --target_dsn '...TARGET...' --mode apply --phase billing --reset_target
```

## 4. 回填与重建（如需）

说明：部分派生统计可以通过重建获得；重建策略以新系统实现为准。

- [ ] Admin Dashboard 预聚合回填（若要启用 pre-agg）：
  - 走接口：`POST /api/v1/admin/dashboard/aggregation/backfill`
  - 或按仓库现有实现改为离线回填（避免卡住 HTTP）
- [ ] 检查 `proxy_usage_metrics_hourly` 是否已迁移/可展示（号池监控与 Dashboard 代理用量图依赖）

## 5. 验收清单（必须）

### 5.1 数据口径

- [ ] 用户/分组行数一致（抽样核对）
- [ ] API Key 行数一致（抽样核对）
- [ ] 订阅与 grant：对比几个典型用户（含叠卡）在旧系统/新系统的剩余额度与到期时间
- [ ] 定价方案：PlanGroup/Plan 可在管理端页面加载、创建/编辑/启用状态正常
- [ ] 号池监控：配置页可读取并保存；监控面板能显示汇总
- [ ] Dashboard：管理员页能加载统计与趋势图（即使代理用量为空也不应整页空白）

### 5.2 功能口径

- [ ] 叠卡兑换：用户兑换订阅时出现 extend/stack 二选一逻辑与正确提示
- [ ] 管理端分配订阅：出现 extend/stack 二选一逻辑

## 6. 回滚预案（必须写清）

- [ ] 保留 source 备份与切换前快照
- [ ] 若验收失败：
  - [ ] 先切回旧系统流量
  - [ ] 新系统停写
  - [ ] 记录失败原因与差异报告，修复后再重新演练
