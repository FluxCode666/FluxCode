package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type mediaModelHandlerStore struct {
	records      []service.MediaModelAdminRecord
	nextID       int64
	refreshCalls int
}

func (s *mediaModelHandlerStore) ListEnabled(context.Context) ([]service.MediaModelDefinition, error) {
	s.refreshCalls++
	items := make([]service.MediaModelDefinition, 0, len(s.records))
	for _, record := range s.records {
		if record.Definition.Enabled {
			items = append(items, cloneHandlerMediaRecord(record).Definition)
		}
	}
	return items, nil
}

func (s *mediaModelHandlerStore) ListAll(context.Context) ([]service.MediaModelAlias, error) {
	items := []service.MediaModelAlias{}
	for _, record := range s.records {
		if !record.Definition.Enabled {
			continue
		}
		for _, alias := range record.Aliases {
			items = append(items, service.MediaModelAlias{RequestedModelID: alias, ModelDefinitionID: record.Definition.ID})
		}
	}
	return items, nil
}

func (s *mediaModelHandlerStore) ListAdmin(context.Context) ([]service.MediaModelAdminRecord, error) {
	items := make([]service.MediaModelAdminRecord, len(s.records))
	for index, record := range s.records {
		items[index] = cloneHandlerMediaRecord(record)
	}
	return items, nil
}

func (s *mediaModelHandlerStore) GetAdminByID(_ context.Context, id int64) (*service.MediaModelAdminRecord, error) {
	for _, record := range s.records {
		if record.Definition.ID == id {
			copy := cloneHandlerMediaRecord(record)
			return &copy, nil
		}
	}
	return nil, service.ErrMediaModelDefinitionNotFound
}

func (s *mediaModelHandlerStore) CreateAdmin(_ context.Context, record service.MediaModelAdminRecord) (*service.MediaModelAdminRecord, error) {
	s.nextID++
	record.Definition.ID = s.nextID
	s.records = append(s.records, cloneHandlerMediaRecord(record))
	copy := cloneHandlerMediaRecord(record)
	return &copy, nil
}

func (s *mediaModelHandlerStore) UpdateAdmin(_ context.Context, id int64, record service.MediaModelAdminRecord) (*service.MediaModelAdminRecord, error) {
	for index := range s.records {
		if s.records[index].Definition.ID == id {
			record.Definition.ID = id
			s.records[index] = cloneHandlerMediaRecord(record)
			copy := cloneHandlerMediaRecord(record)
			return &copy, nil
		}
	}
	return nil, service.ErrMediaModelDefinitionNotFound
}

func (s *mediaModelHandlerStore) DeleteAdmin(_ context.Context, id int64) error {
	for index := range s.records {
		if s.records[index].Definition.ID == id {
			s.records = append(s.records[:index], s.records[index+1:]...)
			return nil
		}
	}
	return service.ErrMediaModelDefinitionNotFound
}

type mediaModelHandlerGroupRepo struct {
	service.GroupRepository
	group *service.Group
}

func (r *mediaModelHandlerGroupRepo) GetByIDLite(context.Context, int64) (*service.Group, error) {
	if r.group == nil {
		return nil, service.ErrGroupNotFound
	}
	copy := *r.group
	return &copy, nil
}

type mediaModelHandlerScopeRepo struct {
	modelIDs []string
}

func (r *mediaModelHandlerScopeRepo) ListMediaModelIDs(context.Context, int64) ([]string, error) {
	return append([]string(nil), r.modelIDs...), nil
}

func (r *mediaModelHandlerScopeRepo) ListEnabledMediaModelIDs(context.Context, int64) ([]string, error) {
	return append([]string(nil), r.modelIDs...), nil
}

func (r *mediaModelHandlerScopeRepo) ReplaceMediaModelScopes(_ context.Context, _ int64, modelIDs []string) error {
	r.modelIDs = append([]string(nil), modelIDs...)
	return nil
}

func cloneHandlerMediaRecord(record service.MediaModelAdminRecord) service.MediaModelAdminRecord {
	record.Definition.Operations = append([]service.MediaOperation(nil), record.Definition.Operations...)
	record.Definition.Constraints = append(json.RawMessage(nil), record.Definition.Constraints...)
	record.Aliases = append([]string(nil), record.Aliases...)
	return record
}

