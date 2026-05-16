# Datadog 监控 PostgreSQL / Redis（最小权限版）

## 1. 最小权限账号创建

### PostgreSQL（建议 PostgreSQL 10+）

```sql
-- 使用超级用户连接后执行
CREATE ROLE dd_monitor WITH LOGIN PASSWORD '请替换为强密码';

-- Datadog 读取数据库运行状态所需最小内置监控权限
GRANT pg_monitor TO dd_monitor;

-- 仅允许连接业务库（按需修改库名）
GRANT CONNECT ON DATABASE fluxcode TO dd_monitor;
```

### Redis（建议 Redis 6+，使用 ACL）

```bash
# 在 redis-cli 中执行（按需修改用户名/密码）
ACL SETUSER dd_monitor on >请替换为你的强密码 ~* -@all +ping +info +config|get +client|list +slowlog|get +latency|latest

```

---

## 2. Compose 文件

文件路径：`deploy/datadog/docker-compose.dd_infra.yml`

```yaml
services:
  dd-agent:
    image: gcr.io/datadoghq/agent:7
    container_name: dd-agent
    restart: unless-stopped
    environment:
      DD_API_KEY: "${DD_API_KEY}"
      DD_SITE: "ap1.datadoghq.com"
      DD_DOGSTATSD_NON_LOCAL_TRAFFIC: "true"
      DD_ENV: "prod"
      DD_LOGS_ENABLED: "true"
      DD_LOGS_CONFIG_AUTO_MULTI_LINE_DETECTION: "true"
      DD_LOGS_CONFIG_CONTAINER_COLLECT_ALL: "true"
      DD_CONTAINER_EXCLUDE_LOGS: "name:dd-agent"
      # 避免 redisdb/postgres 同时跑 conf.yaml 和 auto_conf.yaml（重复检查）
      # 注意：这里的集成名是 postgres（不是 pgsql）
      DD_IGNORE_AUTOCONF: "redisdb,postgres"
      DD_HOSTNAME: "hk-2"

      # PostgreSQL 连接参数（由 .env 注入）
      PG_MONITOR_HOST: "${PG_MONITOR_HOST}"
      PG_MONITOR_PORT: "${PG_MONITOR_PORT:-5432}"
      PG_MONITOR_USER: "${PG_MONITOR_USER}"
      PG_MONITOR_PASSWORD: "${PG_MONITOR_PASSWORD}"
      PG_MONITOR_DB: "${PG_MONITOR_DB:-fluxcode}"

      # Redis 连接参数（由 .env 注入）
      REDIS_MONITOR_HOST: "${REDIS_MONITOR_HOST}"
      REDIS_MONITOR_PORT: "${REDIS_MONITOR_PORT:-6379}"
      REDIS_MONITOR_PASSWORD: "${REDIS_MONITOR_PASSWORD}"
      REDIS_MONITOR_USERNAME: "${REDIS_MONITOR_USERNAME:-}"

    volumes:
      - /opt/datadog-agent/run:/opt/datadog-agent/run:rw
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /proc/:/host/proc/:ro
      - /sys/fs/cgroup/:/host/sys/fs/cgroup:ro
      - /var/lib/docker/containers:/var/lib/docker/containers:ro

      # 挂载 integration 配置
      - /Volumes/T7/project/FluxCode/deploy/datadog/conf.d/postgres.d/conf.yaml:/conf.d/postgres.d/conf.yaml:ro
      - /Volumes/T7/project/FluxCode/deploy/datadog/conf.d/redisdb.d/conf.yaml:/conf.d/redisdb.d/conf.yaml:ro
```

---

## 3. conf.yaml 文件

### PostgreSQL

文件路径：`deploy/datadog/conf.d/postgres.d/conf.yaml`

```yaml
init_config:

instances:
  - host: "%%env_PG_MONITOR_HOST%%"
    port: "%%env_PG_MONITOR_PORT%%"
    username: "%%env_PG_MONITOR_USER%%"
    password: "%%env_PG_MONITOR_PASSWORD%%"
    dbname: "%%env_PG_MONITOR_DB%%"
    ssl: "disable"
```

### Redis

文件路径：`deploy/datadog/conf.d/redisdb.d/conf.yaml`

```yaml
init_config:

instances:
  - host: "%%env_REDIS_MONITOR_HOST%%"
    port: "%%env_REDIS_MONITOR_PORT%%"
    username: "%%env_REDIS_MONITOR_USERNAME%%"
    password: "%%env_REDIS_MONITOR_PASSWORD%%"
```

---

## 4. 注意事项（实战排障结论）

- `redisdb` 或 `postgres` 出现一个 `OK`、一个 `ERROR`（`auto_conf.yaml`）时，说明同一检查被加载了两份，需保留 `conf.yaml` 并设置 `DD_IGNORE_AUTOCONF=redisdb,postgres`。
- `DD_IGNORE_AUTOCONF` 的键名是集成名：`redisdb`、`postgres`（不是 `pgsql`）。
- `Timeout connecting to server` 是网络连通问题（TCP 未建立），不是密码错误；密码错误通常是 `NOAUTH` 或 `WRONGPASS`。
- 修改 `.env` 后不能只 `restart`，必须重建容器（`up -d --force-recreate`）才会加载新变量。
- 在 `redis-cli` 交互模式里执行 ACL 命令不要用反斜杠续行；直接一行输入。
- `agent check redisdb -v` 在当前 Agent 版本不支持，使用 `agent check redisdb`。

---

## 5. 验证方法

### 5.1 重建并加载最新 `.env`

```bash
docker compose --env-file .env -f docker-compose.dd_infra.yml up -d --force-recreate dd-agent
```

### 5.2 确认容器内环境变量

```bash
docker exec -it dd-agent printenv REDIS_MONITOR_HOST REDIS_MONITOR_PORT PG_MONITOR_HOST PG_MONITOR_PORT
```

### 5.3 检查配置是否只加载一份 `redisdb`

```bash
docker exec -it dd-agent agent configcheck | sed -n '/redisdb/,/===/p'
```

期望：仅看到 `file:/etc/datadog-agent/conf.d/redisdb.d/conf.yaml`，不再出现 `auto_conf.yaml`。

### 5.4 检查配置是否只加载一份 `postgres`

```bash
docker exec -it dd-agent agent configcheck | sed -n '/postgres/,/===/p'
```

期望：仅看到 `file:/etc/datadog-agent/conf.d/postgres.d/conf.yaml`，不再出现 `auto_conf.yaml`。

### 5.5 检查运行状态

```bash
docker exec -it dd-agent agent check redisdb
docker exec -it dd-agent agent check postgres
docker exec -it dd-agent agent status | sed -n '/redisdb/,+40p'
docker exec -it dd-agent agent status | sed -n '/postgres/,+40p'
```

### 5.6 快速网络连通性检查（容器内）

```bash
python -c "import os,socket;h=os.getenv('REDIS_MONITOR_HOST');p=int(os.getenv('REDIS_MONITOR_PORT','6379'));socket.create_connection((h,p),3);print('redis ok')"
python -c "import os,socket;h=os.getenv('PG_MONITOR_HOST');p=int(os.getenv('PG_MONITOR_PORT','5432'));socket.create_connection((h,p),3);print('postgres ok')"
```
