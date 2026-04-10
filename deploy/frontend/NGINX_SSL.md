# Sub2API 前端 Nginx 配置与 SSL 证书部署指南

本文档面向多机宿主机部署场景（Ubuntu >= 20.04，apt 安装 nginx），涵盖 Nginx 配置说明、Let's Encrypt SSL 证书申请与管理。

---

## 1. 前提条件

- Ubuntu >= 20.04
- Nginx 通过 `apt` 安装（版本 >= 1.18）
- 域名 DNS 已解析到 nginx 服务器 IP
- 80 / 443 端口对外可访问

---

## 2. 安装

```bash
apt update && apt install -y nginx certbot
```

---

## 3. SSL 证书申请（Let's Encrypt）

### 3.1 两种验证方式

certbot 支持多种域名验证方式，常用的有两种：

| | `--standalone` | `--webroot` |
|---|---|---|
| **原理** | certbot 自己启动临时 HTTP 服务器监听 80 端口 | certbot 将验证文件写入指定目录，由已运行的 web 服务器提供访问 |
| **是否需要停 nginx** | ✅ 需要（80 端口必须空闲） | ❌ 不需要 |
| **前提** | 80 端口对外可访问 | nginx 已运行且配置了 `/.well-known/acme-challenge/` 路由 |
| **适用场景** | 首次申请（nginx 尚未配置好） | nginx 已运行时申请 / 续期 |

### 3.2 首次申请

**方式一：standalone**（nginx 尚未启动或配置好时）

```bash
# 确保 nginx 未占用 80 端口
systemctl stop nginx

# 申请证书
certbot certonly --standalone -d test.flux-code.cc

# 申请完成后启动 nginx
systemctl start nginx
```

**方式二：webroot**（nginx 已运行且配置了 ACME challenge 路由时）

```bash
# nginx.conf 中需要有以下配置：
#   location /.well-known/acme-challenge/ { root /var/www/html; }

# 申请证书（无需停 nginx）
certbot certonly --webroot -w /var/www/html -d test.flux-code.cc
```

> `-w` 参数指定的目录必须与 nginx.conf 中 `/.well-known/acme-challenge/` 的 `root` 一致。
> 本项目配置为 `/var/www/html`。

### 3.3 证书文件

Let's Encrypt 申请后在 `/etc/letsencrypt/live/test.flux-code.cc/` 下生成 4 个文件：

| 文件 | 说明 | nginx 配置项 |
|------|------|-------------|
| `fullchain.pem` | 服务器证书 + 中间证书链 | `ssl_certificate` |
| `privkey.pem` | 私钥 | `ssl_certificate_key` |
| `chain.pem` | 仅中间证书 | `ssl_trusted_certificate`（OCSP stapling） |
| `cert.pem` | 仅服务器证书 | 不需要 |

### 3.4 复制证书到 nginx 目录

nginx.conf 中配置的证书路径为 `/etc/nginx/certs/test.flux-code.cc/`，需手动复制：

```bash
mkdir -p /etc/nginx/certs/test.flux-code.cc

cp /etc/letsencrypt/live/test.flux-code.cc/fullchain.pem /etc/nginx/certs/test.flux-code.cc/
cp /etc/letsencrypt/live/test.flux-code.cc/privkey.pem   /etc/nginx/certs/test.flux-code.cc/
cp /etc/letsencrypt/live/test.flux-code.cc/chain.pem     /etc/nginx/certs/test.flux-code.cc/
```

### 3.5 多机部署

只需在**一台机器**上申请证书，然后将以下 3 个文件复制到每台 nginx 机器的 `/etc/nginx/certs/test.flux-code.cc/` 目录：

- `fullchain.pem`
- `privkey.pem`
- `chain.pem`

---

## 4. 证书自动续期

certbot 通过 systemd timer 自动续期（每天检查两次）。

### 4.1 配置续期钩子

在**运行 certbot 的那台机器**上配置 deploy hook，续期后自动同步证书并 reload nginx：

```bash
cat > /etc/letsencrypt/renewal-hooks/deploy/copy-certs.sh << 'EOF'
#!/bin/bash
cp /etc/letsencrypt/live/test.flux-code.cc/{fullchain,privkey,chain}.pem /etc/nginx/certs/test.flux-code.cc/
systemctl reload nginx
EOF
chmod +x /etc/letsencrypt/renewal-hooks/deploy/copy-certs.sh
```

### 4.2 验证续期

```bash
certbot renew --dry-run
```

### 4.3 多机证书同步

deploy hook 只会在运行 certbot 的机器上执行。其他 nginx 机器需要额外同步机制，例如：

```bash
# 在 certbot 机器的 deploy hook 中追加 scp 同步
cat >> /etc/letsencrypt/renewal-hooks/deploy/copy-certs.sh << 'SYNC'

# 同步到其他 nginx 机器
for HOST in 10.0.1.21 10.0.1.22; do
    scp /etc/nginx/certs/test.flux-code.cc/{fullchain,privkey,chain}.pem root@${HOST}:/etc/nginx/certs/test.flux-code.cc/
    ssh root@${HOST} "systemctl reload nginx"
done
SYNC
```

