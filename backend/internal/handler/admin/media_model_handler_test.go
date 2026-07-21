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
	"github.com/tidwall/gjson"
)

type mediaModelHandlerStore struct {
	records      []service.MediaModelAdminRecord
	nextID       int64
	refreshCalls int
	writeCount   int
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
	s.writeCount++
	s.nextID++
	record.Definition.ID = s.nextID
	s.records = append(s.records, cloneHandlerMediaRecord(record))
	copy := cloneHandlerMediaRecord(record)
	return &copy, nil
}

func (s *mediaModelHandlerStore) UpdateAdmin(_ context.Context, id int64, record service.MediaModelAdminRecord) (*service.MediaModelAdminRecord, error) {
	s.writeCount++
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
	s.writeCount++
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
	record.Definition.AdapterResolution = cloneHandlerMediaAdapterResolution(record.Definition.AdapterResolution)
	record.AdapterResolution = cloneHandlerMediaAdapterResolution(record.AdapterResolution)
	record.Aliases = append([]string(nil), record.Aliases...)
	return record
}

func cloneHandlerMediaAdapterResolution(resolution service.MediaAdapterResolution) service.MediaAdapterResolution {
	if resolution.Capabilities != nil {
		capabilities := *resolution.Capabilities
		capabilities.Operations = append([]service.MediaOperation(nil), resolution.Capabilities.Operations...)
		resolution.Capabilities = &capabilities
	}
	return resolution
}

func defaultMediaModelHandlerRegistration() service.MediaAdapterRegistration {
	return service.MediaAdapterRegistration{
		Key: "openai-images",
		Adapter: service.NewFakeMediaAdapter(service.FakeMediaAdapterOptions{
			Name: "openai-images", NativeAsyncMode: service.NativeAsyncOptional,
		}),
		SupportedOperations: []service.MediaOperation{service.MediaOperationTextToImage},
		ExactRules: []service.MediaAdapterExactRule{{
			Vendor: "openai", ModelID: "gpt-image-2",
			Capabilities: service.MediaAdapterRuleCapabilities{
				Operations:          []service.MediaOperation{service.MediaOperationTextToImage},
				SyncUpstream:        true,
				NativeAsyncUpstream: true,
			},
		}},
	}
}

func newMediaModelHandlerFixture(t *testing.T, platform string, registrations ...service.MediaAdapterRegistration) (*gin.Engine, *mediaModelHandlerStore, *service.MediaModelRegistry) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	if len(registrations) == 0 {
		registrations = []service.MediaAdapterRegistration{defaultMediaModelHandlerRegistration()}
	}
	store := &mediaModelHandlerStore{}
	adapterRegistry := service.NewMediaAdapterRegistry()
	for _, registration := range registrations {
		require.NoError(t, adapterRegistry.RegisterDefinition(registration))
	}
	require.NoError(t, adapterRegistry.Validate())
	resolver := service.NewMediaAdapterResolver(adapterRegistry)
	registry := service.NewMediaModelRegistryWithResolver(store, resolver)
	require.NoError(t, registry.Refresh(context.Background()))
	scopes := &mediaModelHandlerScopeRepo{}
	groups := &mediaModelHandlerGroupRepo{group: &service.Group{ID: 7, Platform: platform}}
	svc := service.NewMediaModelAdminService(store, scopes, groups, registry, resolver)
	handler := NewMediaModelAdminHandler(svc)
	router := gin.New()
	router.GET("/admin/media-models", handler.List)
	router.POST("/admin/media-models", handler.Create)
	router.GET("/admin/media-models/preflight", handler.Preflight)
	router.POST("/admin/media-models/request-mapping-preview", handler.PreviewRequestMapping)
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

