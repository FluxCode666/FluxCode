package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

const userAccessKeyCacheControl = "no-store, private"

// UserAccessKeyHandler exposes the signed-in self-service flow and the small
// user-scoped developer API surface. It intentionally does not expose any
// administrator or model-gateway functionality.
type UserAccessKeyHandler struct {
	userService      *service.UserService
	accessKeyService *service.UserAccessKeyService
}

func NewUserAccessKeyHandler(userService *service.UserService, accessKeyService *service.UserAccessKeyService) *UserAccessKeyHandler {
	return &UserAccessKeyHandler{
		userService:      userService,
		accessKeyService: accessKeyService,
	}
}

// GetAccessKey returns an already generated key to its authenticated owner.
// GET /api/v1/user/access-key
func (h *UserAccessKeyHandler) GetAccessKey(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	c.Header("Cache-Control", userAccessKeyCacheControl)

	info, err := h.accessKeyService.Get(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, info)
}

// CreateAccessKey creates a key on the first call and returns the existing key
// on subsequent calls. The operation is idempotent by design.
// POST /api/v1/user/access-key
func (h *UserAccessKeyHandler) CreateAccessKey(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	c.Header("Cache-Control", userAccessKeyCacheControl)

	info, err := h.accessKeyService.GetOrCreate(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, info)
}

// GetBalance is the user-key-authenticated balance endpoint documented for
// automation clients.
// GET /api/v1/openapi/balance
func (h *UserAccessKeyHandler) GetBalance(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	user, err := h.userService.GetByID(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{
		"user_id":  user.ID,
		"balance":  user.Balance,
		"currency": "USD",
	})
}
