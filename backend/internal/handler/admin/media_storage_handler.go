package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type MediaStorageHandler struct {
	service *service.MediaStorageSettingsService
}

func NewMediaStorageHandler(storageService *service.MediaStorageSettingsService) *MediaStorageHandler {
	return &MediaStorageHandler{service: storageService}
}

func (h *MediaStorageHandler) Get(c *gin.Context) {
	if h == nil || h.service == nil {
		response.InternalError(c, "media storage settings are unavailable")
		return
	}
	cfg, err := h.service.GetConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

func (h *MediaStorageHandler) Update(c *gin.Context) {
	if h == nil || h.service == nil {
		response.InternalError(c, "media storage settings are unavailable")
		return
	}
	var req service.MediaStorageConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid media storage settings")
		return
	}
	cfg, err := h.service.UpdateConfig(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

func (h *MediaStorageHandler) Test(c *gin.Context) {
	if h == nil || h.service == nil {
		response.InternalError(c, "media storage settings are unavailable")
		return
	}
	var req service.MediaStorageConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid media storage settings")
		return
	}
	if err := h.service.TestConfig(c.Request.Context(), req); err != nil {
		response.Success(c, gin.H{"ok": false, "message": err.Error()})
		return
	}
	response.Success(c, gin.H{"ok": true, "message": "connection successful"})
}
