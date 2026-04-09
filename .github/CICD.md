# Sub2API CI/CD 流水线配置说明

## 概览

本项目使用 GitHub Actions 实现完整的 CI/CD 流水线，共包含 **8 条工作流**（含 1 条备用），覆盖持续集成、安全扫描、镜像构建和生产/测试部署。

```
┌─────────────────────────────────────────────────────────────────┐
│                    GitHub Actions 工作流全景                      │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  CI 阶段 (每次 push / PR)                                        │
│  ├── backend-ci.yml ─── Go 单元测试 + 集成测试 + golangci-lint   │
│  └── security-scan.yml ─ govulncheck + pnpm audit (+ 每周定时)   │
│                                                                 │
│  生产部署 (推送 v* tag 或手动触发，前后端分离)                       │
│  ├── deploy-frontend.yml ──── rsync 到 Nginx 宿主机             │
│  └── deploy-backend.yml ───── GHCR → SSH 滚动部署               │
│                                                                 │
│  镜像构建 (推送 v* tag 或手动触发，仅构建不部署)                    │
│  └── build-frontend-image.yml ─ 前端 Docker 镜像 → GHCR       │
│                                                                 │
│  测试部署 (push test 或手动触发，前后端分离)                       │
│  ├── deploy-test-frontend.yml ─ rsync 到测试 Nginx 宿主机       │
│  └── deploy-test-backend.yml ── GHCR → SSH 测试环境部署         │
│                                                                 │
│  备用                                                            │
│  └── release.yml ─── 原 GoReleaser 一体化发布（仅手动触发）       │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## 1. CI — 后端持续集成 (`backend-ci.yml`)

| 属性 | 值 |
|------|-----|
| **文件** | `.github/workflows/backend-ci.yml` |
| **触发** | 所有 `push` 和 `pull_request` |
| **权限** | `contents: read` |

### Jobs

| Job | 说明 |
|-----|------|
| `test` | 校验 Go 1.26.1 版本 → 运行 `make test-unit` + `make test-integration` |
| `golangci-lint` | 使用 golangci-lint v2.9，超时 30 分钟 |

### 注意事项

- Go 版本锁定为 `backend/go.mod` 中声明的版本（当前 1.26.1），并通过 `grep` 强制验证
- 两个 job 并行执行以缩短整体时间

---

## 2. 安全扫描 (`security-scan.yml`)

| 属性 | 值 |
|------|-----|
| **文件** | `.github/workflows/security-scan.yml` |
| **触发** | 所有 `push`、`pull_request`，以及每周一 UTC 03:00 定时 |
| **权限** | `contents: read` |

### Jobs

| Job | 说明 |
|-----|------|
| `backend-security` | `govulncheck ./...` 扫描 Go 依赖已知漏洞（超时 15 分钟） |
| `frontend-security` | `pnpm audit --prod --audit-level=high` → 通过 `check_pnpm_audit_exceptions.py` 校验例外清单 |

### 前端审计例外机制

`.github/audit-exceptions.yml` 中可声明已知且已缓解的漏洞例外。格式：

```yaml
version: 1
exceptions:
  - package: xlsx
    advisory: "GHSA-xxx"
    severity: high
    reason: "仅管理员导出使用，已改为动态导入"
    mitigation: "限制导出权限和数据范围"
    expires_on: "2026-04-05"
    owner: "security@your-domain"
