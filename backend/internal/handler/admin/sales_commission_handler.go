package admin

import (
	"strconv"

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

func (h *SalesCommissionHandler) ListSummaries(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	items, total, err := h.service.ListSummaries(c.Request.Context(), service.SalesCommissionSummaryListParams{
		Search:   c.Query("search"),
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, int64(total), page, pageSize)
}

func (h *SalesCommissionHandler) ListRecords(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	items, total, err := h.service.ListRecords(c.Request.Context(), service.SalesCommissionRecordListParams{
		SalesUserID:    parseInt64Query(c, "sales_user_id"),
		RefereeUserID:  parseInt64Query(c, "referee_user_id"),
		PaymentOrderID: parseInt64Query(c, "payment_order_id"),
		Status:         c.Query("status"),
		Page:           page,
		PageSize:       pageSize,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, int64(total), page, pageSize)
}

func (h *SalesCommissionHandler) ListSettlements(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	items, total, err := h.service.ListSettlements(c.Request.Context(), service.SalesCommissionSettlementListParams{
		SalesUserID: parseInt64Query(c, "sales_user_id"),
		Page:        page,
		PageSize:    pageSize,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, int64(total), page, pageSize)
}

type createSalesCommissionSettlementRequest struct {
	SalesUserID int64   `json:"sales_user_id"`
	AmountCNY   float64 `json:"amount_cny"`
	Note        string  `json:"note"`
}

func (h *SalesCommissionHandler) CreateSettlement(c *gin.Context) {
	var req createSalesCommissionSettlementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if req.SalesUserID <= 0 {
		response.BadRequest(c, "sales_user_id is required")
		return
	}
	if req.AmountCNY <= 0 {
		response.BadRequest(c, "amount_cny must be greater than 0")
		return
	}
	result, err := h.service.CreateSettlement(c.Request.Context(), &service.SalesCommissionSettlementCreate{
		SalesUserID: req.SalesUserID,
		AmountCNY:   req.AmountCNY,
		Note:        req.Note,
		CreatedBy:   parseAdminUserID(c),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func parseInt64Query(c *gin.Context, key string) int64 {
	v, _ := strconv.ParseInt(c.Query(key), 10, 64)
	return v
}

func parseAdminUserID(c *gin.Context) *int64 {
	v, ok := c.Get("user_id")
	if !ok {
		return nil
	}
	switch id := v.(type) {
	case int64:
		return &id
	case int:
		x := int64(id)
		return &x
	default:
		return nil
	}
}
