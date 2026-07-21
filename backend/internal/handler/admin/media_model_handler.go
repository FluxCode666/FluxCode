package admin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type MediaModelAdminHandler struct {
	service *service.MediaModelAdminService
}

func NewMediaModelAdminHandler(service *service.MediaModelAdminService) *MediaModelAdminHandler {
	return &MediaModelAdminHandler{service: service}
}

type mediaModelWriteRequest struct {
	ModelID                  string                   `json:"model_id"`
	Vendor                   string                   `json:"vendor"`
	MediaType                service.MediaType        `json:"media_type"`
	Operations               []service.MediaOperation `json:"operations"`
	Constraints              json.RawMessage          `json:"constraints"`
	BillingUnit              string                   `json:"billing_unit"`
	DeprecatedDefaultAdapter json.RawMessage          `json:"default_adapter"`
	DeprecatedDefaultAsync   json.RawMessage          `json:"default_async_mode"`
	Enabled                  *bool                    `json:"enabled"`
	Aliases                  []string                 `json:"aliases"`
}

type mediaAdapterCapabilitiesResponse struct {
	Operations          []service.MediaOperation `json:"operations"`
	SyncUpstream        bool                     `json:"sync_upstream"`
	NativeAsyncUpstream bool                     `json:"native_async_upstream"`
	ContentFetch        bool                     `json:"content_fetch"`
}

type mediaAdapterResolutionResponse struct {
	Status          service.MediaAdapterResolutionStatus `json:"status"`
	ResolvedAdapter string                               `json:"resolved_adapter"`
	MatchedBy       service.MediaAdapterMatchType        `json:"matched_by"`
	MatchedFamily   string                               `json:"matched_family"`
	Capabilities    *mediaAdapterCapabilitiesResponse    `json:"capabilities"`
	ReasonCode      string                               `json:"reason_code"`
}

type mediaModelResponse struct {
	ID                int64                          `json:"id"`
	ModelID           string                         `json:"model_id"`
	Vendor            string                         `json:"vendor"`
	MediaType         service.MediaType              `json:"media_type"`
	Operations        []service.MediaOperation       `json:"operations"`
	Constraints       json.RawMessage                `json:"constraints"`
	BillingUnit       string                         `json:"billing_unit"`
	DefaultAdapter    string                         `json:"default_adapter"`
	DefaultAsyncMode  service.NativeAsyncMode        `json:"default_async_mode"`
	AdapterResolution mediaAdapterResolutionResponse `json:"adapter_resolution"`
	Enabled           bool                           `json:"enabled"`
	Aliases           []string                       `json:"aliases"`
	CreatedAt         time.Time                      `json:"created_at"`
	UpdatedAt         time.Time                      `json:"updated_at"`
}

type mediaModelListResponse struct {
	Items []mediaModelResponse `json:"items"`
}

type mediaModelScopesRequest struct {
	ModelIDs *[]string `json:"model_ids"`
}

type mediaModelScopesResponse struct {
	ModelIDs []string `json:"model_ids"`
}

type mediaAdapterPreflightItemResponse struct {
	ModelID                 string                               `json:"model_id"`
	Enabled                 bool                                 `json:"enabled"`
	ResolutionStatus        service.MediaAdapterResolutionStatus `json:"resolution_status"`
	ResolvedAdapter         string                               `json:"resolved_adapter"`
	LegacyDefaultAdapter    string                               `json:"legacy_default_adapter"`
	LegacyCheckApplicable   bool                                 `json:"legacy_check_applicable"`
	AdapterKeyMatches       bool                                 `json:"adapter_key_matches"`
	LegacyDefaultAsyncMode  service.NativeAsyncMode              `json:"legacy_default_async_mode"`
	LegacyAsyncModeReadable bool                                 `json:"legacy_async_mode_readable"`
	ReasonCode              string                               `json:"reason_code"`
	RolloutSafe             bool                                 `json:"rollout_safe"`
}

type mediaAdapterPreflightResponse struct {
	Safe          bool                                `json:"safe"`
	BlockingCount int                                 `json:"blocking_count"`
	Items         []mediaAdapterPreflightItemResponse `json:"items"`
}

const mediaRequestMappingPreviewMaxBytes = 1 << 20

type mediaRequestMappingPreviewRequest struct {
	Request *map[string]any              `json:"request"`
	Mapping *service.MediaRequestMapping `json:"mapping"`
}

type mediaRequestMappingPreviewResponse struct {
	Result map[string]any `json:"result"`
}

