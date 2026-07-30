package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type accessKeyAuthRepoStub struct {
	user   *service.User
	byHash map[string]*service.User
}

func (r *accessKeyAuthRepoStub) GetByID(_ context.Context, _ int64) (*service.User, error) {
	return r.user, nil
}

func (r *accessKeyAuthRepoStub) GetByAccessKeyHash(_ context.Context, hash string) (*service.User, error) {
	if user := r.byHash[hash]; user != nil {
		return user, nil
	}
	return nil, service.ErrUserNotFound
}

func (r *accessKeyAuthRepoStub) CreateAccessKeyIfAbsent(_ context.Context, _ int64, hash, encrypted string, createdAt time.Time) (bool, error) {
	if r.user.UserAccessKeyHash != nil {
		return false, nil
	}
	r.user.UserAccessKeyHash = &hash
	r.user.UserAccessKeyEncrypted = &encrypted
	r.user.UserAccessKeyCreatedAt = &createdAt
	if r.byHash == nil {
		r.byHash = make(map[string]*service.User)
	}
	r.byHash[hash] = r.user
	return true, nil
}

type accessKeyAuthEncryptorStub struct{}

func (accessKeyAuthEncryptorStub) Encrypt(plain string) (string, error) {
	return "encrypted:" + plain, nil
}
func (accessKeyAuthEncryptorStub) Decrypt(cipher string) (string, error) {
	return strings.TrimPrefix(cipher, "encrypted:"), nil
}

func TestUserAccessKeyAuthMiddlewareAuthenticatesDedicatedHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	user := &service.User{ID: 9, Email: "developer@example.com", Concurrency: 3, Status: service.StatusActive}
	repo := &accessKeyAuthRepoStub{user: user}
	accessKeys := service.NewUserAccessKeyService(repo, accessKeyAuthEncryptorStub{}, &config.Config{
		Totp: config.TotpConfig{EncryptionKeyConfigured: true},
	})
	info, err := accessKeys.GetOrCreate(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("GetOrCreate() error = %v", err)
	}

	router := gin.New()
	router.GET("/api/v1/openapi/balance", gin.HandlerFunc(NewUserAccessKeyAuthMiddleware(accessKeys)), func(c *gin.Context) {
		subject, ok := GetAuthSubjectFromContext(c)
		if !ok || subject.UserID != user.ID {
			c.Status(http.StatusUnauthorized)
			return
		}
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/openapi/balance", nil)
	request.Header.Set(UserAccessKeyHeader, info.Key)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusNoContent, response.Body.String())
	}
}

func TestUserAccessKeyAuthMiddlewareRejectsMissingHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	accessKeys := service.NewUserAccessKeyService(&accessKeyAuthRepoStub{}, accessKeyAuthEncryptorStub{}, &config.Config{
		Totp: config.TotpConfig{EncryptionKeyConfigured: true},
	})
	router := gin.New()
	router.GET("/protected", gin.HandlerFunc(NewUserAccessKeyAuthMiddleware(accessKeys)), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/protected", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
	if !strings.Contains(response.Body.String(), "USER_ACCESS_KEY_REQUIRED") {
		t.Fatalf("unexpected response body: %s", response.Body.String())
	}
}
