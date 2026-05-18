package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type SalesCommissionHandler struct {
	service *service.SalesCommissionService
}

func NewSalesCommissionHandler(svc *service.SalesCommissionService) *SalesCommissionHandler {
	return &SalesCommissionHandler{service: svc}
}

func (h *SalesCommissionHandler) GetSummary(c *gin.Context) {
	userID, ok := h.authorizedSalesCommissionUserID(c)
	if !ok {
		return
	}
	summary, err := h.service.GetSummaryBySalesUser(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, summary)
}

func (h *SalesCommissionHandler) ListRecords(c *gin.Context) {
	userID, ok := h.authorizedSalesCommissionUserID(c)
	if !ok {
		return
	}
	page, pageSize := response.ParsePagination(c)
	items, total, err := h.service.ListRecords(c.Request.Context(), service.SalesCommissionRecordListParams{
		SalesUserID: userID,
		Status:      c.Query("status"),
		Page:        page,
		PageSize:    pageSize,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, int64(total), page, pageSize)
}

func (h *SalesCommissionHandler) authorizedSalesCommissionUserID(c *gin.Context) (int64, bool) {
	userID, ok := currentSalesCommissionUserID(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return 0, false
	}
	allowed, err := h.service.IsSalesUser(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return 0, false
	}
	if !allowed {
		response.Forbidden(c, "Sales access required")
		return 0, false
	}
	return userID, true
}

func currentSalesCommissionUserID(c *gin.Context) (int64, bool) {
	sub, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		return 0, false
	}
	return sub.UserID, true
}
