# Sub2API 集群部署指南

本文档介绍如何以前后端分离的方式部署 Sub2API，支持两种模式：
- **单机多容器** — 一台机器上运行所有服务（体验/测试用）
- **多机部署** — 前端、后端、数据库分布在不同机器上（生产推荐）

前端/LB 支持两种 Web 服务器：
- **Nginx** — 成熟稳定，需手动配置 HTTPS 证书
- **Caddy** — 自动 HTTPS（Let's Encrypt），内置 HTTP/2 + HTTP/3

---

## 架构概览

```
                        ┌─────────────────────────┐
  机器 A (前端/LB)       │   Nginx (前端 + LB)       │
                        │   端口 80/443             │
                        │                          │
                        │  ┌─ 静态文件 → 本地 dist   │
                        │  ├─ /api/*   ─┐           │
                        │  ├─ /v1/*    ─┤           │
                        │  ├─ /sora/*  ─┤→ upstream │
                        │  ├─ /health  ─┤   后端集群  │
                        │  └─ SPA路由  → index.html  │
                        └────────┬────────┬─────────┘
                                 │        │
                    ┌────────────┘        └────────────┐
                    ▼                                   ▼
  机器 B             ┌──────────────┐  机器 C   ┌──────────────┐
                    │  Backend #1   │          │  Backend #2   │
                    │  :8080 (API)  │          │  :8080 (API)  │
                    │  无前端嵌入    │          │  无前端嵌入    │
                    └──────┬───────┘          └──────┬───────┘
                           │                          │
                           └──────────┬───────────────┘
                                      │
                         ┌────────────┴────────────┐
                         ▼                         ▼
  机器 D (或云服务)  ┌────────────┐          ┌─────────────┐
                    │ PostgreSQL  │          │    Redis     │
                    │   15+       │          │    7+        │
                    └────────────┘          └─────────────┘
```

---

## 前提条件

- Docker 24+ 和 Docker Compose v2
- PostgreSQL 15+（内置或外部/云服务）
- Redis 7+（内置或外部/云服务）
- **所有后端实例必须共享相同的** `JWT_SECRET` 和 `TOTP_ENCRYPTION_KEY`

---

## 部署方式一：单机多容器

适用于在一台机器上体验集群模式（前后端分离，但所有容器在同一台机器）：

```bash
cd deploy

# 1. 复制环境变量模板
cp .env.cluster.example .env

# 2. 编辑配置（必须设置 DATABASE_PASSWORD 和 JWT_SECRET）
#    生成密钥: openssl rand -hex 32
nano .env

# 3. 构建并启动
docker compose -f docker-compose.cluster.yml up -d --build

# 4. 访问
#    前端: http://localhost (或 $FRONTEND_PORT)
#    健康检查: curl http://localhost/health
```

相关文件：
- `docker-compose.cluster.yml` — 编排所有服务
- `.env.cluster.example` — 环境变量模板

---

## 部署方式二：多机部署（生产推荐）

每台机器独立运行自己的容器，通过实际 IP 地址通信。

### 步骤 1：部署数据库（机器 D）

**方式一：使用 docker-compose（推荐自建）**

```bash
# 在数据库机器上
mkdir -p /opt/sub2api && cd /opt/sub2api

# 复制部署目录
# (从项目 deploy/db/ 复制 docker-compose.yml 和 .env.example)
cp .env.example .env
nano .env  # 设置 POSTGRES_PASSWORD

# 创建数据目录
mkdir -p postgres_data redis_data

# 启动
docker compose up -d
```

**方式二：使用云服务**

直接使用 AWS RDS / 阿里云 RDS / 腾讯云数据库 + ElastiCache / 阿里云 Redis，跳过此步骤。

> ⚠️ **安全提醒**：使用防火墙规则限制数据库端口（5432、6379），只允许后端机器 IP 访问。

相关文件：
- `deploy/db/docker-compose.yml` — 数据库编排
- `deploy/db/.env.example` — 环境变量模板

### 步骤 2：部署后端（机器 B、C、...）

> **首次部署时，先只启动 1 个后端实例**，等数据库初始化完成后再启动其余实例。

