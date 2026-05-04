package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type SalesCommissionHandler struct {
	service *service.SalesCommissionService
}

func NewSalesCommissionHandler(svc *service.SalesCommissionService) *SalesCommissionHandler {
	return &SalesCommissionHandler{service: svc}
}

func (h *SalesCommissionHandler) GetSummary(c *gin.Context) {
	userID, ok := currentSalesCommissionUserID(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
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
	userID, ok := currentSalesCommissionUserID(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
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

func currentSalesCommissionUserID(c *gin.Context) (int64, bool) {
	v, ok := c.Get("user_id")
	if !ok {
		return 0, false
	}
	switch id := v.(type) {
	case int64:
		return id, true
	case int:
		return int64(id), true
	default:
		return 0, false
	}
}
