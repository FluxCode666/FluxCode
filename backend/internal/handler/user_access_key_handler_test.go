package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type userAccessKeyHandlerRepoStub struct {
	user   *service.User
	byHash map[string]*service.User
}

func (r *userAccessKeyHandlerRepoStub) GetByID(_ context.Context, _ int64) (*service.User, error) {
	return r.user, nil
}

func (r *userAccessKeyHandlerRepoStub) GetByAccessKeyHash(_ context.Context, hash string) (*service.User, error) {
	return r.byHash[hash], nil
}

func (r *userAccessKeyHandlerRepoStub) CreateAccessKeyIfAbsent(_ context.Context, _ int64, hash, encrypted string, createdAt time.Time) (bool, error) {
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

type userAccessKeyHandlerEncryptorStub struct{}

func (userAccessKeyHandlerEncryptorStub) Encrypt(plaintext string) (string, error) {
	return "encrypted:" + plaintext, nil
}

func (userAccessKeyHandlerEncryptorStub) Decrypt(ciphertext string) (string, error) {
	return strings.TrimPrefix(ciphertext, "encrypted:"), nil
}

func TestUserAccessKeyHandlerDisablesCachingForPlaintextKeyResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tc := range []struct {
		name      string
		method    string
		generated bool
	}{
		{name: "read existing key", method: http.MethodGet, generated: true},
		{name: "create key", method: http.MethodPost},
	} {
		t.Run(tc.name, func(t *testing.T) {
			user := &service.User{ID: 101, Status: service.StatusActive}
			repo := &userAccessKeyHandlerRepoStub{user: user}
			accessKeys := service.NewUserAccessKeyService(repo, userAccessKeyHandlerEncryptorStub{}, &config.Config{
				Totp: config.TotpConfig{EncryptionKeyConfigured: true},
			})
			if tc.generated {
				_, err := accessKeys.GetOrCreate(context.Background(), user.ID)
				require.NoError(t, err)
			}

			h := NewUserAccessKeyHandler(nil, accessKeys)
			router := gin.New()
			router.Handle(tc.method, "/api/v1/user/access-key", func(c *gin.Context) {
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: user.ID})
				c.Next()
			}, func(c *gin.Context) {
				if tc.method == http.MethodGet {
					h.GetAccessKey(c)
					return
				}
				h.CreateAccessKey(c)
			})

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(tc.method, "/api/v1/user/access-key", nil))

			require.Equal(t, http.StatusOK, recorder.Code)
			require.Equal(t, userAccessKeyCacheControl, recorder.Header().Get("Cache-Control"))
		})
	}
}

func TestUserAccessKeyHandlerRequiresExternallyConfiguredEncryptionKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	user := &service.User{ID: 101, Status: service.StatusActive}
	accessKeys := service.NewUserAccessKeyService(
		&userAccessKeyHandlerRepoStub{user: user},
		userAccessKeyHandlerEncryptorStub{},
		&config.Config{},
	)
	h := NewUserAccessKeyHandler(nil, accessKeys)
	router := gin.New()
	router.POST("/api/v1/user/access-key", func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: user.ID})
		h.CreateAccessKey(c)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/user/access-key", nil))

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Contains(t, recorder.Body.String(), "USER_ACCESS_KEY_ENCRYPTION_KEY_REQUIRED")
}
