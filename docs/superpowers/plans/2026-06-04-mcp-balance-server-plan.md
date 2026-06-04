# MCP 余额查询服务 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 FluxCode backend 中集成 MCP 余额查询服务，支持每用户 MCP 密钥鉴权。

**Architecture:** `users` 表新增 `mcp_api_key` 字段，新增 MCP 鉴权中间件和 MCP Server（`mark3labs/mcp-go` Streamable HTTP），前端个人设置页提供 MCP Key 管理 UI。

**Tech Stack:** Go 1.26, Gin v1.9.1, Ent ORM, mark3labs/mcp-go, Vue 3 + TypeScript

---

### Task 1: 添加 `mark3labs/mcp-go` 依赖

**Files:**
- Modify: `backend/go.mod`

- [ ] **Step 1: 添加依赖**

```bash
cd /Volumes/T7/project/new/FluxCode/backend && go get github.com/mark3labs/mcp-go@latest
```

- [ ] **Step 2: 验证编译**

```bash
cd /Volumes/T7/project/new/FluxCode/backend && go build ./...
```

Expected: 编译成功。

- [ ] **Step 3: Commit**

```bash
git add backend/go.mod backend/go.sum
git commit -m "deps: add mark3labs/mcp-go for MCP server"
```

---

### Task 2: Ent Schema — User 新增 `mcp_api_key` 字段

**Files:**
- Modify: `backend/ent/schema/user.go`

- [ ] **Step 1: 在 Fields() 末尾添加 mcp_api_key 字段**

在 `backend/ent/schema/user.go` 的 `Fields()` 方法中，`referral_code` 字段之后、`referred_by` 字段之前添加：

```go
		// MCP 服务密钥
		field.String("mcp_api_key").
			MaxLen(128).
			Optional().
			Nillable().
			Unique().
			Comment("MCP 服务 API Key，mcp- 前缀"),
```

- [ ] **Step 2: 运行 ent generate**

```bash
cd /Volumes/T7/project/new/FluxCode/backend && go generate ./ent
```

- [ ] **Step 3: 验证编译**

```bash
cd /Volumes/T7/project/new/FluxCode/backend && go build ./...
```

Expected: 编译成功。

- [ ] **Step 4: Commit**

```bash
git add backend/ent/
git commit -m "feat: add mcp_api_key field to User ent schema"
```

---

### Task 3: 数据库迁移

**Files:**
- Create: `backend/migrations/077_add_user_mcp_api_key.sql`

- [ ] **Step 1: 创建迁移文件**

```sql
-- Add mcp_api_key column to users table for MCP service authentication
ALTER TABLE users ADD COLUMN IF NOT EXISTS mcp_api_key VARCHAR(128);

-- Partial unique index for soft-delete support
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_mcp_api_key
    ON users (mcp_api_key)
    WHERE deleted_at IS NULL AND mcp_api_key IS NOT NULL;
```

- [ ] **Step 2: Commit**

```bash
git add backend/migrations/077_add_user_mcp_api_key.sql
git commit -m "feat: add migration for user mcp_api_key column"
```

---

### Task 4: User 模型新增 `MCPAPIKey` 字段

**Files:**
- Modify: `backend/internal/service/user.go`

- [ ] **Step 1: 在 User struct 中添加字段**

在 `Subscriptions []UserSubscription` 之后添加：

```go
	MCPAPIKey *string // MCP 服务 API Key（mcp- 前缀，nullable）
```

- [ ] **Step 2: Commit**

```bash
git add backend/internal/service/user.go
git commit -m "feat: add MCPAPIKey field to User model"
```

---

### Task 5: userEntityToService 映射 mcp_api_key

**Files:**
- Modify: `backend/internal/repository/api_key_repo.go`

- [ ] **Step 1: 在 userEntityToService 中添加映射**

在 `ReferredBy: u.ReferredBy,` 之后添加：

```go
		MCPAPIKey:                      u.MCPAPIKey,
```

- [ ] **Step 2: Commit**

```bash
git add backend/internal/repository/api_key_repo.go
git commit -m "feat: map mcp_api_key in userEntityToService"
```

---

### Task 6: UserRepository — 新增 MCP Key 方法

**Files:**
- Modify: `backend/internal/service/user_service.go` (UserRepository interface)
- Modify: `backend/internal/repository/user_repo.go`