// List returns all enabled and disabled global media model definitions.
// GET /api/v1/admin/media-models
func (h *MediaModelAdminHandler) List(c *gin.Context) {
	items, err := h.service.List(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	output := make([]mediaModelResponse, 0, len(items))
	for _, item := range items {
		output = append(output, mediaModelRecordToResponse(item))
	}
	response.Success(c, mediaModelListResponse{Items: output})
}

// Preflight reports whether persisted media models are safe for a rolling
// migration to code-owned adapter resolution.
// GET /api/v1/admin/media-models/preflight
func (h *MediaModelAdminHandler) Preflight(c *gin.Context) {
	report, err := h.service.Preflight(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, mediaAdapterPreflightResponseFromService(report))
}

// PreviewRequestMapping applies a declarative account request mapping to a
// unified downstream request sample without persisting either value.
// POST /api/v1/admin/media-models/request-mapping-preview
func (h *MediaModelAdminHandler) PreviewRequestMapping(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, mediaRequestMappingPreviewMaxBytes)
	var req mediaRequestMappingPreviewRequest
	if err := decodeStrictAdminJSON(c, &req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if req.Request == nil {
		response.BadRequest(c, "Invalid request: request is required and must be an object")
		return
	}
	if req.Mapping == nil {
		response.BadRequest(c, "Invalid request: mapping is required and must be an object")
		return
	}
	result, err := req.Mapping.Apply(*req.Request)
	if err != nil {
		response.BadRequest(c, "Invalid request mapping preview: "+err.Error())
		return
	}
	response.Success(c, mediaRequestMappingPreviewResponse{Result: result})
}

// GetByID returns one global media model definition.
// GET /api/v1/admin/media-models/:id
func (h *MediaModelAdminHandler) GetByID(c *gin.Context) {
	id, ok := parseMediaModelPathID(c, "media model")
	if !ok {
		return
	}
	item, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, mediaModelRecordToResponse(*item))
}

// Create atomically creates a definition and all aliases.
// POST /api/v1/admin/media-models
func (h *MediaModelAdminHandler) Create(c *gin.Context) {
	req, ok := decodeMediaModelWriteRequest(c)
	if !ok {
		return
	}
	created, err := h.service.Create(c.Request.Context(), req.toServiceRecord())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, mediaModelRecordToResponse(*created))
}

// Update fully replaces a definition and its aliases.
// PUT /api/v1/admin/media-models/:id
func (h *MediaModelAdminHandler) Update(c *gin.Context) {
	id, ok := parseMediaModelPathID(c, "media model")
	if !ok {
		return
	}
	req, ok := decodeMediaModelWriteRequest(c)
	if !ok {
		return
	}
	updated, err := h.service.Update(c.Request.Context(), id, req.toServiceRecord())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, mediaModelRecordToResponse(*updated))
}

// Delete removes a definition; database cascades remove its aliases and group scopes.
// DELETE /api/v1/admin/media-models/:id
func (h *MediaModelAdminHandler) Delete(c *gin.Context) {
	id, ok := parseMediaModelPathID(c, "media model")
	if !ok {
		return
	}
	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, nil)
}

// GetGroupScopes returns the canonical public model ID whitelist for a media group.
// GET /api/v1/admin/groups/:id/media-model-scopes
func (h *MediaModelAdminHandler) GetGroupScopes(c *gin.Context) {
	groupID, ok := parseMediaModelPathID(c, "group")
	if !ok {
		return
	}
	modelIDs, err := h.service.GetGroupScopes(c.Request.Context(), groupID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, mediaModelScopesResponse{ModelIDs: modelIDs})
}

// ReplaceGroupScopes atomically replaces the whitelist for a media group.
// PUT /api/v1/admin/groups/:id/media-model-scopes
func (h *MediaModelAdminHandler) ReplaceGroupScopes(c *gin.Context) {
	groupID, ok := parseMediaModelPathID(c, "group")
	if !ok {
		return
	}
	var req mediaModelScopesRequest
	if err := decodeStrictAdminJSON(c, &req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if req.ModelIDs == nil {
		response.BadRequest(c, "Invalid request: model_ids is required")
		return
	}
	modelIDs, err := h.service.ReplaceGroupScopes(c.Request.Context(), groupID, *req.ModelIDs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, mediaModelScopesResponse{ModelIDs: modelIDs})
}

func decodeMediaModelWriteRequest(c *gin.Context) (mediaModelWriteRequest, bool) {
	var req mediaModelWriteRequest
	if err := decodeStrictAdminJSON(c, &req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return mediaModelWriteRequest{}, false
	}
	if req.Enabled == nil {
		response.BadRequest(c, "Invalid request: enabled is required")
		return mediaModelWriteRequest{}, false
	}
	if len(req.Constraints) == 0 {
		response.BadRequest(c, "Invalid request: constraints is required")
		return mediaModelWriteRequest{}, false
	}
	return req, true
}

func decodeStrictAdminJSON(c *gin.Context, target any) error {
	if c == nil || c.Request == nil || c.Request.Body == nil {
		return fmt.Errorf("request body is required")
	}
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("request body must contain one JSON object")
		}
		return err
	}
	return nil
}