在每台后端机器上：

```bash
mkdir -p /opt/sub2api && cd /opt/sub2api

# 复制部署目录
# (从项目 deploy/backend/ 复制 docker-compose.yml 和 .env.example)
cp .env.example .env
nano .env  # 设置数据库地址、Redis 地址、JWT_SECRET 等

# 创建数据目录
mkdir -p data

# 启动
docker compose up -d

# 验证
curl http://localhost:8080/health
# 应返回: {"status":"ok"}
```

`.env` 中的关键配置：

```bash
# 数据库（填入机器 D 的 IP）
DATABASE_HOST=10.0.1.100
DATABASE_PORT=5432
DATABASE_PASSWORD=your_secure_password

# Redis（填入机器 D 的 IP）
REDIS_HOST=10.0.1.100
REDIS_PORT=6379

# ⚠️ 所有后端实例必须完全一致！
JWT_SECRET=your_fixed_jwt_secret
TOTP_ENCRYPTION_KEY=your_fixed_totp_key
```

相关文件：
- `deploy/backend/docker-compose.yml` — 后端编排（暴露 8080 端口）
- `deploy/backend/.env.example` — 环境变量模板

### 步骤 3：部署前端/LB（机器 A）

前端/LB 支持 Nginx 和 Caddy 两种方案，选择其一即可。

#### 方案 A：Nginx（需手动配置 HTTPS）

```bash
mkdir -p /opt/sub2api && cd /opt/sub2api

# 复制部署目录
# (从项目 deploy/frontend/ 复制 docker-compose.yml、.env.example)
cp .env.example .env
nano .env  # 设置 BACKEND_SERVERS
```

`.env` 关键配置：

```bash
# 逗号分隔的后端实例地址
BACKEND_SERVERS=10.0.1.10:8080,10.0.1.11:8080
```

启动：

```bash
docker compose up -d

# 验证前端
curl http://localhost/
# 应返回 HTML 页面

# 验证 API 代理
curl http://localhost/health
# 应返回: {"status":"ok"}
```

前端容器启动时，`entrypoint.sh` 会自动从 `BACKEND_SERVERS` 环境变量生成 Nginx upstream 配置。

相关文件：
- `deploy/frontend/docker-compose.yml` — Nginx 前端编排
- `deploy/frontend/.env.example` — 环境变量模板
- `deploy/frontend/entrypoint.sh` — 启动脚本（envsubst 模板替换）
- `deploy/frontend/nginx.conf.template` — Nginx 配置模板

#### 方案 B：Caddy（自动 HTTPS，推荐）

Caddy 会自动申请和续期 Let's Encrypt / ZeroSSL 证书，无需手动管理 TLS。

```bash
mkdir -p /opt/sub2api && cd /opt/sub2api

# 复制部署目录
# (从项目 deploy/frontend/ 复制 docker-compose.caddy.yml、.env.example)
cp .env.example .env
nano .env  # 设置 BACKEND_SERVERS 和 SITE_DOMAIN
```

`.env` 关键配置：

```bash
# 逗号分隔的后端实例地址
BACKEND_SERVERS=10.0.1.10:8080,10.0.1.11:8080

# 域名（自动 HTTPS）或 ":80"（仅 HTTP）
# 使用域名时，该域名必须解析到本机公网 IP
SITE_DOMAIN=api.sub2api.com
```

启动：

```bash
docker compose -f docker-compose.caddy.yml up -d

# 验证前端
curl http://localhost/
# 应返回 HTML 页面（HTTPS 模式下会 301 到 https://）

# 验证 API 代理
curl http://localhost/health
# 应返回: {"status":"ok"}
```

相关文件：
- `deploy/frontend/docker-compose.caddy.yml` — Caddy 前端编排
- `deploy/frontend/.env.example` — 环境变量模板
- `deploy/frontend/caddy-entrypoint.sh` — 启动脚本（envsubst 模板替换）
- `deploy/frontend/Caddyfile.template` — Caddy 配置模板

### 扩缩后端节点

**添加新后端**：

