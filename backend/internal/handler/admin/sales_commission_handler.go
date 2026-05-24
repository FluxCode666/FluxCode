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
	// sort_order 仅接受 "asc" / "desc"，其余值（包括空）会被 service 归一化为 "desc"（默认倒序）。
	items, total, err := h.service.ListRecords(c.Request.Context(), service.SalesCommissionRecordListParams{
		SalesUserID:    parseInt64Query(c, "sales_user_id"),
		RefereeUserID:  parseInt64Query(c, "referee_user_id"),
		PaymentOrderID: parseInt64Query(c, "payment_order_id"),
		Status:         c.Query("status"),
		SortOrder:      c.Query("sort_order"),
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

type createSettlementRequest struct {
	SalesUserID int64   `json:"sales_user_id" binding:"required,min=1"`
	AmountCNY   float64 `json:"amount_cny" binding:"required,gt=0"`
	Note        string  `json:"note"`
}

// CreateSettlement 为指定销售用户创建一笔结算记录，并将其可结算佣金标记为已结算。
// POST /admin/sales-commissions/settlements
func (h *SalesCommissionHandler) CreateSettlement(c *gin.Context) {
	var req createSettlementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	adminID := parseAdminUserID(c)

	settlement, err := h.service.CreateSettlement(c.Request.Context(), &service.SalesCommissionSettlementCreate{
		SalesUserID: req.SalesUserID,
		AmountCNY:   req.AmountCNY,
		Note:        req.Note,
		CreatedBy:   adminID,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, settlement)
}

// RecomputeMissingRequest 是 POST /admin/sales-commissions/recompute 的可选请求体。
//
//   - Limit: 单次重算扫描的最大候选订单数。<=0 时 service 用默认值（500）；上限 2000。
//
// 前端按钮场景下通常省略 body，让后端用默认值即可。
type RecomputeMissingRequest struct {
	Limit int `json:"limit,omitempty"`
}

// RecomputeMissing 兜底重算「应当存在但目前缺失」的销售佣金记录（见 service 同名方法注释）。
//
// 幂等：sales_commission_records.payment_order_id 上有 partial unique 索引 +
// CreateForOrder 用 ON CONFLICT DO NOTHING，重复点击安全。
//
// POST /admin/sales-commissions/recompute
// Body: 可选 { "limit": int }
func (h *SalesCommissionHandler) RecomputeMissing(c *gin.Context) {
	var req RecomputeMissingRequest
	// Body 是可选的，无 body 时直接走默认 limit。
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			response.BadRequest(c, "Invalid request: "+err.Error())
			return
		}
	}
	res, err := h.service.RecomputeMissingCommissions(c.Request.Context(), req.Limit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, res)
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