func TestMediaModelHandlerIgnoresDeprecatedAdapterInputs(t *testing.T) {
	registration := service.MediaAdapterRegistration{
		Key: "xai-image",
		Adapter: service.NewFakeMediaAdapter(service.FakeMediaAdapterOptions{
			Name: "xai-image", NativeAsyncMode: service.NativeAsyncUnsupported,
		}),
		SupportedOperations: []service.MediaOperation{service.MediaOperationTextToImage},
		ExactRules: []service.MediaAdapterExactRule{{
			Vendor: "xai", ModelID: "grok-2-image",
			Capabilities: service.MediaAdapterRuleCapabilities{
				Operations: []service.MediaOperation{service.MediaOperationTextToImage}, SyncUpstream: true,
			},
		}},
	}
	expectedResolution := `{"status":"ready","resolved_adapter":"xai-image","matched_by":"exact","matched_family":"","capabilities":{"operations":["text_to_image"],"sync_upstream":true,"native_async_upstream":false,"content_fetch":false},"reason_code":""}`
	for _, tt := range []struct {
		name             string
		deprecatedFields string
	}{
		{name: "adapter string and async null", deprecatedFields: `"default_adapter":"client-value","default_async_mode":null`},
		{name: "adapter null and arbitrary async string", deprecatedFields: `"default_adapter":null,"default_async_mode":"sometimes"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			router, store, _ := newMediaModelHandlerFixture(t, service.PlatformMedia, registration)
			body := `{"model_id":"grok-2-image","vendor":"xai","media_type":"image","operations":["text_to_image"],"constraints":{},"billing_unit":"image","enabled":true,"aliases":[],` + tt.deprecatedFields + `}`
			recorder := performMediaModelHandlerRequest(router, http.MethodPost, "/admin/media-models", body)

			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			require.Len(t, store.records, 1)
			require.Empty(t, store.records[0].Definition.DefaultAdapter)
			require.Empty(t, store.records[0].Definition.DefaultAsyncMode)
			require.Empty(t, store.records[0].LegacyDefaultAdapter)
			require.JSONEq(t, expectedResolution, gjson.Get(recorder.Body.String(), "data.adapter_resolution").Raw)
			require.Equal(t, "xai-image", gjson.Get(recorder.Body.String(), "data.default_adapter").String())
			require.Equal(t, string(service.NativeAsyncUnsupported), gjson.Get(recorder.Body.String(), "data.default_async_mode").String())
		})
	}
}

func TestMediaModelHandlerReturnsSameResolutionAcrossReadAndWriteResponses(t *testing.T) {
	router, store, _ := newMediaModelHandlerFixture(t, service.PlatformMedia)
	created := performMediaModelHandlerRequest(router, http.MethodPost, "/admin/media-models", validMediaModelHandlerBody())
	require.Equal(t, http.StatusOK, created.Code, created.Body.String())
	require.Len(t, store.records, 1)
	store.records[0].LegacyDefaultAdapter = "legacy-wrong-adapter"
	store.records[0].LegacyDefaultAsyncMode = service.NativeAsyncRequired

	got := performMediaModelHandlerRequest(router, http.MethodGet, "/admin/media-models/1", "")
	listed := performMediaModelHandlerRequest(router, http.MethodGet, "/admin/media-models", "")
	updatedBody := strings.ReplaceAll(validMediaModelHandlerBody(), "GPT-IMAGE-LATEST", "GPT-IMAGE-NEXT")
	updated := performMediaModelHandlerRequest(router, http.MethodPut, "/admin/media-models/1", updatedBody)
	for name, recorder := range map[string]*httptest.ResponseRecorder{
		"create": created, "get": got, "list": listed, "update": updated,
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			path := "data.adapter_resolution"
			if name == "list" {
				path = "data.items.0.adapter_resolution"
			}
			require.JSONEq(t, `{"status":"ready","resolved_adapter":"openai-images","matched_by":"exact","matched_family":"","capabilities":{"operations":["text_to_image"],"sync_upstream":true,"native_async_upstream":true,"content_fetch":false},"reason_code":""}`, gjson.Get(recorder.Body.String(), path).Raw)
			basePath := "data"
			if name == "list" {
				basePath = "data.items.0"
			}
			require.Equal(t, "openai-images", gjson.Get(recorder.Body.String(), basePath+".default_adapter").String())
			require.Equal(t, string(service.NativeAsyncOptional), gjson.Get(recorder.Body.String(), basePath+".default_async_mode").String())
		})
	}
}

func TestMediaModelHandlerPreflightIsReadOnlyAndReturnsUnsafeReport(t *testing.T) {
	router, store, _ := newMediaModelHandlerFixture(t, service.PlatformMedia)
	created := performMediaModelHandlerRequest(router, http.MethodPost, "/admin/media-models", validMediaModelHandlerBody())
	require.Equal(t, http.StatusOK, created.Code, created.Body.String())
	store.records[0].LegacyDefaultAdapter = "legacy-wrong-adapter"
	store.records[0].LegacyDefaultAsyncMode = service.NativeAsyncRequired
	writesBefore, refreshesBefore := store.writeCount, store.refreshCalls

	recorder := performMediaModelHandlerRequest(router, http.MethodGet, "/admin/media-models/preflight", "")

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.False(t, gjson.Get(recorder.Body.String(), "data.safe").Bool())
	require.Equal(t, int64(1), gjson.Get(recorder.Body.String(), "data.blocking_count").Int())
	require.Equal(t, "gpt-image-2", gjson.Get(recorder.Body.String(), "data.items.0.model_id").String())
	require.Equal(t, "ready", gjson.Get(recorder.Body.String(), "data.items.0.resolution_status").String())
	require.Equal(t, "openai-images", gjson.Get(recorder.Body.String(), "data.items.0.resolved_adapter").String())
	require.Equal(t, "legacy-wrong-adapter", gjson.Get(recorder.Body.String(), "data.items.0.legacy_default_adapter").String())
	require.False(t, gjson.Get(recorder.Body.String(), "data.items.0.adapter_key_matches").Bool())
	require.False(t, gjson.Get(recorder.Body.String(), "data.items.0.rollout_safe").Bool())
	require.Equal(t, writesBefore, store.writeCount)
	require.Equal(t, refreshesBefore, store.refreshCalls)
}

func TestMediaModelHandlerPreviewsRequestMappingWithoutWrites(t *testing.T) {
	router, store, _ := newMediaModelHandlerFixture(t, service.PlatformMedia)
	writesBefore, refreshesBefore := store.writeCount, store.refreshCalls
	body := `{
		"request":{"prompt":"hello","size":"1024x1024","seed":"42"},
		"mapping":{"rules":[
			{"operation":"rename","source":"size","target":"image_size"},
			{"operation":"default","target":"count","value":1},
			{"operation":"cast","source":"seed","target":"seed_number","cast":"integer"}
		]}
	}`

	recorder := performMediaModelHandlerRequest(router, http.MethodPost, "/admin/media-models/request-mapping-preview", body)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.JSONEq(t, `{"prompt":"hello","image_size":"1024x1024","seed":"42","count":1,"seed_number":42}`, gjson.Get(recorder.Body.String(), "data.result").Raw)
	require.Equal(t, writesBefore, store.writeCount)
	require.Equal(t, refreshesBefore, store.refreshCalls)
}

func TestMediaModelHandlerPreviewSkipsMissingOptionalMappingSource(t *testing.T) {
	router, store, _ := newMediaModelHandlerFixture(t, service.PlatformMedia)
	writesBefore, refreshesBefore := store.writeCount, store.refreshCalls
	body := `{
		"request":{"prompt":"hello"},
		"mapping":{"rules":[
			{"operation":"copy","source":"size","target":"image_size"}
		]}
	}`

	recorder := performMediaModelHandlerRequest(router, http.MethodPost, "/admin/media-models/request-mapping-preview", body)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.JSONEq(t, `{"prompt":"hello"}`, gjson.Get(recorder.Body.String(), "data.result").Raw)
	require.Equal(t, writesBefore, store.writeCount)
	require.Equal(t, refreshesBefore, store.refreshCalls)
}

func TestMediaModelHandlerPreviewRejectsInvalidEnvelopeAndMapping(t *testing.T) {
	router, _, _ := newMediaModelHandlerFixture(t, service.PlatformMedia)
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "missing request", body: `{"mapping":{}}`, want: "request is required"},
		{name: "missing mapping", body: `{"request":{}}`, want: "mapping is required"},
		{name: "request must be object", body: `{"request":[],"mapping":{}}`, want: "cannot unmarshal array"},
		{name: "unknown envelope field", body: `{"request":{},"mapping":{},"script":"x"}`, want: "unknown field"},
		{name: "unknown mapping field", body: `{"request":{},"mapping":{"script":"x"}}`, want: "unknown field"},
		{name: "unknown rule field", body: `{"request":{},"mapping":{"rules":[{"operation":"default","target":"n","value":1,"script":"x"}]}}`, want: "unknown field"},
		{name: "rule missing source", body: `{"request":{},"mapping":{"rules":[{"operation":"copy","target":"image_size"}]}}`, want: "path is empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := performMediaModelHandlerRequest(router, http.MethodPost, "/admin/media-models/request-mapping-preview", tt.body)
			require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
			require.Contains(t, recorder.Body.String(), tt.want)
		})
	}
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
		{name: "operation type mismatch", body: `{"model_id":"image","vendor":"openai","media_type":"image","operations":["text_to_video"],"constraints":{},"billing_unit":"image","default_adapter":"openai-images","default_async_mode":"optional","enabled":true,"aliases":[]}`},
		{name: "unknown constraints field", body: `{"model_id":"image","vendor":"openai","media_type":"image","operations":["text_to_image"],"constraints":{"script":"x"},"billing_unit":"image","default_adapter":"openai-images","default_async_mode":"optional","enabled":true,"aliases":[]}`},
		{name: "unsupported billing unit", body: `{"model_id":"image","vendor":"openai","media_type":"image","operations":["text_to_image"],"constraints":{},"billing_unit":"request","default_adapter":"openai-images","default_async_mode":"optional","enabled":true,"aliases":[]}`},
		{name: "mismatched billing unit", body: `{"model_id":"image","vendor":"openai","media_type":"image","operations":["text_to_image"],"constraints":{},"billing_unit":"second","default_adapter":"openai-images","default_async_mode":"optional","enabled":true,"aliases":[]}`},
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
	registration := service.MediaAdapterRegistration{
		Key: "scope-images",
		Adapter: service.NewFakeMediaAdapter(service.FakeMediaAdapterOptions{
			Name: "scope-images", NativeAsyncMode: service.NativeAsyncUnsupported,
		}),
		SupportedOperations: []service.MediaOperation{service.MediaOperationTextToImage},
		ExactRules: []service.MediaAdapterExactRule{
			{Vendor: "openai", ModelID: "image-one", Capabilities: service.MediaAdapterRuleCapabilities{Operations: []service.MediaOperation{service.MediaOperationTextToImage}, SyncUpstream: true}},
			{Vendor: "openai", ModelID: "image-two", Capabilities: service.MediaAdapterRuleCapabilities{Operations: []service.MediaOperation{service.MediaOperationTextToImage}, SyncUpstream: true}},
		},
	}
	router, store, registry := newMediaModelHandlerFixture(t, service.PlatformMedia, registration)
	store.records = []service.MediaModelAdminRecord{
		{Definition: service.MediaModelDefinition{ID: 1, ModelID: "image-one", Vendor: "openai", MediaType: service.MediaTypeImage, Operations: []service.MediaOperation{service.MediaOperationTextToImage}, Constraints: json.RawMessage(`{}`), BillingUnit: "image", Enabled: true}},
		{Definition: service.MediaModelDefinition{ID: 2, ModelID: "image-two", Vendor: "openai", MediaType: service.MediaTypeImage, Operations: []service.MediaOperation{service.MediaOperationTextToImage}, Constraints: json.RawMessage(`{}`), BillingUnit: "image", Enabled: true}},
	}
	require.NoError(t, registry.Refresh(context.Background()))
	putRecorder := performMediaModelHandlerRequest(router, http.MethodPut, "/admin/groups/7/media-model-scopes", `{"model_ids":[" IMAGE-TWO ","image-one"]}`)
	require.Equal(t, http.StatusOK, putRecorder.Code, putRecorder.Body.String())
	require.Contains(t, putRecorder.Body.String(), `"model_ids":["image-one","image-two"]`)
	require.Equal(t, 2, store.refreshCalls)

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