```bash
# 1. 在新机器上启动后端（使用相同的 .env 配置）
# 2. 在前端机器上更新 BACKEND_SERVERS
nano .env
# BACKEND_SERVERS=10.0.1.10:8080,10.0.1.11:8080,10.0.1.12:8080

# 3. 重启前端容器使新 upstream 生效
docker compose up -d
```

**移除后端**：从 `BACKEND_SERVERS` 中去掉对应地址，重启前端容器。

---

## Caddy 配置说明

### Nginx vs Caddy 对比

| 特性 | Nginx | Caddy |
|------|-------|-------|
| HTTPS 证书 | 手动配置 / certbot | 自动申请和续期 |
| HTTP/2 | 需配置 | 内置 |
| HTTP/3 (QUIC) | 不支持 | 内置 |
| WebSocket | 需配置 Upgrade 头 | 自动处理 |
| 下划线 Header | 需 `underscores_in_headers on` | 默认支持 |
| 配置复杂度 | 较复杂 | 简洁 |

### 自定义 Caddy 配置

挂载自定义 Caddyfile 完全覆盖模板：

```yaml
# docker-compose.caddy.yml 中
volumes:
  - ./custom-Caddyfile:/etc/caddy/Caddyfile:ro
```

此时 `BACKEND_SERVERS` 和 `SITE_DOMAIN` 环境变量不再生效，需要在自定义 Caddyfile 中手动配置。

### Caddy 多机/多服务同域名部署

如果需要多台前端机器水平扩展（多个 Caddy 实例服务同一个域名），有以下几种方案：

#### 方案 1：外部 LB + Caddy HTTP 模式（推荐）

```
用户 → [Cloudflare / 云SLB / HAProxy] → Caddy 实例 A (HTTP) → 后端集群
                                        → Caddy 实例 B (HTTP) → 后端集群
```

外部 LB 负责 TLS 终止，所有 Caddy 实例配置为 HTTP 模式：

```bash
# 所有 Caddy 实例的 .env
SITE_DOMAIN=:80
```

适用于：Cloudflare（免费 HTTPS + CDN）、阿里云 SLB、AWS ALB、自建 HAProxy 等。

#### 方案 2：手动管理共享证书（推荐多 Caddy 实例方案）

多个 Caddy 实例挂载同一份证书文件，不触发 ACME 自动申请，无速率限制和 challenge 冲突问题：

```
用户 → [DNS轮询 / L4 LB] → Caddy A ──┐
                           → Caddy B ──┤── 挂载相同的 cert + key 文件
                           → Caddy C ──┘
```

**操作步骤：**

1. 在任意一台机器上用 certbot / acme.sh 申请证书：

```bash
# 使用 certbot
certbot certonly --standalone -d api.sub2api.com

# 或使用 acme.sh（支持 DNS 验证，适合无法开放 80 端口的场景）
acme.sh --issue -d api.sub2api.com --dns dns_cf
```

2. 将证书文件分发到所有前端机器的 `ssl/` 目录：

```bash
# 每台前端机器
mkdir -p /opt/sub2api/ssl
scp root@cert-server:/etc/letsencrypt/live/api.sub2api.com/fullchain.pem /opt/sub2api/ssl/
scp root@cert-server:/etc/letsencrypt/live/api.sub2api.com/privkey.pem /opt/sub2api/ssl/
```

3. 配置 `.env`：

```bash
SITE_DOMAIN=api.sub2api.com
BACKEND_SERVERS=10.0.1.10:8080,10.0.1.11:8080

# 手动证书路径（容器内路径）
TLS_CERT=/etc/caddy/ssl/fullchain.pem
TLS_KEY=/etc/caddy/ssl/privkey.pem
```

4. 取消 `docker-compose.caddy.yml` 中 ssl volume 挂载的注释：

```yaml
volumes:
  - ./ssl/fullchain.pem:/etc/caddy/ssl/fullchain.pem:ro
  - ./ssl/privkey.pem:/etc/caddy/ssl/privkey.pem:ro
```

5. 启动：

```bash
docker compose -f docker-compose.caddy.yml up -d
```

