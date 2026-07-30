package middleware

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// UserAccessKeyHeader is purposefully separate from Authorization. JWT bearer
// tokens retain their existing meaning and cannot accidentally be sent to a
// user-key endpoint (or vice versa).
const UserAccessKeyHeader = "X-User-Access-Key"

// NewUserAccessKeyAuthMiddleware authenticates only the user-scoped developer API.
func NewUserAccessKeyAuthMiddleware(accessKeyService *service.UserAccessKeyService) UserAccessKeyAuthMiddleware {
	return UserAccessKeyAuthMiddleware(func(c *gin.Context) {
		rawKey := strings.TrimSpace(c.GetHeader(UserAccessKeyHeader))
		if rawKey == "" {
			AbortWithError(c, 401, "USER_ACCESS_KEY_REQUIRED", UserAccessKeyHeader+" header is required")
			return
		}

		user, err := accessKeyService.Validate(c.Request.Context(), rawKey)
		if err != nil {
			AbortWithError(c, 401, "INVALID_USER_ACCESS_KEY", "invalid user access key")
			return
		}

		setAuthenticatedUserContext(c, user)
		c.Next()
	})
}
