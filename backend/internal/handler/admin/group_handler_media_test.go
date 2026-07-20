package admin

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	handlerdto "github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type mediaGroupAdminService struct {
	*stubAdminService
	created *service.CreateGroupInput
	updated *service.UpdateGroupInput
}

func (s *mediaGroupAdminService) CreateGroup(_ context.Context, input *service.CreateGroupInput) (*service.Group, error) {
	s.created = input
	return &service.Group{
		ID:                        10,
		Name:                      input.Name,
		Platform:                  input.Platform,
		AllowImageGeneration:      input.AllowImageGeneration,
		AllowVideoGeneration:      input.AllowVideoGeneration,
		MediaCrossPlatformEnabled: input.MediaCrossPlatformEnabled,
	}, nil
}

func (s *mediaGroupAdminService) UpdateGroup(_ context.Context, id int64, input *service.UpdateGroupInput) (*service.Group, error) {
	s.updated = input
	group := &service.Group{ID: id, Name: input.Name}
	if input.AllowImageGeneration != nil {
		group.AllowImageGeneration = *input.AllowImageGeneration
	}
	if input.AllowVideoGeneration != nil {
		group.AllowVideoGeneration = *input.AllowVideoGeneration
	}
	if input.MediaCrossPlatformEnabled != nil {
		group.MediaCrossPlatformEnabled = *input.MediaCrossPlatformEnabled
	}
	return group, nil
}

func TestAdminGroupDTOIncludesMediaFlags(t *testing.T) {
	groupDTO := handlerdto.GroupFromServiceAdmin(&service.Group{
		AllowImageGeneration:      true,
		AllowVideoGeneration:      true,
		MediaCrossPlatformEnabled: true,
	})

	require.True(t, groupDTO.AllowImageGeneration)
	require.True(t, groupDTO.AllowVideoGeneration)
	require.True(t, groupDTO.MediaCrossPlatformEnabled)
}

func TestGroupHandlerCreateForwardsMediaFlags(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adminSvc := &mediaGroupAdminService{stubAdminService: newStubAdminService()}
	handler := NewGroupHandler(adminSvc, nil, nil)
	router := gin.New()
	router.POST("/api/v1/admin/groups", handler.Create)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/groups", bytes.NewBufferString(`{
		"name":"media",
		"platform":"media",
		"allow_image_generation":true,
		"allow_video_generation":true,
		"media_cross_platform_enabled":true
	}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NotNil(t, adminSvc.created)
	require.Equal(t, service.PlatformMedia, adminSvc.created.Platform)
	require.True(t, adminSvc.created.AllowImageGeneration)
	require.True(t, adminSvc.created.AllowVideoGeneration)
	require.True(t, adminSvc.created.MediaCrossPlatformEnabled)
	require.Contains(t, rec.Body.String(), `"allow_video_generation":true`)
	require.Contains(t, rec.Body.String(), `"media_cross_platform_enabled":true`)
}

func TestGroupHandlerUpdateForwardsExplicitFalseMediaFlags(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adminSvc := &mediaGroupAdminService{stubAdminService: newStubAdminService()}
	handler := NewGroupHandler(adminSvc, nil, nil)
	router := gin.New()
	router.PUT("/api/v1/admin/groups/:id", handler.Update)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/groups/10", bytes.NewBufferString(`{
		"allow_image_generation":false,
		"allow_video_generation":false,
		"media_cross_platform_enabled":false
	}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NotNil(t, adminSvc.updated)
	require.NotNil(t, adminSvc.updated.AllowImageGeneration)
	require.NotNil(t, adminSvc.updated.AllowVideoGeneration)
	require.NotNil(t, adminSvc.updated.MediaCrossPlatformEnabled)
	require.False(t, *adminSvc.updated.AllowImageGeneration)
	require.False(t, *adminSvc.updated.AllowVideoGeneration)
	require.False(t, *adminSvc.updated.MediaCrossPlatformEnabled)
}