```

> 例外到期后，工作流将自动失败以提醒更新。

---

## 3. 测试前端部署 (`deploy-test-frontend.yml`)

| 属性 | 值 |
|------|-----|
| **文件** | `.github/workflows/deploy-test-frontend.yml` |
| **触发** | push `test` 分支（`frontend/**` 或 `deploy/frontend/**` 变更）或手动触发 |
| **权限** | `contents: read` |
| **环境** | `test-frontend` |

与 `deploy-frontend.yml` 结构完全一致，区别仅为目标分支和环境不同。

### 手动触发参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `skip_deploy` | 仅构建，跳过部署 | `false` |
| `target_hosts` | 覆盖目标主机 | 使用 secret |
| `deploy_nginx_conf` | 同时部署 nginx.conf | `false` |

---

## 4. 测试后端部署 (`deploy-test-backend.yml`)

| 属性 | 值 |
|------|-----|
| **文件** | `.github/workflows/deploy-test-backend.yml` |
| **触发** | push `test` 分支（`backend/**` 或 `deploy/backend/**` 变更）或手动触发 |
| **权限** | `contents: read`, `packages: write` |
| **环境** | `test-backend` |

与 `deploy-backend.yml` 结构完全一致，区别仅为目标分支、环境和镜像标签不同。

### 手动触发参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `tag` | 自定义镜像标签 | `test-{sha7}` |
| `skip_deploy` | 仅构建推送镜像，跳过 SSH 部署 | `false` |
| `target_hosts` | 覆盖目标主机 | 使用 secret |

### 生产 vs 测试环境对比

| | 生产 | 测试 |
|---|---|---|
| **触发** | 推送 `v*` tag | push `test` 分支 |
| **前端流水线** | `deploy-frontend.yml` | `deploy-test-frontend.yml` |
| **后端流水线** | `deploy-backend.yml` | `deploy-test-backend.yml` |
| **Environment** | `production-frontend` / `production-backend` | `test-frontend` / `test-backend` |
| **Secrets 前缀** | `FRONTEND_*` / `BACKEND_*` | `TEST_FRONTEND_*` / `TEST_BACKEND_*` |
| **后端镜像标签** | `{version}` + `latest` | `test-{sha7}` + `test-latest` |
| **镜像仓库** | GHCR (+可选 DockerHub) | 仅 GHCR |

---

## 5. 原一体化发布 (`release.yml`) — 备用

| 属性 | 值 |
|------|-----|
| **文件** | `.github/workflows/release.yml` |
| **状态** | **备用，仅手动触发** (`workflow_dispatch`) |
| **权限** | `contents: write`, `packages: write` |

> 这是原有的 GoReleaser 一体化发布工作流。保留作为备用，可在 GitHub Actions UI 手动触发。

---

## 6. 生产前端部署 (`deploy-frontend.yml`)

| 属性 | 值 |
|------|-----|
| **文件** | `.github/workflows/deploy-frontend.yml` |
| **触发** | 推送 `v*` tag（如 `v1.0.0`）或手动触发 |
| **权限** | `contents: read` |
| **环境** | `production-frontend`（deploy job） |

### 架构

```
前端不使用 Docker，Nginx 直接运行在宿主机上。

┌──────────────┐     rsync      ┌────────────────────┐
│ GitHub CI    │ ──────────────→│  Nginx 宿主机 1     │
│ pnpm build   │     rsync      ├────────────────────┤
│ → dist/      │ ──────────────→│  Nginx 宿主机 2     │
│              │     rsync      ├────────────────────┤
│              │ ──────────────→│  Nginx 宿主机 N     │
└──────────────┘                └────────────────────┘
                              systemctl reload nginx
```

### 流程

1. **build**: checkout → pnpm install → pnpm build → 上传 dist 产物（保留 7 天）
2. **deploy**: 逐台服务器执行：
   - 备份当前 html 目录到 `{HTML_PATH}.bak`
   - `rsync --delete` 同步新静态文件
   - （可选）同步 `nginx.conf`
   - `nginx -t` 验证配置 → `systemctl reload nginx` 优雅重载（零停机）

### 手动触发参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `skip_deploy` | 仅构建，跳过部署 | `false` |
| `target_hosts` | 覆盖目标主机列表（逗号分隔） | 使用 secret |
| `deploy_nginx_conf` | 同时部署 `nginx.conf` 配置文件 | `false` |

### Nginx 配置文件说明

| 文件 | 说明 |
|------|------|
| `deploy/frontend/nginx.conf` | 静态配置，手动编辑 upstream 后端地址 |
| `deploy/frontend/nginx.conf.template` | 动态模板，配合 Docker entrypoint 使用 |

宿主机部署时使用 `nginx.conf`，需手动编辑 `upstream backend` 块中的后端地址：

```nginx
upstream backend {
    server 10.0.1.10:8080;
    server 10.0.1.11:8080;
    keepalive 64;
}
```

---

## 7. 生产后端部署 (`deploy-backend.yml`)

| 属性 | 值 |
|------|-----|
| **文件** | `.github/workflows/deploy-backend.yml` |
| **触发** | 推送 `v*` tag（如 `v1.0.0`）或手动触发 |
| **权限** | `contents: read`, `packages: write` |
| **环境** | `production-backend`（deploy job） |

### 架构

```
┌──────────────┐   push image   ┌─────────┐
│ GitHub CI    │ ──────────────→│  GHCR   │
│ Docker Build │                └────┬────┘
└──────────────┘                     │
                                     │ docker compose pull
       ┌─────────────────────────────┼──────────────────────┐
       ▼                             ▼                      ▼
┌──────────────┐          ┌──────────────┐        ┌──────────────┐
│ 后端机器 1    │          │ 后端机器 2    │        │ 后端机器 N    │
│ docker-compose│         │ docker-compose│        │ docker-compose│
│ + health check│         │ + health check│        │ + health check│
└──────────────┘          └──────────────┘        └──────────────┘
       │                         │                        │
       └─────────── 共享外部 PostgreSQL + Redis ──────────┘
```

### 流程

1. **build-and-push**:
   - 使用 `deploy/backend/Dockerfile` 多阶段构建（纯后端，无嵌入前端）
   - **双架构构建**：`linux/amd64` + `linux/arm64`（Go 原生交叉编译，不经 QEMU 模拟，速度接近单架构）
   - 推送到 GHCR，标签：`{version}` + `latest`（如 `1.0.0` + `latest`）
   - 使用 GitHub Actions Cache 加速 Docker 构建
2. **deploy**: 使用 `appleboy/ssh-action` SSH 到所有后端服务器（原生支持逗号分隔多主机）：
   - 更新 `.env` 中的 `BACKEND_TAG` 为当前版本号
   - `docker compose pull backend`
   - `docker compose up -d --remove-orphans backend`
   - 等待健康检查通过（最多 5 分钟，每 10 秒检查一次）
   - 清理 7 天以上的旧镜像

### 手动触发参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `tag` | 版本号（如 `v1.0.0`） | 必填 |
| `skip_deploy` | 仅构建推送镜像，跳过 SSH 部署 | `false` |
| `target_hosts` | 覆盖目标主机列表 | 使用 secret |

### 后端 Docker Compose 配置

每台后端机器上的 `docker-compose.yml`（来自 `deploy/backend/docker-compose.yml`）：

- 镜像：`${BACKEND_IMAGE}:${BACKEND_TAG}`（部署时自动更新为具体版本号）
- 连接外部 PostgreSQL 和 Redis
- 关键约束：**所有实例必须共享相同的 `JWT_SECRET` 和 `TOTP_ENCRYPTION_KEY`**
- 首个实例应单独启动以完成数据库初始化

---

## Secrets 配置清单

### 生产前端 Secrets（`production-frontend` Environment）

| Secret | 说明 | 必需 | 默认值 |
|--------|------|:----:|--------|
| `FRONTEND_DEPLOY_HOSTS` | Nginx 服务器 IP 列表（逗号分隔） | ✅ | — |
| `FRONTEND_DEPLOY_USER` | SSH 用户 | ✅ | `root` |
| `FRONTEND_DEPLOY_PASSWORD` | SSH 密码 | ✅ | — |
| `FRONTEND_DEPLOY_PORT` | SSH 端口 | — | `22` |
| `FRONTEND_NGINX_HTML_PATH` | Nginx html 根目录 | — | `/var/www/html` |
| `FRONTEND_NGINX_CONF_PATH` | Nginx 配置文件路径 | — | `/etc/nginx/nginx.conf` |

### 生产后端 Secrets（`production-backend` Environment）

| Secret | 说明 | 必需 | 默认值 |
|--------|------|:----:|--------|
| `BACKEND_DEPLOY_HOSTS` | 后端服务器 IP 列表（逗号分隔） | ✅ | — |
| `BACKEND_DEPLOY_USER` | SSH 用户 | ✅ | `root` |
| `BACKEND_DEPLOY_PASSWORD` | SSH 密码 | ✅ | — |
| `BACKEND_DEPLOY_PORT` | SSH 端口 | — | `22` |
| `BACKEND_DEPLOY_PATH` | docker-compose 所在目录 | — | `/opt/sub2api/backend` |

### 测试前端 Secrets（`test-frontend` Environment）

| Secret | 说明 | 必需 | 默认值 |
|--------|------|:----:|--------|
| `TEST_FRONTEND_DEPLOY_HOSTS` | 测试 Nginx 服务器 IP 列表 | ✅ | — |
| `TEST_FRONTEND_DEPLOY_USER` | SSH 用户 | ✅ | `root` |
| `TEST_FRONTEND_DEPLOY_PASSWORD` | SSH 密码 | ✅ | — |
| `TEST_FRONTEND_DEPLOY_PORT` | SSH 端口 | — | `22` |
| `TEST_FRONTEND_NGINX_HTML_PATH` | Nginx html 根目录 | — | `/var/www/html` |

### 测试后端 Secrets（`test-backend` Environment）

| Secret | 说明 | 必需 | 默认值 |
|--------|------|:----:|--------|
| `TEST_BACKEND_DEPLOY_HOSTS` | 测试后端服务器 IP 列表 | ✅ | — |
| `TEST_BACKEND_DEPLOY_USER` | SSH 用户 | ✅ | `root` |
| `TEST_BACKEND_DEPLOY_PASSWORD` | SSH 密码 | ✅ | — |
| `TEST_BACKEND_DEPLOY_PORT` | SSH 端口 | — | `22` |
| `TEST_BACKEND_DEPLOY_PATH` | docker-compose 所在目录 | — | `/opt/sub2api/backend` |

### 发布 Secrets

| Secret | 说明 | 必需 |
|--------|------|:----:|
| `DOCKERHUB_USERNAME` | DockerHub 用户名（留空跳过 DockerHub 推送） | — |
| `DOCKERHUB_TOKEN` | DockerHub 访问令牌 | — |
| `TELEGRAM_BOT_TOKEN` | Telegram 机器人令牌（发版通知） | — |
| `TELEGRAM_CHAT_ID` | Telegram 通知目标 Chat ID | — |

> `GITHUB_TOKEN` 由 GitHub 自动提供，用于 GHCR 推送和 Release 创建。

---

## Environments 配置

需要在 GitHub 仓库 **Settings → Environments** 中创建：

| Environment | 用途 | 使用流水线 | 建议配置 |
|-------------|------|---------|--------|
| `production-frontend` | 生产前端部署 | `deploy-frontend.yml` | 添加审批人 / 分支保护 |
| `production-backend` | 生产后端部署 | `deploy-backend.yml` | 添加审批人 / 分支保护 |
| `test-frontend` | 测试前端部署 | `deploy-test-frontend.yml` | 可选审批 |
| `test-backend` | 测试后端部署 | `deploy-test-backend.yml` | 可选审批 |

> 配置 Environment 保护规则后，每次部署需经过审批才能执行，防止意外推送到生产。测试环境可根据需要配置较宽松的规则。

---

## 首次部署指南

### 前端服务器初始化

在每台前端（Nginx）宿主机上执行：

```bash
# 1. 安装 nginx（如未安装）
apt update && apt install -y nginx

# 2. 准备静态文件目录（Ubuntu apt nginx 默认路径）
mkdir -p /var/www/html

# 3. 部署 nginx.conf（从仓库复制并编辑 upstream 后端地址）
#    修改 upstream backend { ... } 块中的服务器地址
cp nginx.conf /etc/nginx/nginx.conf
vim /etc/nginx/nginx.conf

# 4. 测试并启动
nginx -t && systemctl enable --now nginx
```

### 后端服务器初始化

在每台后端机器上执行：

```bash
# 1. 安装 Docker 和 Docker Compose
curl -fsSL https://get.docker.com | sh

# 2. 准备部署目录
mkdir -p /opt/sub2api/backend
cd /opt/sub2api/backend

# 3. 创建 docker-compose.yml 和 .env
#    从仓库 deploy/backend/ 复制文件，并编辑 .env
cp docker-compose.yml .
cp .env.example .env
vim .env  # 设置 DATABASE_HOST, REDIS_HOST, JWT_SECRET 等

# 4. 如果使用 GHCR 私有镜像，需要登录
echo $GITHUB_TOKEN | docker login ghcr.io -u USERNAME --password-stdin

# 5. 启动（第一台先单独启动，完成数据库初始化）
docker compose up -d
docker compose logs -f  # 确认启动成功
```

### GitHub Secrets 配置

在 GitHub 仓库 **Settings → Environments** 中，为每个环境配置对应的 Secrets：

| Environment | 必需 Secrets |
|---|---|
| `production-frontend` | `FRONTEND_DEPLOY_HOSTS`, `FRONTEND_DEPLOY_USER`, `FRONTEND_DEPLOY_PASSWORD` |
| `production-backend` | `BACKEND_DEPLOY_HOSTS`, `BACKEND_DEPLOY_USER`, `BACKEND_DEPLOY_PASSWORD` |
| `test-frontend` | `TEST_FRONTEND_DEPLOY_HOSTS`, `TEST_FRONTEND_DEPLOY_USER`, `TEST_FRONTEND_DEPLOY_PASSWORD` |
| `test-backend` | `TEST_BACKEND_DEPLOY_HOSTS`, `TEST_BACKEND_DEPLOY_USER`, `TEST_BACKEND_DEPLOY_PASSWORD` |

> 确保目标服务器已开启 SSH 密码登录（`/etc/ssh/sshd_config` 中 `PasswordAuthentication yes`）。

---

## 工作流触发规则一览

| 工作流 | v* tag | push test | 手动 | 其他 |
|--------|:-:|:-:|:-:|:-:|
| CI (`backend-ci.yml`) | — | ✅ | — | 所有 push/PR |
| Security Scan | — | ✅ | — | 所有 push/PR + 每周一 |
| Deploy Frontend (`deploy-frontend.yml`) | ✅ | — | ✅ | — |
| Deploy Backend (`deploy-backend.yml`) | ✅ | — | ✅ | — |
| Build Frontend Image (`build-frontend-image.yml`) | ✅ | — | ✅ | — |
| Deploy Test Frontend (`deploy-test-frontend.yml`) | — | ✅¹ | ✅ | — |
| Deploy Test Backend (`deploy-test-backend.yml`) | — | ✅² | ✅ | — |
| Release 备用 (`release.yml`) | — | — | ✅ | — |

- ¹ 仅 `frontend/**` 或 `deploy/frontend/**` 路径变更时触发
- ² 仅 `backend/**` 或 `deploy/backend/**` 路径变更时触发

---

## 常见操作

### 发布生产版本

```bash
# 创建版本 tag 并推送，自动触发前后端部署
git tag -a v1.2.3 -m "版本说明"
git push origin v1.2.3
```

> 推送 tag 后，`deploy-frontend.yml`、`deploy-backend.yml` 和 `build-frontend-image.yml` 同时触发。后端镜像标签为 `1.2.3`，并自动更新服务器 `.env` 中的 `BACKEND_TAG`。前端 Docker 镜像也会同步构建推送到 GHCR。

### 手动部署生产环境

GitHub → Actions → Deploy Backend → Run workflow：
- `tag`: `v1.2.3`（必填）
- `target_hosts`: `10.0.1.10`（可选，指定单台）
- `skip_deploy`: `true`（可选，仅构建镜像不部署）

### 部署到测试环境

```bash
# 推送代码到 test 分支即可自动触发测试环境部署
git push origin test

# 或从 feature 分支合并到 test
git checkout test
git merge feature/my-feature
git push origin test
```

> 前端和后端流水线独立触发，仅当对应路径有变更时才会执行。

---

## 故障排除

| 问题 | 排查 |
|------|------|
| 前端部署后页面空白 | 检查 rsync 目标路径是否和 nginx.conf 中 `root` 一致 |
| nginx reload 失败 | 查看 `nginx -t` 输出；检查 upstream 后端地址是否可达 |
| 后端健康检查超时 | 检查数据库/Redis 连接；查看 `docker compose logs backend` |
| GHCR 镜像拉取失败 | 确认服务器已 `docker login ghcr.io`；检查镜像是否公开 |
| 部署 SSH 连接失败 | 确认密码正确且服务器已开启 `PasswordAuthentication yes` |
| 测试环境未触发部署 | 确认推送到 `test` 分支且修改了对应路径（`frontend/**` 或 `backend/**`） |