func newMediaModelHandlerFixture(t *testing.T, platform string) (*gin.Engine, *mediaModelHandlerStore, *service.MediaModelRegistry) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	store := &mediaModelHandlerStore{}
	registry := service.NewMediaModelRegistry(store)
	require.NoError(t, registry.Refresh(context.Background()))
	scopes := &mediaModelHandlerScopeRepo{}
	groups := &mediaModelHandlerGroupRepo{group: &service.Group{ID: 7, Platform: platform}}
	svc := service.NewMediaModelAdminService(store, scopes, groups, registry)
	handler := NewMediaModelAdminHandler(svc)
	router := gin.New()
	router.GET("/admin/media-models", handler.List)
	router.POST("/admin/media-models", handler.Create)
	router.GET("/admin/media-models/:id", handler.GetByID)
	router.PUT("/admin/media-models/:id", handler.Update)
	router.DELETE("/admin/media-models/:id", handler.Delete)
	router.GET("/admin/groups/:id/media-model-scopes", handler.GetGroupScopes)
	router.PUT("/admin/groups/:id/media-model-scopes", handler.ReplaceGroupScopes)
	return router, store, registry
}

func validMediaModelHandlerBody() string {
	return `{
		"model_id":" GPT-IMAGE-2 ",
		"vendor":" OpenAI ",
		"media_type":"image",
		"operations":["text_to_image"],
		"constraints":{"image_sizes":["1024x1024"]},
		"billing_unit":"image",
		"default_adapter":" OpenAI-Images ",
		"default_async_mode":"optional",
		"enabled":true,
		"aliases":[" GPT-IMAGE-LATEST "]
	}`
}

func performMediaModelHandlerRequest(router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)
	return recorder
}

func TestMediaModelAdminHandlerCreateReturnsCompleteNormalizedModelAndRefreshesRegistry(t *testing.T) {
	router, store, registry := newMediaModelHandlerFixture(t, service.PlatformMedia)
	recorder := performMediaModelHandlerRequest(router, http.MethodPost, "/admin/media-models", validMediaModelHandlerBody())

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Len(t, store.records, 1)
	require.Contains(t, recorder.Body.String(), `"model_id":"gpt-image-2"`)
	require.Contains(t, recorder.Body.String(), `"vendor":"openai"`)
	require.Contains(t, recorder.Body.String(), `"default_adapter":"openai-images"`)
	require.Contains(t, recorder.Body.String(), `"aliases":["gpt-image-latest"]`)
	resolved, err := registry.Resolve("gpt-image-latest", service.MediaOperationTextToImage)
	require.NoError(t, err)
	require.Equal(t, "gpt-image-2", resolved.ModelID)
	require.Equal(t, 2, store.refreshCalls)

	listRecorder := performMediaModelHandlerRequest(router, http.MethodGet, "/admin/media-models", "")
	require.Equal(t, http.StatusOK, listRecorder.Code)
	require.Contains(t, listRecorder.Body.String(), `"items":[{`)
}

func TestMediaModelAdminHandlerUpdateAndDeleteRefreshRegistry(t *testing.T) {
	router, store, registry := newMediaModelHandlerFixture(t, service.PlatformMedia)
	created := performMediaModelHandlerRequest(router, http.MethodPost, "/admin/media-models", validMediaModelHandlerBody())
	require.Equal(t, http.StatusOK, created.Code, created.Body.String())

	updatedBody := strings.ReplaceAll(validMediaModelHandlerBody(), "GPT-IMAGE-LATEST", "GPT-IMAGE-NEXT")
	updated := performMediaModelHandlerRequest(router, http.MethodPut, "/admin/media-models/1", updatedBody)
	require.Equal(t, http.StatusOK, updated.Code, updated.Body.String())
	_, err := registry.Resolve("gpt-image-latest", service.MediaOperationTextToImage)
	require.ErrorIs(t, err, service.ErrMediaModelNotFound)
	resolved, err := registry.Resolve("gpt-image-next", service.MediaOperationTextToImage)
	require.NoError(t, err)
	require.Equal(t, "gpt-image-2", resolved.ModelID)

	deleted := performMediaModelHandlerRequest(router, http.MethodDelete, "/admin/media-models/1", "")
	require.Equal(t, http.StatusOK, deleted.Code, deleted.Body.String())
	require.NotContains(t, deleted.Body.String(), `"data"`)
	_, err = registry.Resolve("gpt-image-next", service.MediaOperationTextToImage)
	require.ErrorIs(t, err, service.ErrMediaModelNotFound)
	require.Equal(t, 4, store.refreshCalls)
}