- [ ] **Step 1: 在 UserRepository interface 中添加方法签名**

在 `ListActiveUserIDs` 之后添加：

```go
	// MCP Key
	GetByMCPKey(ctx context.Context, key string) (*User, error)
	SetMCPKey(ctx context.Context, userID int64, key string) error
	ClearMCPKey(ctx context.Context, userID int64) error
```

- [ ] **Step 2: 在 user_repo.go 末尾实现方法**

```go
// GetByMCPKey 根据 MCP API Key 查找用户
func (r *userRepository) GetByMCPKey(ctx context.Context, key string) (*service.User, error) {
	if key == "" {
		return nil, service.ErrUserNotFound
	}
	m, err := r.client.User.Query().
		Where(dbuser.MCPAPIKeyEQ(key), dbuser.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrUserNotFound, nil)
	}
	return userEntityToService(m), nil
}

// SetMCPKey 设置用户的 MCP API Key
func (r *userRepository) SetMCPKey(ctx context.Context, userID int64, key string) error {
	client := clientFromContext(ctx, r.client)
	n, err := client.User.UpdateOneID(userID).SetMCPAPIKey(key).Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrUserNotFound, nil)
	}
	if n == 0 {
		return service.ErrUserNotFound
	}
	return nil
}

// ClearMCPKey 清除用户的 MCP API Key
func (r *userRepository) ClearMCPKey(ctx context.Context, userID int64) error {
	client := clientFromContext(ctx, r.client)
	n, err := client.User.UpdateOneID(userID).ClearMCPAPIKey().Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrUserNotFound, nil)
	}
	if n == 0 {
		return service.ErrUserNotFound
	}
	return nil
}
```

- [ ] **Step 3: 验证编译**

```bash
cd /Volumes/T7/project/new/FluxCode/backend && go build ./...
```

Expected: 编译成功。

- [ ] **Step 4: Commit**

```bash
git add backend/internal/repository/user_repo.go backend/internal/service/user_service.go
git commit -m "feat: add MCP key repository methods"
```

---

### Task 7: UserService — 新增 MCP Key 业务方法

**Files:**
- Modify: `backend/internal/service/user_service.go`

- [ ] **Step 1: 添加 import**

在 import 块中添加：

```go
	"crypto/rand"
	"encoding/hex"
```

- [ ] **Step 2: 添加常量**

在 `notifyCodeUserRateWindow` 之后添加：

```go
	// MCPAPIKeyPrefix MCP API Key 前缀
	MCPAPIKeyPrefix = "mcp-"
```

- [ ] **Step 3: 在文件末尾添加方法**

```go
// GenerateMCPKey 为用户生成新的 MCP API Key
func (s *UserService) GenerateMCPKey(ctx context.Context, userID int64) (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}
	key := MCPAPIKeyPrefix + hex.EncodeToString(bytes)
	if err := s.userRepo.SetMCPKey(ctx, userID, key); err != nil {
		return "", fmt.Errorf("save mcp key: %w", err)
	}
	return key, nil
}

// GetMCPKeyStatus 获取用户 MCP Key 状态（脱敏）
func (s *UserService) GetMCPKeyStatus(ctx context.Context, userID int64) (maskedKey string, exists bool, err error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return "", false, fmt.Errorf("get user: %w", err)
	}
	if user.MCPAPIKey == nil || *user.MCPAPIKey == "" {
		return "", false, nil
	}
	key := *user.MCPAPIKey
	if len(key) > 14 {
		maskedKey = key[:10] + "..." + key[len(key)-4:]
	} else {
		maskedKey = key
	}
	return maskedKey, true, nil
}

// DeleteMCPKey 删除用户的 MCP API Key
func (s *UserService) DeleteMCPKey(ctx context.Context, userID int64) error {
	return s.userRepo.ClearMCPKey(ctx, userID)
}

// GetByMCPKey 根据 MCP API Key 获取用户（供中间件使用）
func (s *UserService) GetByMCPKey(ctx context.Context, key string) (*User, error) {
	return s.userRepo.GetByMCPKey(ctx, key)
}
```

- [ ] **Step 4: 验证编译**

```bash
cd /Volumes/T7/project/new/FluxCode/backend && go build ./...
```

Expected: 编译成功。

- [ ] **Step 5: Commit**

