package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type mediaAccountConnectionTestRepo struct {
	AccountRepository
	account *Account
}

func (r *mediaAccountConnectionTestRepo) GetByID(context.Context, int64) (*Account, error) {
	return r.account, nil
}

func TestAccountTestServiceRejectsMediaAccountBeforeTextFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/1/test", nil)

	service := &AccountTestService{accountRepo: &mediaAccountConnectionTestRepo{account: &Account{
		ID: 1, Platform: PlatformMedia, Type: AccountTypeAPIKey,
	}}}
	err := service.TestAccountConnection(c, 1, "seedance", "test")

	require.EqualError(t, err, "Media account connection testing is not supported yet")
	require.Contains(t, recorder.Body.String(), "Media account connection testing is not supported yet")
}