**证书续期：** Let's Encrypt 证书有效期 90 天，续期后将新证书分发到所有机器并重启容器：

```bash
# 续期（在申请证书的机器上）
certbot renew

# 分发到所有前端机器后重启
docker compose -f docker-compose.caddy.yml restart
```

> 💡 可配合 cron job 自动完成续期 + 分发 + 重启。

#### 方案 3：Caddy 集群存储（多实例自动共享证书）

多个 Caddy 实例通过共享存储后端同步证书，避免重复申请：

```
用户 → [DNS轮询 / L4 LB] → Caddy A ──┐
                           → Caddy B ──┤── 共享证书存储 (Consul / Redis / S3)
                           → Caddy C ──┘
```

需要自定义编译 Caddy 并加入存储插件（如 `caddy-storage-consul`、`caddy-storage-redis`），在 Caddyfile 中配置：

```caddyfile
{
    storage consul {
        address "consul:8500"
        prefix "caddy"
    }
}

api.sub2api.com {
    # ... 路由配置 ...
}
```

> ⚠️ 此方案需要自定义 Caddy 镜像，部署复杂度较高，仅在对前端高可用有严格要求时使用。

#### 方案 4：纯内网部署（无 HTTPS 冲突）

所有 Caddy 实例都使用 HTTP 模式（`SITE_DOMAIN=:80`），不涉及证书申请，多实例无冲突：

```bash
SITE_DOMAIN=:80
```

### 推荐生产架构

大多数生产环境推荐以下架构：

```
用户 → Cloudflare (CDN + HTTPS + DDoS防护)
         → 单台 Caddy/Nginx (HTTP 模式)
              → 多台后端 (round_robin 负载均衡)
```

或需要前端高可用时：

```
用户 → Cloudflare (CDN + HTTPS)
         → 云 SLB / HAProxy
              → 多台 Caddy/Nginx (HTTP 模式)
                   → 多台后端
```

---

## Nginx 配置说明

### 自定义 Nginx 配置

如果需要完全自定义 Nginx 配置（如添加 HTTPS），可以挂载自定义文件：

```yaml
# docker-compose.frontend.yml 中
volumes:
  - ./custom-nginx.conf:/etc/nginx/nginx.conf:ro
```

此时 `BACKEND_SERVERS` 环境变量不再生效，需要在自定义文件中手动配置 upstream。

### 负载均衡策略

默认使用 round_robin（轮询），可在自定义 nginx.conf 中修改：

```nginx
upstream backend {
    # ip_hash;      # 基于客户端 IP 的粘性会话
    # least_conn;   # 最少连接
    server 10.0.1.10:8080;
    server 10.0.1.11:8080;
    keepalive 64;
}
```

### HTTPS/TLS 配置

```nginx
server {
    listen 443 ssl http2;
    server_name your-domain.com;

    ssl_certificate     /etc/nginx/ssl/fullchain.pem;
    ssl_certificate_key /etc/nginx/ssl/privkey.pem;
    ssl_protocols       TLSv1.2 TLSv1.3;

    # ... 其余配置与 80 端口相同 ...
}

# HTTP → HTTPS 重定向
server {
    listen 80;
    server_name your-domain.com;
    return 301 https://$host$request_uri;
}
```

---

## 重要注意事项

### 1. JWT_SECRET 必须一致

所有后端实例 **必须** 使用相同的 `JWT_SECRET`。否则用户被路由到不同后端时 Token 验证失败，导致被登出。

```bash
# 生成密钥
openssl rand -hex 32
```

### 2. TOTP_ENCRYPTION_KEY 必须一致

如果启用了 2FA，所有后端实例必须使用相同的 `TOTP_ENCRYPTION_KEY`。

### 3. 数据库迁移

首次部署时，**只启动 1 个后端实例**完成数据库初始化。确认就绪后再启动其余实例。

### 4. 前端配置注入差异

集群模式下 Nginx 直接服务 `index.html`，无法注入 `window.__APP_CONFIG__`：
- 首次加载 site_name/logo 有短暂默认值显示（~100-300ms）
- 前端自动通过 `GET /api/v1/settings/public` 回退获取
- 后续导航不受影响

