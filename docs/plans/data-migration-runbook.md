# 数据迁移操作手册（照做版）

> 本文档从设计方案、执行进度与切换清单中提炼而来，目标是让你从头到尾照做即可完成旧系统到新系统的数据迁移。

---

## 一、前置条件

在开始之前，逐项确认：

- [ ] **新系统数据库 migrations 已全部执行**（包括 `081_subscription_grants_stack_mode.sql`）
- [ ] **旧系统数据库已做完整备份**（`pg_dump` 或快照，用于回滚）
- [ ] **确认新系统目标库可被清空**（首次迁移推荐 `--reset_target`；若不可清空则走增量模式）
- [ ] **新旧系统时区一致**（建议 `TZ=Asia/Shanghai`）
- [ ] **旧系统 `config.yaml` 路径已知**（默认：`/Volumes/T7/project/FluxCode/backend/config.yaml`）
- [ ] **新系统 `config.yaml` 路径已知**（默认：`/Volumes/T7/project/new/FluxCode/backend/config.yaml`）

---

## 二、了解迁移器

- **入口**：`backend/cmd/data-migrator/main.go`
- **运行目录**：`backend/`

### CLI 参数

| 参数 | 说明 | 示例 |
|---|---|---|
| `--source_dsn` | 旧系统 Postgres DSN | `postgres://user:pass@host:5432/olddb?sslmode=disable` |
| `--target_dsn` | 新系统 Postgres DSN | `postgres://user:pass@host:5432/newdb?sslmode=disable` |
| `--source_config` | 旧系统 `config.yaml` 路径（可替代 `source_dsn`） | `/Volumes/T7/project/FluxCode/backend/config.yaml` |
| `--target_config` | 新系统 `config.yaml` 路径（可替代 `target_dsn`） | `/Volumes/T7/project/new/FluxCode/backend/config.yaml` |
| `--mode` | `dry-run`（只读对账）或 `apply`（实际写入） | `dry-run` |
| `--phase` | 迁移阶段，见下表 | `all` |
| `--reset_target` | apply 时先 TRUNCATE 目标表（首次推荐开启） | 加上即生效 |
| `--report_file` | JSON 报告输出路径 | `/tmp/migration-report.json` |

### Phase 阶段与对应表

按依赖顺序执行，**不可乱序**：

| 序号 | Phase | 包含表 |
|---|---|---|
| 1 | `core` | `groups`、`users`、`settings` |
| 2 | `access_control` | `user_allowed_groups`、`orphan_allowed_groups_audit`、`user_attribute_definitions`、`user_attribute_values` |
| 3 | `infrastructure` | `proxies`、`accounts`、`account_groups` |
| 4 | `api_keys` | `api_keys` |
| 5 | `subscriptions` | `user_subscriptions`、`subscription_grants`、`redeem_codes` |
| 6 | `pricing` | `pricing_plan_groups`、`pricing_plans` |
| 7 | `pool_monitor` | `account_pool_alert_configs`、`proxy_usage_metrics_hourly` |
| 8 | `usage` | `usage_logs`（大表，耗时最长） |
| 9 | `ops_metrics` | `ops_metrics_hourly`、`ops_metrics_daily` |
| 10 | `billing` | `billing_usage_entries` |

> 使用 `--phase all` 会按上述顺序依次执行全部阶段。

---

## 三、Step 1：Dry-Run 预演（只读，不写入）

> 目的：验证连接、确认各表行数差异，不会修改任何数据。**至少执行 1 次。**

### 方式 A：用 config.yaml（推荐）

```bash
cd /Volumes/T7/project/new/FluxCode/backend

/opt/homebrew/bin/go run ./cmd/data-migrator \
  --source_config /Volumes/T7/project/FluxCode/backend/config.yaml \
  --target_config /Volumes/T7/project/new/FluxCode/backend/config.yaml \
  --mode dry-run \
  --phase all \
  --report_file /tmp/migration-dry-run.json
```

### 方式 B：用 DSN