func parseMediaModelPathID(c *gin.Context, resource string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid "+resource+" ID")
		return 0, false
	}
	return id, true
}

func (r mediaModelWriteRequest) toServiceRecord() service.MediaModelAdminRecord {
	enabled := false
	if r.Enabled != nil {
		enabled = *r.Enabled
	}
	return service.MediaModelAdminRecord{
		Definition: service.MediaModelDefinition{
			ModelID:     r.ModelID,
			Vendor:      r.Vendor,
			MediaType:   r.MediaType,
			Operations:  append([]service.MediaOperation(nil), r.Operations...),
			Constraints: append(json.RawMessage(nil), r.Constraints...),
			BillingUnit: r.BillingUnit,
			Enabled:     enabled,
		},
		Aliases: append([]string(nil), r.Aliases...),
	}
}

func mediaModelRecordToResponse(record service.MediaModelAdminRecord) mediaModelResponse {
	definition := record.Definition
	operations := append([]service.MediaOperation(nil), definition.Operations...)
	if operations == nil {
		operations = []service.MediaOperation{}
	}
	aliases := append([]string(nil), record.Aliases...)
	if aliases == nil {
		aliases = []string{}
	}
	constraints := append(json.RawMessage(nil), definition.Constraints...)
	if len(constraints) == 0 {
		constraints = json.RawMessage(`{}`)
	}
	return mediaModelResponse{
		ID:                definition.ID,
		ModelID:           definition.ModelID,
		Vendor:            definition.Vendor,
		MediaType:         definition.MediaType,
		Operations:        operations,
		Constraints:       constraints,
		BillingUnit:       definition.BillingUnit,
		DefaultAdapter:    record.AdapterResolution.ResolvedAdapter,
		DefaultAsyncMode:  record.AdapterResolution.CompatibilityAsyncMode(),
		AdapterResolution: mediaAdapterResolutionToResponse(record.AdapterResolution),
		Enabled:           definition.Enabled,
		Aliases:           aliases,
		CreatedAt:         definition.CreatedAt,
		UpdatedAt:         definition.UpdatedAt,
	}
}

func mediaAdapterResolutionToResponse(resolution service.MediaAdapterResolution) mediaAdapterResolutionResponse {
	response := mediaAdapterResolutionResponse{
		Status:          resolution.Status,
		ResolvedAdapter: resolution.ResolvedAdapter,
		MatchedBy:       resolution.MatchedBy,
		MatchedFamily:   resolution.MatchedFamily,
		ReasonCode:      resolution.ReasonCode,
	}
	if resolution.Capabilities == nil {
		return response
	}
	operations := append([]service.MediaOperation(nil), resolution.Capabilities.Operations...)
	if operations == nil {
		operations = []service.MediaOperation{}
	}
	response.Capabilities = &mediaAdapterCapabilitiesResponse{
		Operations:          operations,
		SyncUpstream:        resolution.Capabilities.SyncUpstream,
		NativeAsyncUpstream: resolution.Capabilities.NativeAsyncUpstream,
		ContentFetch:        resolution.Capabilities.ContentFetch,
	}
	return response
}

func mediaAdapterPreflightResponseFromService(report *service.MediaAdapterPreflightReport) mediaAdapterPreflightResponse {
	response := mediaAdapterPreflightResponse{Items: []mediaAdapterPreflightItemResponse{}}
	if report == nil {
		return response
	}
	response.Safe = report.Safe
	response.BlockingCount = report.BlockingCount
	response.Items = make([]mediaAdapterPreflightItemResponse, 0, len(report.Items))
	for _, item := range report.Items {
		response.Items = append(response.Items, mediaAdapterPreflightItemResponse{
			ModelID:                 item.ModelID,
			Enabled:                 item.Enabled,
			ResolutionStatus:        item.Status,
			ResolvedAdapter:         item.ResolvedAdapter,
			LegacyDefaultAdapter:    item.LegacyDefaultAdapter,
			LegacyCheckApplicable:   item.LegacyCheckApplicable,
			AdapterKeyMatches:       item.AdapterKeyMatches,
			LegacyDefaultAsyncMode:  item.LegacyDefaultAsyncMode,
			LegacyAsyncModeReadable: item.LegacyAsyncModeReadable,
			ReasonCode:              item.ReasonCode,
			RolloutSafe:             item.RolloutSafe,
		})
	}
	return response
}