### 5. session_id Header

Nginx 配置已包含 `underscores_in_headers on`，支持 Sora 粘性会话的 `session_id` header。外部 LB 也需确保不过滤下划线 header。

### 6. WebSocket 与 SSE

已处理的特殊路由：
- `/responses` — WebSocket 升级（超时 3600s）
- `/api/v1/admin/ops/ws/qps` — 管理后台 WebSocket
- 所有 API 路由 — `proxy_buffering off`（SSE 流式响应不缓冲）

---

## 文件清单

### 镜像构建

| 文件 | 说明 |
|------|------|
| `deploy/frontend/Dockerfile` | 前端镜像 — Nginx + Vue 3 dist |
| `deploy/frontend/Dockerfile.caddy` | 前端镜像 — Caddy + Vue 3 dist |
| `deploy/frontend/nginx.conf` | Nginx 静态配置（单机参考） |
| `deploy/frontend/nginx.conf.template` | Nginx 模板（多机用，envsubst） |
| `deploy/frontend/entrypoint.sh` | Nginx 启动脚本 |
| `deploy/frontend/Caddyfile` | Caddy 静态配置（单机参考） |
| `deploy/frontend/Caddyfile.template` | Caddy 模板（多机用，envsubst） |
| `deploy/frontend/caddy-entrypoint.sh` | Caddy 启动脚本 |
| `deploy/backend/Dockerfile` | 后端镜像（仅 API，不含前端） |

### 单机多容器

| 文件 | 说明 |
|------|------|
| `deploy/docker-compose.cluster.yml` | 单机编排（frontend + backend + db + redis） |
| `deploy/.env.cluster.example` | 环境变量模板 |

### 多机部署

| 文件 | 部署到 | 说明 |
|------|--------|------|
| `deploy/frontend/docker-compose.yml` | 前端/LB 机器 | Nginx 方案 |
| `deploy/frontend/docker-compose.caddy.yml` | 前端/LB 机器 | Caddy 方案 |
| `deploy/frontend/.env.example` | 前端/LB 机器 | 配置后端地址和域名 |
| `deploy/backend/docker-compose.yml` | 每台后端机器 | Go API 容器 |
| `deploy/backend/.env.example` | 每台后端机器 | 配置 DB/Redis/密钥 |
| `deploy/db/docker-compose.yml` | 数据库机器 | PostgreSQL + Redis |
| `deploy/db/.env.example` | 数据库机器 | 配置密码和参数 |

---

## 故障排除

### 前端页面空白

```bash
# Nginx
docker logs sub2api-frontend
docker exec sub2api-frontend ls /usr/share/nginx/html/

# Caddy
docker logs sub2api-frontend-caddy
docker exec sub2api-frontend-caddy ls /srv/sub2api/
```

### API 请求返回 502

```bash
# 检查后端健康
curl http://<backend-ip>:8080/health

# 检查前端到后端的网络连通性（Nginx）
docker exec sub2api-frontend wget -qO- http://<backend-ip>:8080/health

# 检查前端到后端的网络连通性（Caddy）
docker exec sub2api-frontend-caddy wget -qO- http://<backend-ip>:8080/health
```

### 登录后被反复登出

- 确认所有后端实例 `JWT_SECRET` 完全一致
- 确认所有后端实例时区 `TZ` 一致

### 2FA 验证码无效

- 确认所有后端实例 `TOTP_ENCRYPTION_KEY` 完全一致

### WebSocket 连接失败

- 确认外部 CDN/LB（如 Cloudflare）已开启 WebSocket 支持
- Nginx：检查 `/responses` 和 `/api/` 路由的 WebSocket 升级头配置
- Caddy：WebSocket 自动处理，一般无需额外配置

### Caddy HTTPS 证书申请失败

- 确认域名已正确解析到本机公网 IP（`dig +short your-domain.com`）
- 确认 80 和 443 端口对外可访问（ACME HTTP-01 challenge 需要）
- 检查 Caddy 日志：`docker logs sub2api-frontend-caddy`
- 确认 `caddy_data` volume 未被误删（证书存储在此 volume 中）
