package admin

import (
	"net/http"
	"strconv"
	"strings"
	"time"

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
	ID             int64    `json:"id"`
	Provider       string   `json:"provider"`
	UserID         int64    `json:"user_id"`
	APIKeyID       int64    `json:"api_key_id"`
	AccountID      int64    `json:"account_id"`
	UserEmail      string   `json:"user_email"`
	APIKeyName     string   `json:"api_key_name"`
	AccountName    string   `json:"account_name"`
	AccountGroups  []string `json:"account_group_names"`
	RequestID      string   `json:"request_id"`
	Model          string   `json:"model"`
	Prompt         string   `json:"prompt"`
	RevisedPrompt  string   `json:"revised_prompt"`
	ResponseFormat string   `json:"response_format"`
	Source         string   `json:"source"`
	ContentType    string   `json:"content_type"`
	SizeBytes      int      `json:"size_bytes"`
	ContentURL     string   `json:"content_url"`
	CreatedAt      string   `json:"created_at"`
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
	params := service.GeneratedImageListParams{
		PaginationParams: pagination.PaginationParams{Page: page, PageSize: pageSize},
		UserEmail:        strings.TrimSpace(c.Query("user_email")),
	}
	if groupIDText := strings.TrimSpace(c.Query("group_id")); groupIDText != "" {
		groupID, err := strconv.ParseInt(groupIDText, 10, 64)
		if err != nil || groupID <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
		}
		params.GroupID = groupID
	}
	if startText := strings.TrimSpace(c.Query("start_at")); startText != "" {
		startAt, err := parseGeneratedImageDateParam(startText, false)
		if err != nil {
			response.BadRequest(c, "Invalid start_at")
			return
		}
		params.StartAt = &startAt
	}
	if endText := strings.TrimSpace(c.Query("end_at")); endText != "" {
		endAt, err := parseGeneratedImageDateParam(endText, true)
		if err != nil {
			response.BadRequest(c, "Invalid end_at")
			return
		}
		params.EndAt = &endAt
	}
	items, pageResult, err := h.store.List(c.Request.Context(), params)
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

func (h *GeneratedImageHandler) DeleteByDateRange(c *gin.Context) {
	if h == nil || h.store == nil {
		response.InternalError(c, "Generated image store is not configured")
		return
	}

	startText := strings.TrimSpace(c.Query("start_at"))
	endText := strings.TrimSpace(c.Query("end_at"))
	if startText == "" || endText == "" {
		response.BadRequest(c, "start_at and end_at are required")
		return
	}

	startAt, err := parseGeneratedImageDateParam(startText, false)
	if err != nil {
		response.BadRequest(c, "Invalid start_at")
		return
	}
	endAt, err := parseGeneratedImageDateParam(endText, true)
	if err != nil {
		response.BadRequest(c, "Invalid end_at")
		return
	}
	if !endAt.After(startAt) {
		response.BadRequest(c, "end_at must be after start_at")
		return
	}

	deleted, err := h.store.DeleteByDateRange(c.Request.Context(), startAt, endAt)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{"deleted_count": deleted})
}

func generatedImageToListItem(item service.GeneratedImage) generatedImageListItem {
	return generatedImageListItem{
		ID:             item.ID,
		Provider:       item.Provider,
		UserID:         item.UserID,
		APIKeyID:       item.APIKeyID,
		AccountID:      item.AccountID,
		UserEmail:      item.UserEmail,
		APIKeyName:     item.APIKeyName,
		AccountName:    item.AccountName,
		AccountGroups:  append([]string(nil), item.AccountGroups...),
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

func parseGeneratedImageDateParam(value string, endOfDate bool) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed, nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, err
	}
	if endOfDate {
		return parsed.AddDate(0, 0, 1), nil
	}
	return parsed, nil
}
