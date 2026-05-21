package admin

import (
	"strconv"
	"time"

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

// GetOverview 返回 admin 端销售佣金数据看板（spec §15.3）。
//
// query string:
//   - range: today|this_week|this_month|this_quarter|this_year|last_30d|last_90d|custom，默认 this_month
//   - start, end: 当 range=custom 时必填，格式 YYYY-MM-DD（按 Asia/Shanghai 解析）
func (h *SalesCommissionHandler) GetOverview(c *gin.Context) {
	params := service.SalesCommissionOverviewParams{
		RangeKey: c.Query("range"),
	}
	if v := c.Query("start"); v != "" {
		t, err := parseSalesCommissionDate(v)
		if err != nil {
			response.BadRequest(c, "invalid start date: "+err.Error())
			return
		}
		params.Start = &t
	}
	if v := c.Query("end"); v != "" {
		t, err := parseSalesCommissionDate(v)
		if err != nil {
			response.BadRequest(c, "invalid end date: "+err.Error())
			return
		}
		params.End = &t
	}
	overview, err := h.service.GetOverview(c.Request.Context(), params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, overview)
}

var salesCommissionDashboardLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

func parseSalesCommissionDate(s string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02", s, salesCommissionDashboardLocation)
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
