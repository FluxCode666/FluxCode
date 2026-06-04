# MCP 余额查询服务设计

**日期**: 2026-06-04
**状态**: 已确认

---

## 1. 概述

在现有 FluxCode backend 中集成 MCP (Model Context Protocol) 服务，提供余额查询能力。每个用户拥有独立的 MCP 密钥用于鉴权，用户可在前端个人设置页面自行管理。

## 2. 技术选型

| 项目 | 选择 |
|------|------|
| MCP SDK | `mark3labs/mcp-go` |
| Transport | Streamable HTTP（集成在 backend 进程中） |
| MCP Key 存储 | `users` 表新增 `mcp_api_key` 字段 |

## 3. 数据模型

### 3.1 users 表新增字段

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| `mcp_api_key` | VARCHAR(128) | nullable, unique | MCP 密钥，`mcp-` + 64 位 hex |

- 前缀 `mcp-` 与现有 `admin-`（Admin API Key）、`sk-`（用户 API Key）区分
- nullable：用户默认无 MCP key，需手动生成
- unique：全局唯一，防止冲突

### 3.2 数据库迁移

新增迁移文件，为 `users` 表添加 `mcp_api_key` 列及唯一索引。

## 4. API 设计

### 4.1 MCP Key 管理 API（需 JWT 鉴权）

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/api/v1/user/mcp-key` | 查看 MCP key 状态（脱敏显示） |
| `POST` | `/api/v1/user/mcp-key/regenerate` | 生成/重新生成 MCP key（返回完整 key，仅此一次） |
| `DELETE` | `/api/v1/user/mcp-key` | 删除 MCP key |

**GET 响应**:
```json
{
  "exists": true,
  "masked_key": "mcp-a1b2c3d4...x9y0",
  "mcp_config": {
    "mcpServers": {
      "FluxCode": {
        "url": "https://your-domain.com/mcp",
        "headers": {
          "Authorization": "Bearer mcp-xxxxxxxx"
        }
      }
    }
  }
}
```

**POST 响应**:
```json
{
  "key": "mcp-a1b2c3d4e5f6...full-key",
  "mcp_config": {
    "mcpServers": {
      "FluxCode": {
        "url": "https://your-domain.com/mcp",
        "headers": {
          "Authorization": "Bearer mcp-a1b2c3d4e5f6...full-key"
        }
      }
    }
  }
}
```

- `mcp_config` 中的 `url` 从配置项 `MCP_PUBLIC_URL` 获取（环境变量或配置文件）
- 前端提供一键复制按钮：复制 MCP Key、复制 MCP 配置 JSON

### 4.2 MCP Server 端点

| 端点 | 说明 |
|------|------|
| `POST /mcp` | MCP JSON-RPC 入口，经 MCP 鉴权中间件 |

## 5. 鉴权设计

### 5.1 MCP 鉴权中间件

- 从 `Authorization: Bearer <mcp-key>` 或 `x-api-key: <mcp-key>` 提取 key
- 查 `users` 表匹配 `mcp_api_key` 字段
- 验证用户状态（active）
- 通过后将 `user` 注入 gin context

### 5.2 错误码

| 场景 | HTTP 状态码 | 错误码 | 说明 |
|------|------------|--------|------|
| 无 MCP key | 401 | `MCP_KEY_REQUIRED` | 未提供认证信息 |
| MCP key 无效 | 401 | `INVALID_MCP_KEY` | key 不匹配或不存在 |
| 用户被禁用 | 401 | `USER_INACTIVE` | 关联用户不可用 |
| 生成 key 失败 | 500 | `INTERNAL_ERROR` | 服务端错误 |

## 6. MCP Tool 定义

### 6.1 get_balance

```json
{
  "name": "get_balance",
  "description": "查询当前账户余额及用量概览",
  "inputSchema": {
    "type": "object",
    "properties": {}
  }
}
```

**返回内容**:
```json
{
  "balance": 10.50,
  "username": "user@example.com",
  "total_recharged": 100.00,
  "monthly_usage": 25.30,
  "monthly_requests": 1523
}
```

## 7. 架构概览

```
┌─────────────────────────────────────────────────────┐
│                    Frontend                          │
│  ┌─────────────────────────────────────────────┐    │
│  │  个人设置 > MCP Key 管理                      │    │
│  │  - 查看状态 / 生成 / 删除 / 复制 Key          │    │
│  │  - 复制 MCP 配置 JSON                        │    │
│  └─────────────────────────────────────────────┘    │
└──────────────────────┬──────────────────────────────┘
                       │ JWT Auth
