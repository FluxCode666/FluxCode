# PostgreSQL + Redis 迁移指南（多机部署 / Docker Compose）

适用范围：
- 旧/新“状态机”使用 `deploy/infra/docker-compose.infra.yml` 运行 PostgreSQL + Redis
- “应用节点”使用 `deploy/node/docker-compose.node.yml` 连接远程 PostgreSQL/Redis
- 目标：在 **停机窗口约 1 小时**内，把 PostgreSQL 数据 + Redis `redis_data` 迁移到另一台服务器

> 本文默认旧/新两边都使用同版本镜像（当前 compose：Postgres `postgres:18-alpine`、Redis `redis:7-alpine`）。

---

## 0. 你需要先准备的信息

请先明确三类机器：
- **旧状态机（OLD_INFRA）**：当前运行 PostgreSQL + Redis 的服务器
- **新状态机（NEW_INFRA）**：要迁移到的服务器
- **应用节点（APP_NODES）**：运行 FluxCode 的一台或多台服务器（都会连接数据库/缓存）

并准备以下占位符：
- `<FLUXCODE_DIR>`：每台服务器上 FluxCode 仓库路径（例如 `/opt/FluxCode`）
- `<NEW_INFRA_IP>`：新状态机内网 IP（应用节点能访问到的地址）
- `<SSH_USER>`：你用来 SSH 登录新状态机的用户名

可选：为了方便复制粘贴，你也可以在每台服务器先设置环境变量：
```bash
export FLUXCODE_DIR=<FLUXCODE_DIR>
export NEW_INFRA_IP=<NEW_INFRA_IP>
export SSH_USER=<SSH_USER>
```

---

## 1. 新状态机预准备（不停机，在 NEW_INFRA 执行）

进入目录，准备环境变量文件：
```bash
cd "$FLUXCODE_DIR/deploy/infra"
cp .env.infra.example .env
nano .env
```

至少确认/设置（建议与旧状态机保持一致，减少变量差异）：
- `POSTGRES_USER`
- `POSTGRES_PASSWORD`
- `POSTGRES_DB`
- `REDIS_PASSWORD`
- （建议）`POSTGRES_BIND_HOST` / `REDIS_BIND_HOST` 绑定新状态机内网 IP，或保持 `0.0.0.0` 但用防火墙严格限制来源

预拉镜像（加快停机窗口操作）：
```bash
cd "$FLUXCODE_DIR/deploy/infra"
docker compose -f docker-compose.infra.yml pull
mkdir -p migrate
```

---

## 2. 停机窗口开始：停止所有应用节点写入（在每台 APP_NODE 执行）

**必须停掉所有应用节点**，避免迁移过程中仍有写入：
```bash
cd "$FLUXCODE_DIR/deploy/node"
docker compose -f docker-compose.node.yml down
```

（可选）确认已停：
```bash
cd "$FLUXCODE_DIR/deploy/node"
docker compose -f docker-compose.node.yml ps
```

---

## 3. 导出 PostgreSQL（在 OLD_INFRA 执行）

使用 `pg_dump` 导出为自定义格式（`-Fc`），为“尽量快”默认不压缩（`-Z 0`）：
```bash
cd "$FLUXCODE_DIR/deploy/infra"
mkdir -p migrate

docker compose -f docker-compose.infra.yml exec -T postgres sh -c \
'export PGPASSWORD="$POSTGRES_PASSWORD"; pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc -Z 0 --no-owner --no-acl' \
> migrate/fluxcode.dump

ls -lh migrate/fluxcode.dump
```

> 如果你网络很慢，也可以把 `-Z 0` 改成 `-Z 9` 降低传输体积（导出会稍慢）。

---

## 4. 导出 Redis（在 OLD_INFRA 执行）

本项目 Redis 开启了 AOF（`--appendonly yes`），迁移时我们用 RDB 快照 `dump.rdb`：
```bash
cd "$FLUXCODE_DIR/deploy/infra"

# 生成一份最新 RDB
docker compose -f docker-compose.infra.yml exec -T redis sh -c 'redis-cli SAVE'

# 拷出 dump.rdb 到宿主机
REDIS_CID=$(docker compose -f docker-compose.infra.yml ps -q redis)
docker cp "$REDIS_CID":/data/dump.rdb migrate/dump.rdb

ls -lh migrate/dump.rdb
```

---

## 5. 传输备份文件到新状态机（在 OLD_INFRA 执行）

先在新状态机创建目录：
```bash
ssh "$SSH_USER@$NEW_INFRA_IP" "mkdir -p '$FLUXCODE_DIR/deploy/infra/migrate'"
```

推荐用 `rsync`（有进度显示）：
```bash
cd "$FLUXCODE_DIR/deploy/infra"
rsync -avP migrate/fluxcode.dump migrate/dump.rdb "$SSH_USER@$NEW_INFRA_IP:$FLUXCODE_DIR/deploy/infra/migrate/"
```

---

## 6. 恢复 PostgreSQL（在 NEW_INFRA 执行）

如果新状态机上这套 compose **以前启动过**，并且你确认没有要保留的数据，建议先清空卷：
```bash
cd "$FLUXCODE_DIR/deploy/infra"
docker compose -f docker-compose.infra.yml down -v
```

> 警告：`down -v` 会删除 `postgres_data/redis_data` 卷（不可恢复）。只在“新状态机没有重要数据”时使用。