> 适用于旧库 / 新库在远程服务器或 Docker 中、本地没有对应 `config.yaml` 的场景。
> **密码含特殊字符**时需先做 URL 编码，详见[附录：DSN 密码特殊字符处理](#q-dsn-密码含特殊字符-如--怎么办)。

```bash
cd /Volumes/T7/project/new/FluxCode/backend

# 先用 Python 对密码做 URL 编码（特殊字符如 % @ # 等必须编码）
# python3 -c "import urllib.parse; print(urllib.parse.quote('你的原始密码', safe=''))"

/opt/homebrew/bin/go run ./cmd/data-migrator \
  --source_dsn 'postgres://USER:ENCODED_PASS@HOST:5432/OLD_DB?sslmode=disable' \
  --target_dsn 'postgres://USER:ENCODED_PASS@HOST:5432/NEW_DB?sslmode=disable' \
  --mode dry-run \
  --phase all \
  --report_file /tmp/migration-dry-run.json
```

### 检查报告

```bash
cat /tmp/migration-dry-run.json | python3 -m json.tool
```

重点看每个表的 `source_rows`（旧库行数）和 `target_rows`（新库行数）、`diff`（差异）。

- **如果 target 全是 0** → 正常（首次迁移）
- **如果 target 非 0** → 说明目标库已有数据，后续 apply 时需决定是否 `--reset_target`

---

## 四、Step 2：冻结旧系统

> 在正式 apply 之前，**必须**停止旧系统的写入，防止迁移期间数据不一致。

- [ ] 停止旧系统后端服务（或切为只读/维护模式）
- [ ] 记录冻结时间点：`________`（用于后续验收口径）

---

## 五、Step 3：正式 Apply（写入新库）

### 方式 A：一次性全量 — config.yaml（简单快速）

```bash
cd /Volumes/T7/project/new/FluxCode/backend

/opt/homebrew/bin/go run ./cmd/data-migrator \
  --source_config /Volumes/T7/project/FluxCode/backend/config.yaml \
  --target_config /Volumes/T7/project/new/FluxCode/backend/config.yaml \
  --mode apply \
  --phase all \
  --reset_target \
  --report_file /tmp/migration-apply.json
```

### 方式 B：分阶段执行 — config.yaml（推荐，可控性更强）

逐阶段执行，每阶段完成后可检查输出确认无误再继续。

> 将下面的 `SOURCE_CFG` 和 `TARGET_CFG` 替换为实际路径，或改用 `--source_dsn` / `--target_dsn`。

```bash
cd /Volumes/T7/project/new/FluxCode/backend

SOURCE_CFG=/Volumes/T7/project/FluxCode/backend/config.yaml
TARGET_CFG=/Volumes/T7/project/new/FluxCode/backend/config.yaml

# 1. 核心：分组、用户、设置
/opt/homebrew/bin/go run ./cmd/data-migrator --source_config $SOURCE_CFG --target_config $TARGET_CFG --mode apply --phase core --reset_target

# 2. 访问控制
/opt/homebrew/bin/go run ./cmd/data-migrator --source_config $SOURCE_CFG --target_config $TARGET_CFG --mode apply --phase access_control --reset_target

# 3. 基础设施：代理、账号
/opt/homebrew/bin/go run ./cmd/data-migrator --source_config $SOURCE_CFG --target_config $TARGET_CFG --mode apply --phase infrastructure --reset_target

# 4. API Key
/opt/homebrew/bin/go run ./cmd/data-migrator --source_config $SOURCE_CFG --target_config $TARGET_CFG --mode apply --phase api_keys --reset_target

# 5. 订阅与叠卡
/opt/homebrew/bin/go run ./cmd/data-migrator --source_config $SOURCE_CFG --target_config $TARGET_CFG --mode apply --phase subscriptions --reset_target

# 6. 定价
/opt/homebrew/bin/go run ./cmd/data-migrator --source_config $SOURCE_CFG --target_config $TARGET_CFG --mode apply --phase pricing --reset_target

# 7. 号池监控
/opt/homebrew/bin/go run ./cmd/data-migrator --source_config $SOURCE_CFG --target_config $TARGET_CFG --mode apply --phase pool_monitor --reset_target

# 8. 使用日志（大表，耗时较长，请耐心等待）
/opt/homebrew/bin/go run ./cmd/data-migrator --source_config $SOURCE_CFG --target_config $TARGET_CFG --mode apply --phase usage --reset_target

# 9. 运营指标
/opt/homebrew/bin/go run ./cmd/data-migrator --source_config $SOURCE_CFG --target_config $TARGET_CFG --mode apply --phase ops_metrics --reset_target

# 10. 计费
/opt/homebrew/bin/go run ./cmd/data-migrator --source_config $SOURCE_CFG --target_config $TARGET_CFG --mode apply --phase billing --reset_target
```

### 方式 C：用 DSN（数据库在远程/Docker，本地无 config.yaml）

> 适用场景：旧库和新库分别在不同服务器的 Docker 容器中，本地无法指定 `config.yaml`。
> 前提：本机能通过网络连通两端 PostgreSQL（确认 Docker 已映射端口，如 `-p 5432:5432`）。

#### 准备 DSN

```bash
# 1. 对密码做 URL 编码（特殊字符如 % @ # 等必须编码）
python3 -c "import urllib.parse; print(urllib.parse.quote('旧库密码', safe=''))"
python3 -c "import urllib.parse; print(urllib.parse.quote('新库密码', safe=''))"

# 2. 设置变量（用单引号包裹，避免 shell 二次解释）
SOURCE_DSN='postgres://旧库用户:编码后密码@旧库IP:5432/旧库名?sslmode=disable'
TARGET_DSN='postgres://新库用户:编码后密码@新库IP:5432/新库名?sslmode=disable'
```

#### C-1. 一次性全量

```bash
cd /Volumes/T7/project/new/FluxCode/backend

/opt/homebrew/bin/go run ./cmd/data-migrator \
  --source_dsn "$SOURCE_DSN" \
  --target_dsn "$TARGET_DSN" \
  --mode apply \
  --phase all \
  --reset_target \
  --report_file /tmp/migration-apply.json
```

#### C-2. 分阶段执行（推荐）

```bash
cd /Volumes/T7/project/new/FluxCode/backend

for PHASE in core access_control infrastructure api_keys subscriptions pricing pool_monitor usage ops_metrics billing; do
  echo "=== Migrating phase: $PHASE ==="
  /opt/homebrew/bin/go run ./cmd/data-migrator \
    --source_dsn "$SOURCE_DSN" \
    --target_dsn "$TARGET_DSN" \
    --mode apply \
    --phase "$PHASE" \
    --reset_target \
    --report_file "/tmp/migration-apply-${PHASE}.json"
  echo "=== Phase $PHASE done ==="
done
```

### Apply 完成后，再跑一次 dry-run 验证

**config.yaml 模式：**

```bash
/opt/homebrew/bin/go run ./cmd/data-migrator \
  --source_config $SOURCE_CFG --target_config $TARGET_CFG \
  --mode dry-run --phase all \
  --report_file /tmp/migration-verify.json
```

**DSN 模式：**

```bash
/opt/homebrew/bin/go run ./cmd/data-migrator \
  --source_dsn "$SOURCE_DSN" --target_dsn "$TARGET_DSN" \
  --mode dry-run --phase all \
  --report_file /tmp/migration-verify.json
```

检查报告中所有表的 `diff` 应为 **0**。

---

## 六、Step 4：回填衍生聚合表

以下表不从旧库直接搬运，而是从已导入的原始数据重建：

- `usage_dashboard_hourly` / `usage_dashboard_daily`
- `usage_dashboard_hourly_users` / `usage_dashboard_daily_users`
- `usage_dashboard_aggregation_watermark`
- `usage_billing_dedup` / `usage_billing_dedup_archive`

### 方式 A：通过 HTTP 接口（数据量小时可用）

启动新系统后端，然后调用回填接口：

```bash
# 启动新系统后端（如尚未启动）
# 然后调用 Dashboard 预聚合回填
curl -X POST http://localhost:8080/api/v1/admin/dashboard/aggregation/backfill \
  -H "Authorization: Bearer <YOUR_ADMIN_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"start":"2026-01-01T00:00:00Z","end":"2026-04-07T00:00:00Z"}'
```

> 注意：HTTP 接口有超时限制（默认 30 分钟），数据量大时可能超时。

### 方式 B：离线脚本（推荐，无超时限制）

**入口**：`backend/cmd/dashboard-backfill/main.go`

#### CLI 参数

| 参数 | 说明 | 示例 |
|---|---|---|
| `--dsn` | Postgres DSN | `postgres://user:pass@host:5432/db?sslmode=disable` |
| `--config` | `config.yaml` 路径（可替代 `--dsn`） | `/path/to/config.yaml` |
| `--days` | 回填最近 N 天（简写，与 `--start` 二选一） | `90` |
| `--start` | 回填起始时间（RFC3339） | `2026-01-01T00:00:00Z` |
| `--end` | 回填结束时间（RFC3339，默认当前时间） | `2026-04-07T00:00:00Z` |

> **不填日期参数时**，脚本会自动查询 `SELECT MIN(created_at) FROM usage_logs` 确定起始时间，全量回填到当前时间。

#### 用法示例

**全量回填（最简单，不填日期）：**

```bash
cd /Volumes/T7/project/new/FluxCode/backend

go run ./cmd/dashboard-backfill \
  --dsn 'postgres://fluxcode:123456@127.0.0.1:5432/fluxcode?sslmode=disable'
```

**回填最近 90 天：**

```bash
cd /Volumes/T7/project/new/FluxCode/backend

go run ./cmd/dashboard-backfill \
  --config /Volumes/T7/project/new/FluxCode/backend/config.yaml \
  --days 90
```

**指定时间范围：**

```bash
cd /Volumes/T7/project/new/FluxCode/backend

go run ./cmd/dashboard-backfill \
  --dsn 'postgres://fluxcode:123456@127.0.0.1:5432/fluxcode?sslmode=disable' \
  --start '2026-01-01T00:00:00Z' \
  --end '2026-04-07T00:00:00Z'
```

脚本会逐天聚合并打印进度，无超时限制，完成后自动更新水位标记。

---

## 七、Step 5：验收检查

### 7.1 数据口径验收

- [ ] `users` 行数一致
- [ ] `groups` 行数一致
- [ ] `api_keys` 行数一致
- [ ] `user_subscriptions` + `subscription_grants` 行数一致
- [ ] `redeem_codes` 行数一致
- [ ] `pricing_plan_groups` + `pricing_plans` 行数一致
- [ ] `usage_logs` 行数一致（允许微小误差）
- [ ] `proxy_usage_metrics_hourly` 行数一致
- [ ] `ops_metrics_hourly` / `ops_metrics_daily` 行数一致
- [ ] `billing_usage_entries` 行数一致
- [ ] 抽样 3-5 个用户，核对订阅余额与到期时间

### 7.2 功能验收

- [ ] **用户登录**：随机抽样旧用户可登录新系统
- [ ] **叠卡兑换**：用户兑换页出现 extend/stack 二选一
- [ ] **管理端订阅分配**：出现 extend/stack 二选一
- [ ] **我的订阅页**：正确展示 grant/额度信息
- [ ] **定价页**：可见、可编辑、可保存
- [ ] **号池监控**：配置页可读取/保存，监控面板能显示汇总
- [ ] **Dashboard**：管理员页能加载统计与趋势图（代理用量为空时不应整页空白）
- [ ] **多语言**：中英文切换无缺 key、无 fallback 漏网

### 7.3 行为验收

- [ ] 网关产生新消耗后，grant 扣减正确
- [ ] Dashboard / 号池监控 / 定价页面结果与旧系统预期一致

---

## 八、Step 6：切换流量

验收全部通过后：

- [ ] 将流量从旧系统切到新系统
- [ ] 观察新系统日志，确认无异常
- [ ] 保留旧系统备份与迁移报告至少 **7 天**

---

## 九、回滚预案

**如果验收失败：**

1. 立即切回旧系统流量
2. 停止新系统写入
3. 记录失败原因与差异报告
4. 修复后重新从 **Step 1（Dry-Run）** 开始

**关键资产保留：**

- 旧系统 `pg_dump` 备份
- 每次 apply 的 `--report_file` JSON 报告
- 冻结时间点记录

---

## 附录：常见问题

### Q: `--reset_target` 是什么意思？

会在 apply 前对目标表执行 `TRUNCATE ... RESTART IDENTITY CASCADE`，即清空目标表数据。**首次迁移推荐开启**，重复迁移时如果目标库有新增数据要保留则不要开启。

### Q: 不加 `--reset_target` 会怎样？

迁移器会使用 `INSERT ... ON CONFLICT DO NOTHING`，跳过已存在的行。适合增量补数据，但不会更新已有行。

### Q: `usage_logs` 太大怎么办？

迁移器已内置批量插入优化（`jsonb_populate_recordset`）。如果仍然很慢，可以单独用 `--phase usage` 在低峰期执行，不影响其他阶段。

### Q: 两套系统指向同一个数据库可以 dry-run 吗？

可以。dry-run 只读不写，此时 `diff` 会全是 0，但可以验证连接和 phase 是否正常。

### Q: 哪些表是迁移后重建而不是直接搬的？

`usage_dashboard_*`、`usage_billing_dedup*` 等聚合/衍生表，见 Step 4。

### Q: DSN 密码含特殊字符（如 `%`）怎么办？

DSN 是标准 URL，密码中的特殊字符必须做 **URL percent-encoding**。用以下命令获取编码后的密码：

```bash
python3 -c "import urllib.parse; print(urllib.parse.quote('你的原始密码', safe=''))"
```

常见字符对照表：

| 原始字符 | 编码后 |
|---|---|
| `%` | `%25` |
| `@` | `%40` |
| `#` | `%23` |
| `!` | `%21` |
| `$` | `%24` |
| `&` | `%26` |
| `+` | `%2B` |
| `/` | `%2F` |
| `=` | `%3D` |
| `?` | `%3F` |
| 空格 | `%20` |

例如密码 `my%pass@123` 编码后为 `my%25pass%40123`，DSN 写为：

```
postgres://myuser:my%25pass%40123@host:5432/mydb?sslmode=disable
```

> **提示**：DSN 整体用**单引号**包裹，避免 shell 对 `%`、`$` 等符号二次解释。
