package middleware

import (
	"context"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AuthSubject is the minimal authenticated identity stored in gin context.
// Decision: {UserID int64, Email string, Concurrency int}
type AuthSubject struct {
	UserID      int64
	Email       string
	Concurrency int
}

func GetAuthSubjectFromContext(c *gin.Context) (AuthSubject, bool) {
	value, exists := c.Get(string(ContextKeyUser))
	if !exists {
		return AuthSubject{}, false
	}
	subject, ok := value.(AuthSubject)
	return subject, ok
}

func GetUserRoleFromContext(c *gin.Context) (string, bool) {
	value, exists := c.Get(string(ContextKeyUserRole))
	if !exists {
		return "", false
	}
	role, ok := value.(string)
	return role, ok
}

func setAuthenticatedUserContext(c *gin.Context, user *service.User) {
	if c == nil || user == nil {
		return
	}

	email := strings.TrimSpace(user.Email)
	c.Set(string(ContextKeyUser), AuthSubject{
		UserID:      user.ID,
		Email:       email,
		Concurrency: user.Concurrency,
	})
	c.Set(string(ContextKeyUserRole), user.Role)

	if c.Request == nil {
		return
	}

	ctx := c.Request.Context()
	if email != "" {
		ctx = context.WithValue(ctx, ctxkey.UserEmail, email)
		ctx = logger.IntoContext(ctx, logger.FromContext(ctx).With(zap.String("user_email", email)))
	}
	c.Request = c.Request.WithContext(ctx)
}