```bash
git add backend/internal/service/user_service.go
git commit -m "feat: add MCP key service methods"
```

---

### Task 8: UserHandler — 新增 MCP Key 管理 Handler

**Files:**
- Modify: `backend/internal/handler/user_handler.go`

- [ ] **Step 1: 在文件末尾添加 handler 方法**

```go
// GetMCPKey 获取 MCP Key 状态
// GET /api/v1/user/mcp-key
func (h *UserHandler) GetMCPKey(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	maskedKey, exists, err := h.userService.GetMCPKeyStatus(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{
		"exists":     exists,
		"masked_key": maskedKey,
	})
}

// RegenerateMCPKey 生成/重新生成 MCP Key
// POST /api/v1/user/mcp-key/regenerate
func (h *UserHandler) RegenerateMCPKey(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	key, err := h.userService.GenerateMCPKey(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{
		"key": key,
	})
}

// DeleteMCPKey 删除 MCP Key
// DELETE /api/v1/user/mcp-key
func (h *UserHandler) DeleteMCPKey(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	if err := h.userService.DeleteMCPKey(c.Request.Context(), subject.UserID); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{"message": "MCP key deleted"})
}
```

- [ ] **Step 2: 验证编译**

```bash
cd /Volumes/T7/project/new/FluxCode/backend && go build ./...
```

Expected: 编译成功。

- [ ] **Step 3: Commit**

```bash
git add backend/internal/handler/user_handler.go
git commit -m "feat: add MCP key management handlers"
```

---

### Task 9: 注册 MCP Key 管理路由

**Files:**
- Modify: `backend/internal/server/routes/user.go`

- [ ] **Step 1: 在 user group 内添加 MCP key 路由**

在 `user` group 的 `}` 之前添加：

```go
			// MCP Key 管理
			mcpKey := user.Group("/mcp-key")
			{
				mcpKey.GET("", h.User.GetMCPKey)
				mcpKey.POST("/regenerate", h.User.RegenerateMCPKey)
				mcpKey.DELETE("", h.User.DeleteMCPKey)
			}
```

- [ ] **Step 2: 验证编译**

```bash
cd /Volumes/T7/project/new/FluxCode/backend && go build ./...
```

Expected: 编译成功。

- [ ] **Step 3: Commit**

```bash
git add backend/internal/server/routes/user.go
git commit -m "feat: register MCP key management routes"
```

---

### Task 10: MCP 鉴权中间件

**Files:**
- Create: `backend/internal/server/middleware/mcp_auth.go`
- Modify: `backend/internal/server/middleware/wire.go`

- [ ] **Step 1: 创建 mcp_auth.go**

```go
package middleware

import (
	"crypto/subtle"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// NewMCPAuthMiddleware 创建 MCP 认证中间件
func NewMCPAuthMiddleware(userService *service.UserService) MCPAuthMiddleware {
	return MCPAuthMiddleware(mcpAuth(userService))
}

func mcpAuth(userService *service.UserService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var key string

		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
				key = strings.TrimSpace(parts[1])
			}
		}

		if key == "" {
			key = strings.TrimSpace(c.GetHeader("x-api-key"))
		}

		if key == "" {
			AbortWithError(c, 401, "MCP_KEY_REQUIRED", "MCP API key is required")
			return
		}

		user, err := userService.GetByMCPKey(c.Request.Context(), key)
		if err != nil {
			AbortWithError(c, 401, "INVALID_MCP_KEY", "Invalid MCP API key")
			return
		}

		if !user.IsActive() {
			AbortWithError(c, 401, "USER_INACTIVE", "User account is not active")
			return
		}

		if user.MCPAPIKey == nil || subtle.ConstantTimeCompare([]byte(key), []byte(*user.MCPAPIKey)) != 1 {
			AbortWithError(c, 401, "INVALID_MCP_KEY", "Invalid MCP API key")
			return
		}

		setAuthenticatedUserContext(c, user)
		c.Set("auth_method", "mcp_api_key")
		c.Next()
	}
}
```

- [ ] **Step 2: 在 wire.go 中注册类型**

在 `APIKeyAuthMiddleware` 之后添加：

```go
// MCPAuthMiddleware MCP 认证中间件类型
type MCPAuthMiddleware gin.HandlerFunc
```

在 `ProviderSet` 中添加：

```go
	NewMCPAuthMiddleware,
```

