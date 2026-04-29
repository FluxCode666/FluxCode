# PostgreSQL 备份指南

适用范围：
- 使用 `deploy/infra/docker-compose.infra.yml` 运行 PostgreSQL 的 FluxCode 多机部署
- 使用仓库内现成脚本进行手动备份、定时备份和自动清理

如果你的实际部署目录不是 `/opt/FluxCode`，请把下面命令中的路径替换为你的实际路径。

相关文件：
- `deploy/infra/backup-postgres-db.sh`：单次备份脚本
- `deploy/infra/backup-postgres-job.sh`：定时任务入口脚本
- `deploy/infra/backup-postgres-job.env.example`：定时任务配置模板
- `deploy/infra/systemd/fluxcode-pg-backup.service`：systemd service 单元
- `deploy/infra/systemd/fluxcode-pg-backup.timer`：systemd timer 单元
- `deploy/infra/cron/fluxcode-pg-backup.cron.example`：cron 配置示例


🔇注意：job和db备份脚本在`/root/backup`目录下

---

## 1. 手动备份

默认会读取同目录下的 `docker-compose.infra.yml` 和 `.env`，并从 `postgres` 服务中导出指定库。

示例：
```bash
cd /opt/FluxCode/deploy/infra
./backup-postgres-db.sh --db fluxcode
```

常用参数：
- `--db`：数据库名
- `--format custom|plain`：导出格式，默认 `custom`
- `--output-dir`：输出目录
- `--retention-days`：清理超过 N 天的历史备份

示例：
```bash
# 导出为 .dump，并保留最近 7 天
./backup-postgres-db.sh --db fluxcode --retention-days 7

# 导出为 .sql.gz
./backup-postgres-db.sh --db fluxcode --format plain
```

说明：
- `custom` 格式生成 `.dump` 文件，适合后续使用 `pg_restore` 恢复，推荐默认使用。
- `plain` 格式会生成 `.sql.gz` 文件，CPU 压力略高。
- 备份完成后，脚本会自动打印恢复命令示例。

---

## 2. Ubuntu 上使用 systemd timer 定时备份

以下流程适用于在 **Ubuntu** 上把 `deploy/infra/backup-postgres-job.sh` 挂到 `systemd`，实现 **开机自启 + 每天定时备份**。

### 2.1 准备备份配置

先复制任务配置模板，并按需修改：
```bash
cd /opt/FluxCode/deploy/infra
sudo mkdir -p /etc/fluxcode
sudo cp backup-postgres-job.env.example /etc/fluxcode/backup-postgres-job.env
sudo nano /etc/fluxcode/backup-postgres-job.env
```

至少确认以下配置：
- `DB_NAME`：要备份的数据库名，例如 `fluxcode`
- `COMPOSE_FILE`：通常为 `/opt/FluxCode/deploy/infra/docker-compose.infra.yml`
- `ENV_FILE`：通常为 `/opt/FluxCode/deploy/infra/.env`
- `OUTPUT_DIR`：备份输出目录，例如 `/opt/FluxCode/deploy/infra/backups/postgres`
- `RETENTION_DAYS`：保留最近 N 天备份，超期文件会在每次执行后自动清理

### 2.2 安装 service 和 timer

把仓库中的 `systemd` 单元文件复制到系统目录：
```bash
cd /opt/FluxCode/deploy/infra
sudo cp systemd/fluxcode-pg-backup.service /etc/systemd/system/
sudo cp systemd/fluxcode-pg-backup.timer /etc/systemd/system/
```

### 2.3 设置时区为北京时间

`systemd timer` 默认按 **系统本地时区** 触发；如果你希望按北京时间每天凌晨 4 点执行，请确认服务器时区为 `Asia/Shanghai`：
```bash
sudo timedatectl set-timezone Asia/Shanghai
timedatectl
```

### 2.4 启用开机自启并立即生效

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now fluxcode-pg-backup.timer
```

说明：
- 开机自启的是 `fluxcode-pg-backup.timer`，不是 `fluxcode-pg-backup.service`
- `timer` 到点后会自动拉起一次 `service` 执行备份

### 2.5 手动执行一次验证

建议首次配置后手动跑一次，确认路径、权限、数据库连接都正常：
```bash
sudo systemctl start fluxcode-pg-backup.service
sudo journalctl -u fluxcode-pg-backup.service -n 200 --no-pager
```

### 2.6 查看定时器状态

```bash
sudo systemctl status fluxcode-pg-backup.timer --no-pager
sudo systemctl list-timers --all | grep fluxcode-pg-backup
```

### 2.7 当前默认调度说明

仓库中的默认配置是：
- 每天 `04:00` 执行一次
- `Persistent=true`：如果机器在计划时间关机，开机后会补跑一次
- `RandomizedDelaySec=300`：实际会在 `04:00` 到 `04:05` 之间随机启动，以减少固定时刻的资源尖峰

如果你需要 **严格在 04:00:00 执行**，可以把 `systemd/fluxcode-pg-backup.timer` 中的 `RandomizedDelaySec=300` 改成 `0` 或直接删除这一行。

---

## 3. 使用 cron 定时备份（可选）

如果你更习惯 `cron`，也可以使用仓库内提供的模板：
```bash
cd /opt/FluxCode/deploy/infra
sudo cp cron/fluxcode-pg-backup.cron.example /etc/cron.d/fluxcode-pg-backup
sudo chmod 644 /etc/cron.d/fluxcode-pg-backup
sudo systemctl enable --now cron
```

当前模板默认在每天凌晨 `04:00` 执行：
```cron
0 4 * * * root /opt/FluxCode/deploy/infra/backup-postgres-job.sh /etc/fluxcode/backup-postgres-job.env >> /var/log/fluxcode-pg-backup.log 2>&1
```

注意：
- `cron` 和 `systemd timer` 二选一，不要同时启用，否则会重复备份。
- 保留天数仍然由 `/etc/fluxcode/backup-postgres-job.env` 中的 `RETENTION_DAYS` 控制。

---

## 4. 恢复示例

如果备份格式为 `custom`（`.dump`）：
```bash
docker compose -f /opt/FluxCode/deploy/infra/docker-compose.infra.yml exec -T postgres \
  pg_restore -U <POSTGRES_USER> -d <target_db> < /path/to/backup.dump
```

如果备份格式为 `plain`（`.sql.gz`）：
```bash
gunzip -c /path/to/backup.sql.gz | docker compose -f /opt/FluxCode/deploy/infra/docker-compose.infra.yml exec -T postgres \
  psql -U <POSTGRES_USER> -d <target_db>
```
