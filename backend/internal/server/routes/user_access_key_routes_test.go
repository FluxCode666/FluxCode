package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	handlerpkg "github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUserAccessKeyDeveloperRoutesAreRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")

	RegisterUserRoutes(
		v1,
		&handlerpkg.Handlers{
			User:          handlerpkg.NewUserHandler(nil, nil, nil, nil),
			UserAccessKey: handlerpkg.NewUserAccessKeyHandler(nil, nil),
			APIKey:        handlerpkg.NewAPIKeyHandler(nil),
			Usage:         handlerpkg.NewUsageHandler(nil, nil),
		},
		middleware.JWTAuthMiddleware(func(c *gin.Context) { c.Next() }),
		nil,
		middleware.UserAccessKeyAuthMiddleware(func(c *gin.Context) { c.Next() }),
	)

	for _, route := range []struct {
		method string
		path   string
	}{
		{method: "GET", path: "/api/v1/user/access-key"},
		{method: "POST", path: "/api/v1/user/access-key"},
		{method: "GET", path: "/api/v1/openapi/balance"},
		{method: "GET", path: "/api/v1/openapi/keys"},
		{method: "POST", path: "/api/v1/openapi/keys"},
		{method: "GET", path: "/api/v1/openapi/keys/:id"},
		{method: "PUT", path: "/api/v1/openapi/keys/:id"},
		{method: "DELETE", path: "/api/v1/openapi/keys/:id"},
		{method: "GET", path: "/api/v1/openapi/groups/available"},
		{method: "GET", path: "/api/v1/openapi/usage"},
		{method: "GET", path: "/api/v1/openapi/usage/stats"},
		{method: "GET", path: "/api/v1/openapi/usage/:id"},
	} {
		requireRegisteredRoute(t, router, route.method, route.path)
	}
}

type developerUsageRepoCapture struct {
	service.UsageLogRepository
	listParams  pagination.PaginationParams
	listFilters usagestats.UsageLogFilters
	getRecord   *service.UsageLog
}

type developerAPIKeyRepoStub struct {
	service.APIKeyRepository
	apiKey *service.APIKey
}

func (r *developerAPIKeyRepoStub) GetByID(_ context.Context, _ int64) (*service.APIKey, error) {
	return r.apiKey, nil
}

func (r *developerUsageRepoCapture) ListWithFilters(_ context.Context, params pagination.PaginationParams, filters usagestats.UsageLogFilters) ([]service.UsageLog, *pagination.PaginationResult, error) {
	r.listParams = params
	r.listFilters = filters
	return []service.UsageLog{}, &pagination.PaginationResult{
		Total:    0,
		Page:     params.Page,
		PageSize: params.PageSize,
		Pages:    0,
	}, nil
}

func (r *developerUsageRepoCapture) GetByID(_ context.Context, _ int64) (*service.UsageLog, error) {
	return r.getRecord, nil
}

func newUserAccessKeyUsageRouter(repo *developerUsageRepoCapture, apiKeyService *service.APIKeyService, auth middleware.UserAccessKeyAuthMiddleware) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	usageService := service.NewUsageService(repo, nil, nil, nil)

	RegisterUserRoutes(
		v1,
		&handlerpkg.Handlers{
			User:          handlerpkg.NewUserHandler(nil, nil, nil, nil),
			UserAccessKey: handlerpkg.NewUserAccessKeyHandler(nil, nil),
			APIKey:        handlerpkg.NewAPIKeyHandler(nil),
			Usage:         handlerpkg.NewUsageHandler(usageService, apiKeyService),
		},
		middleware.JWTAuthMiddleware(func(c *gin.Context) { c.Next() }),
		nil,
		auth,
	)

	return router
}

func authenticatedDeveloperUsageMiddleware(userID int64) middleware.UserAccessKeyAuthMiddleware {
	return middleware.UserAccessKeyAuthMiddleware(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: userID})
		c.Next()
	})
}

func TestUserAccessKeyDeveloperUsageListUsesCurrentUserAndExistingFilters(t *testing.T) {
	repo := &developerUsageRepoCapture{}
	router := newUserAccessKeyUsageRouter(repo, nil, authenticatedDeveloperUsageMiddleware(84))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/openapi/usage?page=2&page_size=25&model=gpt-4.1&request_type=stream&billing_type=1&start_date=2026-07-01&end_date=2026-07-02&timezone=UTC&sort_by=model&sort_order=asc", nil)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	require.Equal(t, int64(84), repo.listFilters.UserID)
	require.Equal(t, "gpt-4.1", repo.listFilters.Model)
	require.NotNil(t, repo.listFilters.RequestType)
	require.Equal(t, int16(service.RequestTypeStream), *repo.listFilters.RequestType)
	require.Nil(t, repo.listFilters.Stream)
	require.NotNil(t, repo.listFilters.BillingType)
	require.Equal(t, int8(1), *repo.listFilters.BillingType)
	require.NotNil(t, repo.listFilters.StartTime)
	require.NotNil(t, repo.listFilters.EndTime)
	require.Equal(t, "2026-07-01T00:00:00Z", repo.listFilters.StartTime.UTC().Format(time.RFC3339))
	require.Equal(t, "2026-07-03T00:00:00Z", repo.listFilters.EndTime.UTC().Format(time.RFC3339))
	require.Equal(t, pagination.PaginationParams{Page: 2, PageSize: 25, SortBy: "model", SortOrder: "asc"}, repo.listParams)
}

func TestUserAccessKeyDeveloperUsageRecordRejectsAnotherUsersRecord(t *testing.T) {
	repo := &developerUsageRepoCapture{getRecord: &service.UsageLog{ID: 901, UserID: 85}}
	router := newUserAccessKeyUsageRouter(repo, nil, authenticatedDeveloperUsageMiddleware(84))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/openapi/usage/901", nil)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	require.Equal(t, http.StatusForbidden, res.Code, res.Body.String())
	require.Contains(t, res.Body.String(), "Not authorized to access this record")
}

func TestUserAccessKeyDeveloperUsageListRejectsAnotherUsersAPIKeyFilter(t *testing.T) {
	repo := &developerUsageRepoCapture{}
	apiKeyService := service.NewAPIKeyService(
		&developerAPIKeyRepoStub{apiKey: &service.APIKey{ID: 604, UserID: 85}},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	router := newUserAccessKeyUsageRouter(repo, apiKeyService, authenticatedDeveloperUsageMiddleware(84))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/openapi/usage?api_key_id=604", nil)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	require.Equal(t, http.StatusForbidden, res.Code, res.Body.String())
	require.Contains(t, res.Body.String(), "Not authorized to access this API key's usage records")
	require.Zero(t, repo.listFilters.UserID)
}

func TestUserAccessKeyDeveloperUsageRequiresDeveloperAuthentication(t *testing.T) {
	repo := &developerUsageRepoCapture{}
	router := newUserAccessKeyUsageRouter(repo, nil, middleware.UserAccessKeyAuthMiddleware(func(c *gin.Context) {
		c.AbortWithStatus(http.StatusUnauthorized)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/openapi/usage", nil)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	require.Equal(t, http.StatusUnauthorized, res.Code)
	require.Zero(t, repo.listFilters.UserID)
}
