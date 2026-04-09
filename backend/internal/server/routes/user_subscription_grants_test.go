package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	handlerpkg "github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type subscriptionGrantsRoutesUserSubRepoStub struct {
	sub *service.UserSubscription
}

func (s *subscriptionGrantsRoutesUserSubRepoStub) Create(context.Context, *service.UserSubscription) error {
	panic("unexpected Create call")
}

func (s *subscriptionGrantsRoutesUserSubRepoStub) GetByID(context.Context, int64) (*service.UserSubscription, error) {
	if s.sub == nil {
		return nil, service.ErrSubscriptionNotFound
	}
	cp := *s.sub
	return &cp, nil
}

func (s *subscriptionGrantsRoutesUserSubRepoStub) GetByUserIDAndGroupID(context.Context, int64, int64) (*service.UserSubscription, error) {
	panic("unexpected GetByUserIDAndGroupID call")
}

func (s *subscriptionGrantsRoutesUserSubRepoStub) GetActiveByUserIDAndGroupID(context.Context, int64, int64) (*service.UserSubscription, error) {
	panic("unexpected GetActiveByUserIDAndGroupID call")
}

func (s *subscriptionGrantsRoutesUserSubRepoStub) Update(context.Context, *service.UserSubscription) error {
	panic("unexpected Update call")
}

func (s *subscriptionGrantsRoutesUserSubRepoStub) Delete(context.Context, int64) error {
	panic("unexpected Delete call")
}

func (s *subscriptionGrantsRoutesUserSubRepoStub) ListByUserID(context.Context, int64) ([]service.UserSubscription, error) {
	panic("unexpected ListByUserID call")
}

func (s *subscriptionGrantsRoutesUserSubRepoStub) ListActiveByUserID(context.Context, int64) ([]service.UserSubscription, error) {
	panic("unexpected ListActiveByUserID call")
}

func (s *subscriptionGrantsRoutesUserSubRepoStub) ListByGroupID(context.Context, int64, pagination.PaginationParams) ([]service.UserSubscription, *pagination.PaginationResult, error) {
	panic("unexpected ListByGroupID call")
}

func (s *subscriptionGrantsRoutesUserSubRepoStub) List(context.Context, pagination.PaginationParams, *int64, *int64, string, string, string, string) ([]service.UserSubscription, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}

func (s *subscriptionGrantsRoutesUserSubRepoStub) ExistsByUserIDAndGroupID(context.Context, int64, int64) (bool, error) {
	panic("unexpected ExistsByUserIDAndGroupID call")
}

func (s *subscriptionGrantsRoutesUserSubRepoStub) ExtendExpiry(context.Context, int64, time.Time) error {
	panic("unexpected ExtendExpiry call")
}

func (s *subscriptionGrantsRoutesUserSubRepoStub) UpdateStatus(context.Context, int64, string) error {
	panic("unexpected UpdateStatus call")
}

func (s *subscriptionGrantsRoutesUserSubRepoStub) UpdateNotes(context.Context, int64, string) error {
	panic("unexpected UpdateNotes call")
}

func (s *subscriptionGrantsRoutesUserSubRepoStub) ActivateWindows(context.Context, int64, time.Time) error {
	panic("unexpected ActivateWindows call")
}

func (s *subscriptionGrantsRoutesUserSubRepoStub) ResetDailyUsage(context.Context, int64, time.Time) error {
	panic("unexpected ResetDailyUsage call")
}

func (s *subscriptionGrantsRoutesUserSubRepoStub) ResetWeeklyUsage(context.Context, int64, time.Time) error {
	panic("unexpected ResetWeeklyUsage call")
}

func (s *subscriptionGrantsRoutesUserSubRepoStub) ResetMonthlyUsage(context.Context, int64, time.Time) error {
	panic("unexpected ResetMonthlyUsage call")
}

func (s *subscriptionGrantsRoutesUserSubRepoStub) IncrementUsage(context.Context, int64, float64) error {
	panic("unexpected IncrementUsage call")
}

func (s *subscriptionGrantsRoutesUserSubRepoStub) BatchUpdateExpiredStatus(context.Context) (int64, error) {
	panic("unexpected BatchUpdateExpiredStatus call")
}

type subscriptionGrantsRoutesGrantRepoStub struct{}

func (s *subscriptionGrantsRoutesGrantRepoStub) Create(context.Context, *service.SubscriptionGrant) error {
	panic("unexpected Create call")
}

func (s *subscriptionGrantsRoutesGrantRepoStub) CountActiveBySubscriptionID(context.Context, int64, time.Time) (int, error) {
	panic("unexpected CountActiveBySubscriptionID call")
}