启动 PostgreSQL 并导入：
```bash
cd "$FLUXCODE_DIR/deploy/infra"
docker compose -f docker-compose.infra.yml up -d postgres

PG_CID=$(docker compose -f docker-compose.infra.yml ps -q postgres)
docker cp migrate/fluxcode.dump "$PG_CID":/tmp/fluxcode.dump

docker compose -f docker-compose.infra.yml exec -T postgres sh -c \
'export PGPASSWORD="$POSTGRES_PASSWORD"; pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --no-owner --no-acl /tmp/fluxcode.dump'
```

---

## 7. 恢复 Redis（在 NEW_INFRA 执行）

关键点：本项目的 Redis 默认启用了 AOF（`--appendonly yes`）。在 Redis 7.x 的 **multi-part AOF** 模式下，Redis 会优先从 `/data/appendonlydir` 加载；如果它是空的，会表现为 **`dump.rdb` 已放入但 `dbsize=0`**。

因此恢复要分两步：
1) 先用 **appendonly=no** 启动一次，让 Redis 从 `dump.rdb` 把数据加载进内存  
2) 再在运行中开启 AOF 并重写，让后续常规启动（appendonly=yes）也能加载到数据

按以下顺序执行：
```bash
cd "$FLUXCODE_DIR/deploy/infra"

# 如果 redis 之前已经启动过，先停掉并移除容器（不要加 -v，避免删卷）
docker compose -f docker-compose.infra.yml stop redis || true
docker compose -f docker-compose.infra.yml rm -f redis || true

# 只创建容器，不启动（为 docker cp 提供目标容器）
docker compose -f docker-compose.infra.yml create redis

# 将 dump.rdb 放进容器的 /data（对应 redis_data 卷）
# 注意：create 之后容器是 “stopped/created” 状态，`ps` 默认只显示运行中的容器，需要加 `-a`
REDIS_CID=$(docker compose -f docker-compose.infra.yml ps -a -q redis)
docker cp migrate/dump.rdb "$REDIS_CID":/data/dump.rdb

# 清掉可能存在的空 AOF（避免覆盖）
docker compose -f docker-compose.infra.yml run --rm --no-deps --entrypoint sh redis -c \
'rm -rf /data/appendonlydir; rm -f /data/appendonly.aof*; ls -lah /data'

# 以“恢复模式”启动一次：appendonly=no（见同目录的 override 文件）
docker compose -f docker-compose.infra.yml -f docker-compose.infra.restore-redis.yml up -d redis --force-recreate

# 确认已经从 dump.rdb 加载到数据（至少应看到非 0 或 keyspace 有内容）
docker compose -f docker-compose.infra.yml exec -T redis sh -c 'redis-cli dbsize; redis-cli INFO keyspace'

# 在运行中开启 AOF 并重写（生成 appendonlydir/*，供后续常规启动加载）
docker compose -f docker-compose.infra.yml exec -T redis sh -c 'redis-cli CONFIG SET appendonly yes'
docker compose -f docker-compose.infra.yml exec -T redis sh -c 'redis-cli BGREWRITEAOF'

# 切回常规启动（appendonly=yes）
docker compose -f docker-compose.infra.yml up -d redis --force-recreate
```

> 额外校验：如果你怀疑 `dump.rdb` 本身是空的，可在容器里检查（会显示 key 数量）：
> ```bash
> docker compose -f docker-compose.infra.yml exec -T redis sh -c 'redis-check-rdb /data/dump.rdb | tail -n 5'
> ```

---

## 8. 新状态机自检（在 NEW_INFRA 执行）

查看服务状态：
```bash
cd "$FLUXCODE_DIR/deploy/infra"
docker compose -f docker-compose.infra.yml ps
```

检查 PostgreSQL 可用性 + 关键表数量（以 `usage_logs` 为例）：
```bash
cd "$FLUXCODE_DIR/deploy/infra"
docker compose -f docker-compose.infra.yml exec -T postgres sh -c \
'pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB"'

docker compose -f docker-compose.infra.yml exec -T postgres sh -c \
'export PGPASSWORD="$POSTGRES_PASSWORD"; psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "select count(*) from usage_logs;"'
```

检查 Redis：
```bash
cd "$FLUXCODE_DIR/deploy/infra"
docker compose -f docker-compose.infra.yml exec -T redis sh -c 'redis-cli ping'
docker compose -f docker-compose.infra.yml exec -T redis sh -c 'redis-cli dbsize'
docker compose -f docker-compose.infra.yml exec -T redis sh -c 'redis-cli INFO keyspace'
```

---

## 9. 切换应用节点到新状态机并启动（在每台 APP_NODE 执行）

编辑应用节点 `.env`，把数据库/缓存地址指向新状态机：
```bash
cd "$FLUXCODE_DIR/deploy/node"
nano .env
```

至少修改：
- `DATABASE_HOST=<NEW_INFRA_IP>`
- `REDIS_HOST=<NEW_INFRA_IP>`

启动：
```bash
cd "$FLUXCODE_DIR/deploy/node"
docker compose -f docker-compose.node.yml up -d
docker compose -f docker-compose.node.yml ps
docker compose -f docker-compose.node.yml logs -f fluxcode
```

---

## 10. 回滚预案（可选）

如果新状态机验证失败或应用启动异常：
1) 在所有应用节点把 `deploy/node/.env` 里的 `DATABASE_HOST/REDIS_HOST` 改回旧状态机 IP
2) 重新启动：
```bash
cd "$FLUXCODE_DIR/deploy/node"
docker compose -f docker-compose.node.yml up -d
```

---

## 11. 收尾建议（可选）

- 观察一段时间后再下线旧状态机（保留作为回滚保险）
- 备份文件（`migrate/fluxcode.dump`、`migrate/dump.rdb`）建议转存到安全位置后再删除