- [ ] **Step 3: 验证编译**

```bash
cd /Volumes/T7/project/new/FluxCode/backend && go build ./...
```

Expected: 编译成功。

- [ ] **Step 4: Commit**

```bash
git add backend/internal/server/middleware/mcp_auth.go backend/internal/server/middleware/wire.go
git commit -m "feat: add MCP auth middleware"
```

---

### Task 11: MCP Server 实现

**Files:**
- Create: `backend/internal/server/mcp/server.go`

- [ ] **Step 1: 创建 server.go**

```go
package mcp

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

type contextKey string

const mcpUserKey contextKey = "mcp_user"

// MCPServer wraps the MCP server.
type MCPServer struct {
	server      *mcpserver.MCPServer
	userService *service.UserService
}

// NewMCPServer 创建 MCP Server 实例
func NewMCPServer(userService *service.UserService) *MCPServer {
	s := &MCPServer{userService: userService}

	s.server = mcpserver.NewMCPServer(
		"FluxCode",
		"1.0.0",
		mcpserver.WithToolCapabilities(true),
	)

	getBalanceTool := mcp.NewTool(
		"get_balance",
		mcp.WithDescription("查询当前账户余额及用量概览，包括用户名、邮箱、余额和累计充值金额"),
	)
	s.server.AddTool(getBalanceTool, s.handleGetBalance)

	return s
}

func (s *MCPServer) handleGetBalance(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	user, ok := ctx.Value(mcpUserKey).(*service.User)
	if !ok || user == nil {
		return mcp.NewToolResultError("User not authenticated"), nil
	}

	latestUser, err := s.userService.GetByID(ctx, user.ID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get user: %v", err)), nil
	}

	result := fmt.Sprintf(`账户余额信息：
- 用户名：%s
- 邮箱：%s
- 当前余额：%.2f USD
- 累计充值：%.2f USD`,
		latestUser.Username,
		latestUser.Email,
		latestUser.Balance,
		latestUser.TotalRecharged,
	)

	return mcp.NewToolResultText(result), nil
}

// GinHandler 返回 Gin handler，处理 MCP JSON-RPC 请求
func (s *MCPServer) GinHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		subject, ok := middleware.GetAuthSubjectFromContext(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		user, err := s.userService.GetByID(c.Request.Context(), subject.UserID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get user"})
			return
		}

		ctx := context.WithValue(c.Request.Context(), mcpUserKey, user)
		c.Request = c.Request.WithContext(ctx)

		streamableHandler := mcpserver.NewStreamableHTTPServer(s.server)
		streamableHandler.ServeHTTP(c.Writer, c.Request)
	}
}
```

- [ ] **Step 2: 验证编译**

```bash
cd /Volumes/T7/project/new/FluxCode/backend && go build ./...
```

Expected: 编译成功。

- [ ] **Step 3: Commit**

```bash
git add backend/internal/server/mcp/
git commit -m "feat: implement MCP server with get_balance tool"
```

---

### Task 12: 注册 MCP 路由到 Router

**Files:**
- Modify: `backend/internal/server/router.go`
- Modify: `backend/internal/server/http.go`

- [ ] **Step 1: 在 router.go 的 SetupRouter 签名中添加 mcpAuth 参数**

在 `apiKeyAuth middleware2.APIKeyAuthMiddleware,` 之后添加：

```go
	mcpAuth middleware2.MCPAuthMiddleware,
```

- [ ] **Step 2: 在 registerRoutes 签名中添加 mcpAuth 参数**

同上位置添加 `mcpAuth middleware2.MCPAuthMiddleware,`。

- [ ] **Step 3: 在 registerRoutes 末尾注册 MCP 路由**

在 `routes.RegisterPaymentRoutes(...)` 之后添加：

```go
	// MCP 服务路由
	mcpServer := mcp2.NewMCPServer(apiKeyService.UserService()) // 需要调整
	r.POST("/mcp", gin.HandlerFunc(mcpAuth), mcpServer.GinHandler())
```

**注意**: `apiKeyService` 没有直接暴露 `userService`，需要通过其他方式获取。检查 `apiKeyService` 是否有 `userService` 字段，或者通过 setup 层直接注入 `userService`。

- [ ] **Step 4: 在 http.go 的 ProvideRouter 中添加 mcpAuth 参数**