func TestMediaModelAdminHandlerRejectsCanonicalModelIDRename(t *testing.T) {
	router, _, _ := newMediaModelHandlerFixture(t, service.PlatformMedia)
	created := performMediaModelHandlerRequest(router, http.MethodPost, "/admin/media-models", validMediaModelHandlerBody())
	require.Equal(t, http.StatusOK, created.Code, created.Body.String())

	updatedBody := strings.ReplaceAll(validMediaModelHandlerBody(), "GPT-IMAGE-2", "GPT-IMAGE-3")
	updated := performMediaModelHandlerRequest(router, http.MethodPut, "/admin/media-models/1", updatedBody)
	require.Equal(t, http.StatusBadRequest, updated.Code, updated.Body.String())
	require.Contains(t, updated.Body.String(), `"reason":"MEDIA_MODEL_ID_IMMUTABLE"`)
}

func TestMediaModelAdminHandlerStrictlyRejectsInvalidDefinitions(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "unknown request field", body: `{"model_id":"image","vendor":"openai","media_type":"image","operations":["text_to_image"],"constraints":{},"billing_unit":"image","default_adapter":"openai-images","default_async_mode":"optional","enabled":true,"aliases":[],"script":"x"}`},
		{name: "invalid adapter", body: `{"model_id":"image","vendor":"openai","media_type":"image","operations":["text_to_image"],"constraints":{},"billing_unit":"image","default_adapter":"bad adapter","default_async_mode":"optional","enabled":true,"aliases":[]}`},
		{name: "operation type mismatch", body: `{"model_id":"image","vendor":"openai","media_type":"image","operations":["text_to_video"],"constraints":{},"billing_unit":"image","default_adapter":"openai-images","default_async_mode":"optional","enabled":true,"aliases":[]}`},
		{name: "invalid async mode", body: `{"model_id":"image","vendor":"openai","media_type":"image","operations":["text_to_image"],"constraints":{},"billing_unit":"image","default_adapter":"openai-images","default_async_mode":"sometimes","enabled":true,"aliases":[]}`},
		{name: "unknown constraints field", body: `{"model_id":"image","vendor":"openai","media_type":"image","operations":["text_to_image"],"constraints":{"script":"x"},"billing_unit":"image","default_adapter":"openai-images","default_async_mode":"optional","enabled":true,"aliases":[]}`},
		{name: "duplicate normalized alias", body: `{"model_id":"image","vendor":"openai","media_type":"image","operations":["text_to_image"],"constraints":{},"billing_unit":"image","default_adapter":"openai-images","default_async_mode":"optional","enabled":true,"aliases":["Alias"," alias "]}`},
		{name: "missing enabled", body: `{"model_id":"image","vendor":"openai","media_type":"image","operations":["text_to_image"],"constraints":{},"billing_unit":"image","default_adapter":"openai-images","default_async_mode":"optional","aliases":[]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, store, _ := newMediaModelHandlerFixture(t, service.PlatformMedia)
			recorder := performMediaModelHandlerRequest(router, http.MethodPost, "/admin/media-models", tt.body)
			require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
			require.Empty(t, store.records)
		})
	}
}

func TestMediaModelAdminHandlerGroupScopesContractAndMediaIsolation(t *testing.T) {
	router, store, _ := newMediaModelHandlerFixture(t, service.PlatformMedia)
	putRecorder := performMediaModelHandlerRequest(router, http.MethodPut, "/admin/groups/7/media-model-scopes", `{"model_ids":[" IMAGE-TWO ","image-one"]}`)
	require.Equal(t, http.StatusOK, putRecorder.Code, putRecorder.Body.String())
	require.Contains(t, putRecorder.Body.String(), `"model_ids":["image-one","image-two"]`)
	require.Equal(t, 1, store.refreshCalls)

	getRecorder := performMediaModelHandlerRequest(router, http.MethodGet, "/admin/groups/7/media-model-scopes", "")
	require.Equal(t, http.StatusOK, getRecorder.Code, getRecorder.Body.String())
	require.Contains(t, getRecorder.Body.String(), `"model_ids":["image-one","image-two"]`)

	textRouter, _, _ := newMediaModelHandlerFixture(t, service.PlatformOpenAI)
	denied := performMediaModelHandlerRequest(textRouter, http.MethodPut, "/admin/groups/7/media-model-scopes", `{"model_ids":[]}`)
	require.Equal(t, http.StatusBadRequest, denied.Code, denied.Body.String())
	require.Contains(t, denied.Body.String(), `"reason":"MEDIA_GROUP_REQUIRED"`)
}

func TestMediaModelAdminHandlerGroupScopesRequiresUniqueModelIDs(t *testing.T) {
	router, _, _ := newMediaModelHandlerFixture(t, service.PlatformMedia)
	recorder := performMediaModelHandlerRequest(router, http.MethodPut, "/admin/groups/7/media-model-scopes", `{"model_ids":["image-one"," IMAGE-ONE "]}`)
	require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), `"reason":"INVALID_MEDIA_MODEL"`)
}
