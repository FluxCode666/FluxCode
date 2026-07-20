package admin

import (
	"encoding/json"
	"fmt"
	"io"
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
	ModelID          string                   `json:"model_id"`
	Vendor           string                   `json:"vendor"`
	MediaType        service.MediaType        `json:"media_type"`
	Operations       []service.MediaOperation `json:"operations"`
	Constraints      json.RawMessage          `json:"constraints"`
	BillingUnit      string                   `json:"billing_unit"`
	DefaultAdapter   string                   `json:"default_adapter"`
	DefaultAsyncMode service.NativeAsyncMode  `json:"default_async_mode"`
	Enabled          *bool                    `json:"enabled"`
	Aliases          []string                 `json:"aliases"`
}

type mediaModelResponse struct {
	ID               int64                    `json:"id"`
	ModelID          string                   `json:"model_id"`
	Vendor           string                   `json:"vendor"`
	MediaType        service.MediaType        `json:"media_type"`
	Operations       []service.MediaOperation `json:"operations"`
	Constraints      json.RawMessage          `json:"constraints"`
	BillingUnit      string                   `json:"billing_unit"`
	DefaultAdapter   string                   `json:"default_adapter"`
	DefaultAsyncMode service.NativeAsyncMode  `json:"default_async_mode"`
	Enabled          bool                     `json:"enabled"`
	Aliases          []string                 `json:"aliases"`
	CreatedAt        time.Time                `json:"created_at"`
	UpdatedAt        time.Time                `json:"updated_at"`
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
			ModelID:          r.ModelID,
			Vendor:           r.Vendor,
			MediaType:        r.MediaType,
			Operations:       append([]service.MediaOperation(nil), r.Operations...),
			Constraints:      append(json.RawMessage(nil), r.Constraints...),
			BillingUnit:      r.BillingUnit,
			DefaultAdapter:   r.DefaultAdapter,
			DefaultAsyncMode: r.DefaultAsyncMode,
			Enabled:          enabled,
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
		ID:               definition.ID,
		ModelID:          definition.ModelID,
		Vendor:           definition.Vendor,
		MediaType:        definition.MediaType,
		Operations:       operations,
		Constraints:      constraints,
		BillingUnit:      definition.BillingUnit,
		DefaultAdapter:   definition.DefaultAdapter,
		DefaultAsyncMode: definition.DefaultAsyncMode,
		Enabled:          definition.Enabled,
		Aliases:          aliases,
		CreatedAt:        definition.CreatedAt,
		UpdatedAt:        definition.UpdatedAt,
	}
}