┌──────────────────────▼──────────────────────────────┐
│                  Backend (Gin)                       │
│                                                      │
│  ┌──────────────────────────────────────────┐       │
│  │  /api/v1/user/mcp-key                    │       │
│  │  UserHandler.GetMCPKey                   │       │
│  │  UserHandler.RegenerateMCPKey            │       │
│  │  UserHandler.DeleteMCPKey                │       │
│  └──────────────────────────────────────────┘       │
│                                                      │
│  ┌──────────────────────────────────────────┐       │
│  │  /mcp  (MCP JSON-RPC)                    │       │
│  │  MCPAuthMiddleware → MCPServer            │       │
│  │  Tool: get_balance                       │       │
│  └──────────────────────────────────────────┘       │
│                                                      │
│  ┌──────────────────────────────────────────┐       │
│  │  UserService                              │       │
│  │  - GenerateMCPKey(userID)                │       │
│  │  - GetMCPKeyStatus(userID)               │       │
│  │  - DeleteMCPKey(userID)                  │       │
│  │  - GetByMCPKey(key)                      │       │
│  └──────────────────────────────────────────┘       │
└──────────────────────┬──────────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────────┐
│                   PostgreSQL                         │
│  users.mcp_api_key                                  │
└─────────────────────────────────────────────────────┘
```

## 8. 文件变更清单

### Backend

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `ent/schema/user.go` | 修改 | 新增 `mcp_api_key` 字段 |
| `migrations/xxx_add_user_mcp_api_key.sql` | 新增 | 数据库迁移 |
| `internal/service/user_service.go` | 修改 | 新增 MCP key 相关方法 |
| `internal/repository/user_repo.go` | 修改 | 新增 `GetByMCPKey` 查询 |
| `internal/handler/user_handler.go` | 修改 | 新增 MCP key 管理 handler |
| `internal/server/middleware/mcp_auth.go` | 新增 | MCP 鉴权中间件 |
| `internal/server/middleware/wire.go` | 修改 | 注册 MCP 中间件类型 |
| `internal/server/mcp/server.go` | 新增 | MCP Server 实现 |
| `internal/server/mcp/tools.go` | 新增 | get_balance tool 实现 |
| `internal/server/router.go` | 修改 | 注册 `/mcp` 路由 |
| `internal/server/routes/user.go` | 修改 | 注册 MCP key 管理路由 |
| `internal/config/config.go` | 修改 | 新增 `MCPPublicURL` 配置项 |
| `config.yaml` | 修改 | 新增 `mcp_public_url` 配置 |

### Frontend

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `src/views/user/SettingsView.vue` | 修改 | 新增 MCP Key 管理区域 |
| `src/i18n/locales/zh.ts` | 修改 | 新增中文文案 |
| `src/i18n/locales/en.ts` | 修改 | 新增英文文案 |

## 9. 测试策略

- **单元测试**: `UserService` 的 `GenerateMCPKey`/`GetMCPKeyStatus`/`DeleteMCPKey`/`GetByMCPKey`
- **中间件测试**: `MCPAuthMiddleware` — 无 key、无效 key、有效 key、禁用用户
- **集成测试**: MCP server — 模拟客户端调用 `tools/list` 和 `tools/call`（`get_balance`）
