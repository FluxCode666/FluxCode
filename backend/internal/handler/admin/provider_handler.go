package admin

import (
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type ProviderHandler struct {
	service     *service.ProviderService
	diagnostics *service.ProviderGatewayService
}

func NewProviderHandler(providerService *service.ProviderService, diagnostics *service.ProviderGatewayService) *ProviderHandler {
	return &ProviderHandler{service: providerService, diagnostics: diagnostics}
}

type providerRouteAttemptResponse struct {
	TraceID           string                              `json:"trace_id"`
	RouteIdentity     string                              `json:"route_identity"`
	GroupID           int64                               `json:"group_id"`
	ProviderID        int64                               `json:"provider_id"`
	CapabilityID      int64                               `json:"capability_id"`
	EndpointID        int64                               `json:"endpoint_id"`
	LogicalModel      string                              `json:"logical_model"`
	UpstreamModel     string                              `json:"upstream_model"`
	IngressProtocol   service.ProtocolFamily              `json:"ingress_protocol"`
	UpstreamProtocol  service.ProtocolFamily              `json:"upstream_protocol"`
	Tier              service.RouteTier                   `json:"tier"`
	Outcome           service.ProviderRouteAttemptOutcome `json:"outcome"`
	StatusCode        int                                 `json:"status_code"`
	FailureCategory   string                              `json:"failure_category"`
	UpstreamRequestID string                              `json:"upstream_request_id"`
	WireProfile       service.WireProfile                 `json:"wire_profile"`
	ConversionUsed    bool                                `json:"conversion_used"`
	BytesCommitted    int64                               `json:"bytes_committed"`
	FinalReason       string                              `json:"final_reason"`
	StartedAt         time.Time                           `json:"started_at"`
	DurationMs        int64                               `json:"duration_ms"`
}

func (h *ProviderHandler) ListRouteAttempts(c *gin.Context) {
	filter := service.ProviderRouteAttemptFilter{
		GroupID: parsePositiveQueryInt(c.Query("group_id")), ProviderID: parsePositiveQueryInt(c.Query("provider_id")),
		LogicalModel: c.Query("logical_model"), IngressProtocol: service.ProtocolFamily(c.Query("ingress_protocol")),
		UpstreamProtocol: service.ProtocolFamily(c.Query("upstream_protocol")), Outcome: service.ProviderRouteAttemptOutcome(c.Query("outcome")),
		Tier: service.RouteTier(c.Query("tier")), Limit: int(parsePositiveQueryInt(c.Query("limit"))),
	}
	items, err := h.diagnostics.ListRouteAttempts(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	result := make([]providerRouteAttemptResponse, 0, len(items))
	for _, item := range items {
		result = append(result, providerRouteAttemptResponse{
			TraceID: item.TraceID, RouteIdentity: item.RouteIdentity, GroupID: item.GroupID,
			ProviderID: item.Route.ProviderID, CapabilityID: item.Route.CapabilityID, EndpointID: item.Route.EndpointID,
			LogicalModel: item.LogicalModel, UpstreamModel: item.UpstreamModel,
			IngressProtocol: item.IngressProtocol, UpstreamProtocol: item.UpstreamProtocol,
			Tier: item.Tier, Outcome: item.Outcome, StatusCode: item.StatusCode,
			FailureCategory: item.FailureCategory, UpstreamRequestID: item.UpstreamRequestID,
			WireProfile: item.WireProfile, ConversionUsed: item.ConversionUsed, BytesCommitted: item.BytesCommitted,
			FinalReason: item.FinalReason, StartedAt: item.StartedAt, DurationMs: item.Duration.Milliseconds(),
		})
	}
	response.Success(c, result)
}

func parsePositiveQueryInt(raw string) int64 {
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0
	}
	return value
}

func (h *ProviderHandler) List(c *gin.Context) {
	items, err := h.service.List(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.ProvidersFromService(items))
}

func (h *ProviderHandler) GetByID(c *gin.Context) {
	id, ok := providerIDParam(c)
	if !ok {
		return
	}
	item, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.ProviderFromService(item))
}

func (h *ProviderHandler) Create(c *gin.Context) {
	var request dto.ProviderWriteRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, "无效的供应商配置："+err.Error())
		return
	}
	item, err := h.service.Create(c.Request.Context(), request.ServiceInput())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, dto.ProviderFromService(item))
}

func (h *ProviderHandler) Update(c *gin.Context) {
	id, ok := providerIDParam(c)
	if !ok {
		return
	}
	var request dto.ProviderWriteRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, "无效的供应商配置："+err.Error())
		return
	}
	item, err := h.service.Update(c.Request.Context(), id, request.ServiceInput())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.ProviderFromService(item))
}

type providerTestRequest struct {
	CapabilityID int64                  `json:"capability_id"`
	Protocol     service.ProtocolFamily `json:"protocol"`
	LogicalModel string                 `json:"logical_model"`
}