func (s *subscriptionGrantsRoutesGrantRepoStub) ListActiveBySubscriptionID(context.Context, int64, time.Time) ([]*service.SubscriptionGrant, error) {
	panic("unexpected ListActiveBySubscriptionID call")
}

func (s *subscriptionGrantsRoutesGrantRepoStub) ListUnexpiredBySubscriptionID(context.Context, int64, time.Time) ([]*service.SubscriptionGrant, error) {
	return []*service.SubscriptionGrant{}, nil
}

func (s *subscriptionGrantsRoutesGrantRepoStub) SumActiveUsageBySubscriptionID(context.Context, int64, time.Time) (float64, float64, float64, error) {
	panic("unexpected SumActiveUsageBySubscriptionID call")
}

func (s *subscriptionGrantsRoutesGrantRepoStub) MinActiveExpiresAtBySubscriptionID(context.Context, int64, time.Time) (*time.Time, error) {
	panic("unexpected MinActiveExpiresAtBySubscriptionID call")
}

func (s *subscriptionGrantsRoutesGrantRepoStub) ListActiveForUpdate(context.Context, int64, time.Time) ([]*service.SubscriptionGrant, error) {
	panic("unexpected ListActiveForUpdate call")
}

func (s *subscriptionGrantsRoutesGrantRepoStub) GetTailGrantBySubscriptionID(context.Context, int64) (*service.SubscriptionGrant, error) {
	panic("unexpected GetTailGrantBySubscriptionID call")
}

func (s *subscriptionGrantsRoutesGrantRepoStub) UpdateExpiresAt(context.Context, int64, time.Time) error {
	panic("unexpected UpdateExpiresAt call")
}

func (s *subscriptionGrantsRoutesGrantRepoStub) ResetDailyUsageBySubscriptionID(context.Context, int64) error {
	panic("unexpected ResetDailyUsageBySubscriptionID call")
}

func (s *subscriptionGrantsRoutesGrantRepoStub) ResetWeeklyUsageBySubscriptionID(context.Context, int64) error {
	panic("unexpected ResetWeeklyUsageBySubscriptionID call")
}

func (s *subscriptionGrantsRoutesGrantRepoStub) ResetMonthlyUsageBySubscriptionID(context.Context, int64) error {
	panic("unexpected ResetMonthlyUsageBySubscriptionID call")
}

func (s *subscriptionGrantsRoutesGrantRepoStub) AllocateUsageToActiveGrants(context.Context, int64, *service.Group, time.Time, float64) error {
	panic("unexpected AllocateUsageToActiveGrants call")
}

func TestUserSubscriptionGrantsRoute_UnauthorizedWhenNotLoggedIn(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")

	jwtAuth := middleware.JWTAuthMiddleware(func(c *gin.Context) { c.Next() })
	RegisterUserRoutes(v1, &handlerpkg.Handlers{Subscription: handlerpkg.NewSubscriptionHandler(nil)}, jwtAuth, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions/1/grants", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestUserSubscriptionGrantsRoute_OKForOwnedSubscription(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")

	userSubRepo := &subscriptionGrantsRoutesUserSubRepoStub{
		sub: &service.UserSubscription{
			ID:     1,
			UserID: 10,
			Group:  &service.Group{ID: 20, Name: "Group One"},
		},
	}
	subSvc := service.NewSubscriptionService(nil, userSubRepo, &subscriptionGrantsRoutesGrantRepoStub{}, nil, nil, nil)

	jwtAuth := middleware.JWTAuthMiddleware(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 10})
		c.Set(string(middleware.ContextKeyUserRole), "user")
		c.Next()
	})
	RegisterUserRoutes(v1, &handlerpkg.Handlers{Subscription: handlerpkg.NewSubscriptionHandler(subSvc)}, jwtAuth, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions/1/grants", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"subscription_id":1`)
}

func TestUserSubscriptionGrantsRoute_NotFoundForOtherUsersSubscription(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")

	userSubRepo := &subscriptionGrantsRoutesUserSubRepoStub{
		sub: &service.UserSubscription{
			ID:     1,
			UserID: 10,
			Group:  &service.Group{ID: 20, Name: "Group One"},
		},
	}
	subSvc := service.NewSubscriptionService(nil, userSubRepo, &subscriptionGrantsRoutesGrantRepoStub{}, nil, nil, nil)

	jwtAuth := middleware.JWTAuthMiddleware(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 11})
		c.Set(string(middleware.ContextKeyUserRole), "user")
		c.Next()
	})
	RegisterUserRoutes(v1, &handlerpkg.Handlers{Subscription: handlerpkg.NewSubscriptionHandler(subSvc)}, jwtAuth, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions/1/grants", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}
