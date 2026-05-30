package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestChannelMonitorUserHandlerDisabledReturnsEmptyList(t *testing.T) {
	gin.SetMode(gin.TestMode)
	settingService := service.NewSettingService(&channelMonitorHandlerSettingRepoStub{values: map[string]string{}}, &config.Config{})
	handler := NewChannelMonitorUserHandler(nil, settingService)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/channel-monitors", nil)

	handler.List(c)

	require.Equal(t, http.StatusOK, w.Code)
	require.JSONEq(t, `{"code":0,"message":"success","data":{"items":[]}}`, w.Body.String())
}

func TestChannelMonitorUserHandlerDisabledReturnsNotFoundForStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	settingService := service.NewSettingService(&channelMonitorHandlerSettingRepoStub{values: map[string]string{}}, &config.Config{})
	handler := NewChannelMonitorUserHandler(nil, settingService)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/channel-monitors/1/status", nil)

	handler.GetStatus(c)

	require.Equal(t, http.StatusNotFound, w.Code)
}

type channelMonitorHandlerSettingRepoStub struct {
	values map[string]string
}

func (s *channelMonitorHandlerSettingRepoStub) Get(context.Context, string) (*service.Setting, error) {
	panic("unexpected Get call")
}

func (s *channelMonitorHandlerSettingRepoStub) GetValue(context.Context, string) (string, error) {
	panic("unexpected GetValue call")
}

func (s *channelMonitorHandlerSettingRepoStub) Set(context.Context, string, string) error {
	panic("unexpected Set call")
}

func (s *channelMonitorHandlerSettingRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *channelMonitorHandlerSettingRepoStub) SetMultiple(context.Context, map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (s *channelMonitorHandlerSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *channelMonitorHandlerSettingRepoStub) Delete(context.Context, string) error {
	panic("unexpected Delete call")
}
