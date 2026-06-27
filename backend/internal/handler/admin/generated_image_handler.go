package admin

import (
	"net/http"
	"strconv"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type GeneratedImageHandler struct {
	store service.GeneratedImageStore
}

type generatedImageListItem struct {
	ID             int64  `json:"id"`
	Provider       string `json:"provider"`
	UserID         int64  `json:"user_id"`
	APIKeyID       int64  `json:"api_key_id"`
	AccountID      int64  `json:"account_id"`
	RequestID      string `json:"request_id"`
	Model          string `json:"model"`
	Prompt         string `json:"prompt"`
	RevisedPrompt  string `json:"revised_prompt"`
	ResponseFormat string `json:"response_format"`
	Source         string `json:"source"`
	ContentType    string `json:"content_type"`
	SizeBytes      int    `json:"size_bytes"`
	ContentURL     string `json:"content_url"`
	CreatedAt      string `json:"created_at"`
}

func NewGeneratedImageHandler(store service.GeneratedImageStore) *GeneratedImageHandler {
	return &GeneratedImageHandler{store: store}
}

func (h *GeneratedImageHandler) List(c *gin.Context) {
	if h == nil || h.store == nil {
		response.InternalError(c, "Generated image store is not configured")
		return
	}
	page, pageSize := response.ParsePagination(c)
	items, pageResult, err := h.store.List(c.Request.Context(), pagination.PaginationParams{Page: page, PageSize: pageSize})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	out := make([]generatedImageListItem, 0, len(items))
	for _, item := range items {
		out = append(out, generatedImageToListItem(item))
	}
	response.PaginatedWithResult(c, out, &response.PaginationResult{
		Total:    pageResult.Total,
		Page:     pageResult.Page,
		PageSize: pageResult.PageSize,
		Pages:    pageResult.Pages,
	})
}

func (h *GeneratedImageHandler) Content(c *gin.Context) {
	if h == nil || h.store == nil {
		response.InternalError(c, "Generated image store is not configured")
		return
	}
	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid image id")
		return
	}
	data, contentType, err := h.store.GetContent(c.Request.Context(), id)
	if err != nil {
		if dbent.IsNotFound(err) {
			response.NotFound(c, "Generated image not found")
			return
		}
		response.InternalError(c, err.Error())
		return
	}
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/octet-stream"
	}
	c.Header("Cache-Control", "private, max-age=300")
	c.Data(http.StatusOK, contentType, data)
}

func generatedImageToListItem(item service.GeneratedImage) generatedImageListItem {
	return generatedImageListItem{
		ID:             item.ID,
		Provider:       item.Provider,
		UserID:         item.UserID,
		APIKeyID:       item.APIKeyID,
		AccountID:      item.AccountID,
		RequestID:      item.RequestID,
		Model:          item.Model,
		Prompt:         item.Prompt,
		RevisedPrompt:  item.RevisedPrompt,
		ResponseFormat: item.ResponseFormat,
		Source:         item.Source,
		ContentType:    item.ContentType,
		SizeBytes:      item.SizeBytes,
		ContentURL:     "/api/v1/admin/generated-images/" + strconv.FormatInt(item.ID, 10) + "/content",
		CreatedAt:      item.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
