package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func TestSetAuthenticatedUserContextAddsUserEmailToRequestLogger(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sink := initMiddlewareTestLogger(t)

	r := gin.New()
	r.Use(RequestLogger())
	r.GET("/gateway", func(c *gin.Context) {
		setAuthenticatedUserContext(c, &service.User{
			ID:          42,
			Email:       "alice@example.com",
			Role:        service.RoleUser,
			Concurrency: 3,
		})

		subject, ok := GetAuthSubjectFromContext(c)
		if !ok {
			t.Fatalf("auth subject missing")
		}
		if subject.Email != "alice@example.com" {
			t.Fatalf("subject email = %q, want alice@example.com", subject.Email)
		}
		if got, _ := c.Request.Context().Value(ctxkey.UserEmail).(string); got != "alice@example.com" {
			t.Fatalf("context user_email = %q, want alice@example.com", got)
		}

		logger.FromContext(c.Request.Context()).Info("downstream model call")
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/gateway", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}

	for _, event := range sink.list() {
		if event == nil || event.Message != "downstream model call" {
			continue
		}
		if event.Fields["user_email"] != "alice@example.com" {
			t.Fatalf("user_email field mismatch: %+v", event.Fields)
		}
		return
	}
	t.Fatalf("downstream model call log not found")
}