func (h *ProviderHandler) Test(c *gin.Context) {
	id, ok := providerIDParam(c)
	if !ok {
		return
	}
	var request providerTestRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, "无效的能力测试请求："+err.Error())
		return
	}
	result, err := h.service.TestCapability(c.Request.Context(), id, service.ProviderCapabilityTestInput{
		CapabilityID: request.CapabilityID, Protocol: request.Protocol, LogicalModel: request.LogicalModel,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.ProviderCapabilityTestFromService(result))
}

type providerVersionRequest struct {
	Version int64 `json:"version" binding:"required"`
}

func (h *ProviderHandler) Activate(c *gin.Context) {
	h.setStatus(c, true)
}

func (h *ProviderHandler) Disable(c *gin.Context) {
	h.setStatus(c, false)
}

func (h *ProviderHandler) setStatus(c *gin.Context, activate bool) {
	id, ok := providerIDParam(c)
	if !ok {
		return
	}
	var request providerVersionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, "version 为必填项")
		return
	}
	var item *service.ProviderAggregate
	var err error
	if activate {
		item, err = h.service.Activate(c.Request.Context(), id, request.Version)
	} else {
		item, err = h.service.Disable(c.Request.Context(), id, request.Version)
	}
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.ProviderFromService(item))
}

func (h *ProviderHandler) ListGroupCapabilities(c *gin.Context) {
	id, ok := providerIDParam(c)
	if !ok {
		return
	}
	items, err := h.service.ListGroupCapabilities(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}

type groupRouteSnapshotResponse struct {
	ID         int64          `json:"id"`
	GroupID    int64          `json:"group_id"`
	Version    int64          `json:"version"`
	Status     string         `json:"status"`
	Manifest   map[string]any `json:"manifest"`
	ShadowDiff map[string]any `json:"shadow_diff"`
	ApprovedBy *int64         `json:"approved_by"`
	ApprovedAt *time.Time     `json:"approved_at"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

func (h *ProviderHandler) ListGroupRouteSnapshots(c *gin.Context) {
	groupID, ok := providerIDParam(c)
	if !ok {
		return
	}
	items, err := h.service.ListGroupRouteSnapshots(c.Request.Context(), groupID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	result := make([]groupRouteSnapshotResponse, 0, len(items))
	for _, item := range items {
		result = append(result, routeSnapshotResponse(item))
	}
	response.Success(c, result)
}

func (h *ProviderHandler) CreateGroupShadowSnapshot(c *gin.Context) {
	groupID, ok := providerIDParam(c)
	if !ok {
		return
	}
	item, err := h.service.CreateGroupShadowSnapshot(c.Request.Context(), groupID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, routeSnapshotResponse(*item))
}

func (h *ProviderHandler) ApproveGroupRouteSnapshot(c *gin.Context) {
	groupID, ok := providerIDParam(c)
	if !ok {
		return
	}
	version, ok := positiveParam(c, "version")
	if !ok {
		return
	}
	reviewerID := int64(0)
	if subject, exists := servermiddleware.GetAuthSubjectFromContext(c); exists {
		reviewerID = subject.UserID
	}
	item, err := h.service.ApproveGroupRouteSnapshot(c.Request.Context(), groupID, version, reviewerID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, routeSnapshotResponse(*item))
}

func (h *ProviderHandler) ActivateGroupRouteSnapshot(c *gin.Context) {
	groupID, ok := providerIDParam(c)
	if !ok {
		return
	}
	version, ok := positiveParam(c, "version")
	if !ok {
		return
	}
	result, err := h.service.ActivateGroupRouteSnapshot(c.Request.Context(), groupID, version)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"group_id": result.GroupID, "active_version": result.ActiveVersion, "previous_version": result.PreviousVersion})
}

func (h *ProviderHandler) RollbackGroupRouteSnapshot(c *gin.Context) {
	groupID, ok := providerIDParam(c)
	if !ok {
		return
	}
	result, err := h.service.RollbackGroupRouteSnapshot(c.Request.Context(), groupID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"group_id": result.GroupID, "active_version": result.ActiveVersion, "previous_version": result.PreviousVersion})
}

func routeSnapshotResponse(item service.GroupRouteSnapshot) groupRouteSnapshotResponse {
	return groupRouteSnapshotResponse{ID: item.ID, GroupID: item.GroupID, Version: item.Version, Status: item.Status, Manifest: item.Manifest, ShadowDiff: item.ShadowDiff, ApprovedBy: item.ApprovedBy, ApprovedAt: item.ApprovedAt, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt}
}

func positiveParam(c *gin.Context, name string) (int64, bool) {
	value, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || value <= 0 {
		response.BadRequest(c, "无效的 "+name)
		return 0, false
	}
	return value, true
}

func providerIDParam(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "无效的 ID")
		return 0, false
	}
	return id, true
}
