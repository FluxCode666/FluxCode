package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type stubSalesCommissionHandlerUserRepo struct {
	getByIDFn func(ctx context.Context, id int64) (*service.User, error)
}

func (s stubSalesCommissionHandlerUserRepo) GetByID(ctx context.Context, id int64) (*service.User, error) {
	if s.getByIDFn == nil {
		return nil, nil
	}
	return s.getByIDFn(ctx, id)
}

func TestSalesCommissionHandlerRequiresAuthenticatedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewSalesCommissionHandler(&service.SalesCommissionService{})
	r := gin.New()
	r.GET("/sales-commissions/summary", h.GetSummary)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/sales-commissions/summary", nil))

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSalesCommissionHandlerRequiresSalesUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := service.NewSalesCommissionService(nil, nil, stubSalesCommissionHandlerUserRepo{
		getByIDFn: func(ctx context.Context, id int64) (*service.User, error) {
			return &service.User{ID: id, IsSales: false}, nil
		},
	})
	h := NewSalesCommissionHandler(svc)
	r := gin.New()
	r.GET("/sales-commissions/summary", func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 12})
		h.GetSummary(c)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/sales-commissions/summary", nil))

	require.Equal(t, http.StatusForbidden, w.Code)
}