> 或使用 cron 从各机器定期拉取。

---

## 5. Nginx 配置说明

配置文件：`deploy/frontend/nginx.conf`

此文件为完整的 `/etc/nginx/nginx.conf` 替换文件（非 `conf.d` 片段），包含以下主要部分：

### 5.1 架构概览

```
客户端 ──→ Nginx (443 HTTPS)
             ├─ /api/*, /v1/*, /v1beta/*, /responses, /sora/*, ...  ──→ upstream backend
             └─ /*, /assets/*, 静态资源                              ──→ /var/www/html (本地文件)
```

### 5.2 HTTP server（端口 80）

- 服务 Let's Encrypt ACME challenge（`/.well-known/acme-challenge/`）
- 其余请求全部 301 重定向到 HTTPS

### 5.3 HTTPS server（端口 443）

| 配置项 | 说明 |
|--------|------|
| **SSL 证书** | `/etc/nginx/certs/test.flux-code.cc/{fullchain,privkey}.pem` |
| **TLS 版本** | TLSv1.2 + TLSv1.3 |
| **OCSP Stapling** | 默认关闭（Let's Encrypt ISRG Root X1 证书不含 OCSP responder URL） |
| **HSTS** | `max-age=63072000`（2 年），含 `includeSubDomains` 和 `preload` |
| **HTTP/2** | 通过 `listen 443 ssl http2` 启用（兼容 nginx < 1.25.1） |

### 5.4 Backend upstream

需根据实际部署环境修改 `upstream backend` 块：

```nginx
upstream backend {
    # 单机
    server 127.0.0.1:8080;

    # 多机示例
    # server 10.0.1.10:8080;
    # server 10.0.1.11:8080;
    # server 10.0.1.12:8080;

    keepalive 64;
    keepalive_timeout 120s;
}
```

负载均衡策略：
- **round_robin**（默认）— 均匀分配
- **ip_hash** — 基于客户端 IP 的会话粘滞
- **least_conn** — 分配到最空闲的后端

### 5.5 反向代理路由

| 路由 | 说明 | 特殊配置 |
|------|------|---------|
| `/api/` | 管理 REST API | WebSocket 支持（admin ops ws） |
| `/v1/` | OpenAI 兼容网关 API | SSE/streaming |
| `/v1beta/` | Gemini 原生 API | SSE/streaming |
| `/responses` | OpenAI Responses API | WebSocket（超时 3600s） |
| `/chat/completions` | Chat Completions（无 /v1 前缀） | SSE/streaming |
| `/sora/` | Sora 客户端路由 | `underscores_in_headers on` |
| `/antigravity/` | Antigravity 路由 | — |
| `/setup/` | 初始化向导 | — |
| `/health` | 健康检查 | — |

### 5.6 静态文件

| 路径 | 说明 | 缓存策略 |
|------|------|---------|
| `/assets/` | Vite 构建产物（含 hash） | 永久缓存（`immutable`） |
| `*.ico, *.png, ...` | 图片/字体等 | 7 天 |
| `/` | SPA fallback → `index.html` | 不缓存（`no-cache`） |

静态文件根目录：`/var/www/html`

---

## 6. 部署步骤汇总

在**每台** nginx 宿主机上执行：

```bash
# 1. 安装
apt update && apt install -y nginx certbot

# 2. 准备静态文件目录
mkdir -p /var/www/html

# 3. 放置 SSL 证书（首台机器申请，其余机器复制）
mkdir -p /etc/nginx/certs/test.flux-code.cc
# cp fullchain.pem privkey.pem chain.pem 到上述目录

# 4. 部署 nginx.conf（修改 upstream backend 地址）
cp deploy/frontend/nginx.conf /etc/nginx/nginx.conf
vim /etc/nginx/nginx.conf

# 5. 测试并启动
nginx -t && systemctl enable --now nginx
```

---

## 7. 故障排除

| 问题 | 排查 |
|------|------|
| `unknown directive "http2"` | nginx 版本 < 1.25.1，使用 `listen 443 ssl http2`（当前配置已兼容） |
| `cannot load certificate` | 检查证书文件路径和权限（nginx 需要 root 或对应权限读取） |
| `ssl_stapling ignored, no OCSP responder URL` | Let's Encrypt ISRG Root X1 证书不含 OCSP URL，当前配置已默认关闭 OCSP stapling |
| certbot webroot 验证 404 | 确认 `-w` 参数与 nginx.conf 中 ACME challenge 的 `root` 一致（本项目为 `/var/www/html`） |
| 页面空白 | 检查 `/var/www/html` 是否有前端构建产物 |
| 502 Bad Gateway | 检查 upstream 后端地址是否正确且服务已启动 |
| 证书过期 | 运行 `certbot renew` 并同步到 `/etc/nginx/certs/` |
