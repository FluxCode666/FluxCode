package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGeneratedImageHandlerListAndContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &generatedImageHandlerStoreStub{
		items: []service.GeneratedImage{{
			ID:             7,
			Provider:       service.GeneratedImageProviderOpenAI,
			UserID:         12,
			APIKeyID:       34,
			AccountID:      56,
			UserEmail:      "artist@example.com",
			APIKeyName:     "Gallery Key",
			AccountName:    "OpenAI Images",
			AccountGroups:  []string{"image-group"},
			RequestID:      "req_img",
			Model:          "gpt-image-2",
			Prompt:         "draw",
			RevisedPrompt:  "draw refined",
			ResponseFormat: "url",
			Source:         "b64_json",
			ContentType:    "image/png",
			SizeBytes:      9,
			CreatedAt:      time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC),
		}},
		content:     []byte("png-bytes"),
		contentType: "image/png",
	}
	handler := NewGeneratedImageHandler(store)
	router := gin.New()
	router.GET("/admin/generated-images", handler.List)
	router.GET("/admin/generated-images/:id/content", handler.Content)

	listRec := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/admin/generated-images?page=1&page_size=20&user_email=artist%40example.com&group_id=9&start_at=2026-06-20&end_at=2026-06-27", nil)
	router.ServeHTTP(listRec, listReq)

	require.Equal(t, http.StatusOK, listRec.Code)
	require.Equal(t, "artist@example.com", store.lastListParams.UserEmail)
	require.Equal(t, int64(9), store.lastListParams.GroupID)
	require.Equal(t, time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC), *store.lastListParams.StartAt)
	require.Equal(t, time.Date(2026, 6, 28, 0, 0, 0, 0, time.UTC), *store.lastListParams.EndAt)

	var listResp response.Response
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listResp))
	data, ok := listResp.Data.(map[string]any)
	require.True(t, ok)
	items, ok := data["items"].([]any)
	require.True(t, ok)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
	require.Equal(t, float64(7), item["id"])
	require.Equal(t, "openai", item["provider"])
	require.Equal(t, "artist@example.com", item["user_email"])
	require.Equal(t, "Gallery Key", item["api_key_name"])
	require.Equal(t, "OpenAI Images", item["account_name"])
	require.Equal(t, []any{"image-group"}, item["account_group_names"])
	require.Equal(t, "/api/v1/admin/generated-images/7/content", item["content_url"])
	require.NotContains(t, item, "image_data")

	contentRec := httptest.NewRecorder()
	contentReq := httptest.NewRequest(http.MethodGet, "/admin/generated-images/7/content", nil)
	router.ServeHTTP(contentRec, contentReq)

	require.Equal(t, http.StatusOK, contentRec.Code)
	require.Equal(t, "image/png", contentRec.Header().Get("Content-Type"))
	require.Equal(t, []byte("png-bytes"), contentRec.Body.Bytes())
}

type generatedImageHandlerStoreStub struct {
	items          []service.GeneratedImage
	content        []byte
	contentType    string
	lastListParams service.GeneratedImageListParams
}

func (s *generatedImageHandlerStoreStub) Create(ctx context.Context, image *service.GeneratedImage) (*service.GeneratedImage, error) {
	return image, nil
}

func (s *generatedImageHandlerStoreStub) List(ctx context.Context, params service.GeneratedImageListParams) ([]service.GeneratedImage, *pagination.PaginationResult, error) {
	s.lastListParams = params
	return s.items, &pagination.PaginationResult{Total: int64(len(s.items)), Page: params.Page, PageSize: params.PageSize, Pages: 1}, nil
}

func (s *generatedImageHandlerStoreStub) GetContent(ctx context.Context, id int64) ([]byte, string, error) {
	return s.content, s.contentType, nil
}