在 `apiKeyAuth middleware2.APIKeyAuthMiddleware,` 之后添加：

```go
	mcpAuth middleware2.MCPAuthMiddleware,
```

并在 `SetupRouter` 调用中传入 `mcpAuth`。

- [ ] **Step 5: 验证编译并重新生成 wire**

```bash
cd /Volumes/T7/project/new/FluxCode/backend/cmd/server && go generate ./...
cd /Volumes/T7/project/new/FluxCode/backend && go build ./...
```

Expected: 编译成功。

- [ ] **Step 6: Commit**

```bash
git add backend/internal/server/router.go backend/internal/server/http.go backend/cmd/server/wire_gen.go
git commit -m "feat: register MCP routes in router"
```

---

### Task 13: 配置项 — 添加 `mcp_public_url`

**Files:**
- Modify: `backend/internal/config/config.go`
- Modify: `deploy/backend/docker-compose.yml`

- [ ] **Step 1: 在 ServerConfig 中添加字段**

在 `H2C H2CConfig` 之后添加：

```go
	MCPPublicURL string `mapstructure:"mcp_public_url"` // MCP 服务公开 URL
```

- [ ] **Step 2: 在 docker-compose.yml 中添加环境变量**

在 `ANTIGRAVITY_OAUTH_CLIENT_SECRET` 之后添加：

```yaml
      - SERVER_MCP_PUBLIC_URL=${MCP_PUBLIC_URL:-}
```

- [ ] **Step 3: Commit**

```bash
git add backend/internal/config/config.go deploy/backend/docker-compose.yml
git commit -m "feat: add mcp_public_url config"
```

---

### Task 14: 前端 — MCP Key 管理 UI

**Files:**
- Modify: 前端设置页面组件
- Modify: i18n locale 文件

- [ ] **Step 1: 定位设置页面**

```bash
find /Volumes/T7/project/new/FluxCode/frontend/src -name "*Setting*" -o -name "*setting*" | head -10
```

- [ ] **Step 2: 添加 MCP Key 管理区域**

在 API Key 管理区域附近添加卡片，包含：
- 当前状态（脱敏 key + 复制按钮）
- 生成/重新生成按钮（确认弹窗）
- 删除按钮（确认弹窗）
- MCP 配置 JSON 展示 + 复制按钮

- [ ] **Step 3: 添加 API 调用**

```typescript
// GET /api/v1/user/mcp-key
async function fetchMCPKeyStatus(): Promise<{ exists: boolean; masked_key: string }>

// POST /api/v1/user/mcp-key/regenerate
async function regenerateMCPKey(): Promise<{ key: string }>

// DELETE /api/v1/user/mcp-key
async function deleteMCPKey(): Promise<void>
```

- [ ] **Step 4: 添加 i18n 文案**

在 `zh.ts` 和 `en.ts` 中添加 MCP Key 相关翻译。

- [ ] **Step 5: Commit**

```bash
git add frontend/src/
git commit -m "feat: add MCP key management UI"
```

---

### Task 15: 测试

**Files:**
- Create: `backend/internal/server/middleware/mcp_auth_test.go`
- Create: `backend/internal/server/mcp/server_test.go`

- [ ] **Step 1: 中间件测试**

```go
package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

type mockUserRepo struct{}

func (m *mockUserRepo) GetByMCPKey(ctx context.Context, key string) (*service.User, error) {
	if key == "mcp-valid" {
		k := "mcp-valid"
		return &service.User{ID: 1, Status: service.StatusActive, MCPAPIKey: &k}, nil
	}
	return nil, service.ErrUserNotFound
}

// ... 其他 mock 方法 stub ...

func TestMCPAuth_NoKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// 构造 mock userService 并测试
	// ...
}

func TestMCPAuth_InvalidKey(t *testing.T) { /* ... */ }
func TestMCPAuth_ValidKey(t *testing.T) { /* ... */ }
func TestMCPAuth_InactiveUser(t *testing.T) { /* ... */ }
```

- [ ] **Step 2: 运行测试**

```bash
cd /Volumes/T7/project/new/FluxCode/backend && go test ./internal/server/middleware/ -v -run TestMCPAuth
```

- [ ] **Step 3: Commit**

```bash
git add backend/internal/server/middleware/mcp_auth_test.go
git commit -m "test: add MCP auth middleware tests"
```
