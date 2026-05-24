package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
)

var salesCommissionMonthLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

type salesCommissionUserRepository interface {
	GetByID(ctx context.Context, id int64) (*User, error)
}

type SalesCommissionService struct {
	repo         SalesCommissionRepository
	referralRepo ReferralRepository
	userRepo     salesCommissionUserRepository
	nowFn        func() time.Time
}

func NewSalesCommissionService(repo SalesCommissionRepository, referralRepo ReferralRepository, userRepo salesCommissionUserRepository) *SalesCommissionService {
	return &SalesCommissionService{
		repo:         repo,
		referralRepo: referralRepo,
		userRepo:     userRepo,
		nowFn:        func() time.Time { return time.Now().UTC() },
	}
}

// SetNowFunc 仅用于测试，注入可控的当前时间函数。
func (s *SalesCommissionService) SetNowFunc(fn func() time.Time) {
	if s == nil {
		return
	}
	s.nowFn = fn
}

func (s *SalesCommissionService) now() time.Time {
	if s == nil || s.nowFn == nil {
		return time.Now().UTC()
	}
	return s.nowFn()
}

func (s *SalesCommissionService) HandleBalanceRechargeCompleted(ctx context.Context, order *dbent.PaymentOrder) error {
	if s == nil || s.repo == nil || s.referralRepo == nil || s.userRepo == nil || order == nil {
		slog.Warn("[SalesCommission] handle skipped: nil dependency", "serviceNil", s == nil, "orderNil", order == nil)
		return nil
	}
	slog.Info("[SalesCommission] handle entry",
		"orderID", order.ID, "userID", order.UserID,
		"orderType", order.OrderType, "status", order.Status,
		"payAmount", order.PayAmount, "amount", order.Amount)
	if order.OrderType != payment.OrderTypeBalance ||
		order.Status != payment.OrderStatusCompleted ||
		order.PayAmount <= 0 ||
		order.Amount <= 0 {
		slog.Info("[SalesCommission] skip: order does not qualify",
			"orderID", order.ID, "orderType", order.OrderType, "status", order.Status,
			"payAmount", order.PayAmount, "amount", order.Amount)
		return nil
	}

	ref, err := s.referralRepo.GetByRefereeID(ctx, order.UserID)
	if err != nil {
		slog.Warn("[SalesCommission] referral lookup error", "orderID", order.ID, "userID", order.UserID, "error", err)
		return err
	}
	if ref == nil {
		slog.Info("[SalesCommission] skip: no referral for referee", "orderID", order.ID, "refereeUserID", order.UserID)
		return nil
	}

	referrer, err := s.userRepo.GetByID(ctx, ref.ReferrerID)
	if err != nil {
		slog.Warn("[SalesCommission] referrer lookup error", "orderID", order.ID, "referrerID", ref.ReferrerID, "error", err)
		return err
	}
	if !SalesCommissionUserEligible(referrer) {
		mode := ""
		var rate float64
		tiers := 0
		var isSales bool
		if referrer != nil {
			isSales = referrer.IsSales
			mode = string(NormalizeSalesCommissionMode(referrer.SalesCommissionMode))
			rate = referrer.SalesCommissionRate
			tiers = len(referrer.SalesCommissionTiers)
		}
		slog.Info("[SalesCommission] skip: referrer not eligible (treated as normal referral)",
			"orderID", order.ID, "referrerID", ref.ReferrerID,
			"isSales", isSales, "mode", mode, "rate", rate, "tiersCount", tiers)
		return nil
	}

	eventAt := salesCommissionEventTimeFromOrder(order)

	slog.Info("[SalesCommission] dispatch CreateForOrder",
		"orderID", order.ID, "salesUserID", ref.ReferrerID, "refereeUserID", order.UserID,
		"mode", string(NormalizeSalesCommissionMode(referrer.SalesCommissionMode)),
		"tiersCount", len(referrer.SalesCommissionTiers),
		"eventAt", eventAt)
	if err := s.repo.CreateForOrder(ctx, &SalesCommissionCreate{
		SalesUserID:               ref.ReferrerID,
		RefereeUserID:             order.UserID,
		ReferralID:                ref.ID,
		PaymentOrderID:            &order.ID,
		OrderPayAmountCNY:         order.PayAmount,
		OrderCreditedAmount:       order.Amount,
		CommissionMode:            NormalizeSalesCommissionMode(referrer.SalesCommissionMode),
		CommissionRate:            referrer.SalesCommissionRate,
		CommissionMinMonthlySales: referrer.SalesCommissionMinMonthlySales,
		CommissionTiers:           CloneSalesCommissionTiers(referrer.SalesCommissionTiers),
		CommissionEventAt:         eventAt,
		CommissionMonth:           salesCommissionMonthStart(eventAt),
		Note:                      "Balance recharge commission",
	}); err != nil {
		slog.Error("[SalesCommission] CreateForOrder failed", "orderID", order.ID, "salesUserID", ref.ReferrerID, "error", err)
		return err
	}
	slog.Info("[SalesCommission] CreateForOrder ok", "orderID", order.ID, "salesUserID", ref.ReferrerID)
	return nil
}

func (s *SalesCommissionService) HandleReferralManualCompletion(ctx context.Context, ref *Referral, orderPayAmountCNY float64, orderCreditedAmount float64, note string) error {
	if s == nil || s.repo == nil || s.userRepo == nil || ref == nil {
		return nil
	}
	if orderPayAmountCNY <= 0 || orderCreditedAmount <= 0 {
		return nil
	}

	referrer, err := s.userRepo.GetByID(ctx, ref.ReferrerID)
	if err != nil {
		return err
	}
	if !SalesCommissionUserEligible(referrer) {
		return nil
	}

	eventAt := time.Now().UTC()

	return s.repo.CreateForOrder(ctx, &SalesCommissionCreate{
		SalesUserID:               ref.ReferrerID,
		RefereeUserID:             ref.RefereeID,
		ReferralID:                ref.ID,
		PaymentOrderID:            nil,
		OrderPayAmountCNY:         orderPayAmountCNY,
		OrderCreditedAmount:       orderCreditedAmount,
		CommissionMode:            NormalizeSalesCommissionMode(referrer.SalesCommissionMode),
		CommissionRate:            referrer.SalesCommissionRate,
		CommissionMinMonthlySales: referrer.SalesCommissionMinMonthlySales,
		CommissionTiers:           CloneSalesCommissionTiers(referrer.SalesCommissionTiers),
		CommissionEventAt:         eventAt,
		CommissionMonth:           salesCommissionMonthStart(eventAt),
		Note:                      "Referral manual completion: " + note,
	})
}

// salesCommissionRecomputeDefaultLimit 控制"重算缺失佣金"单次调用最多扫多少条候选订单。
//
// 这个值在前端按钮单次点击就能完成的体感上限（几秒内）和 PG 单次查询健康范围之间取折中：
// 实际生产环境历史数据通常 < 1000 条，调一次就清空；运维需要补更多时多点几下即可。
const salesCommissionRecomputeDefaultLimit = 500
const salesCommissionRecomputeMaxLimit = 2000

func (s *SalesCommissionService) RecomputeMissingCommissions(ctx context.Context, limit int) (*SalesCommissionRecomputeResult, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("sales commission service not initialized")
	}
	limit = normalizeRecomputeLimit(limit)

	orders, err := s.repo.ListMissingCommissionPaymentOrders(ctx, limit)
	if err != nil {
		slog.Error("[SalesCommission] recompute: list missing failed", "error", err, "limit", limit)
		return nil, err
	}

	res := &SalesCommissionRecomputeResult{Scanned: len(orders)}
	if len(orders) == 0 {
		slog.Info("[SalesCommission] recompute: no missing commissions found", "limit", limit)
		return res, nil
	}

	slog.Info("[SalesCommission] recompute: start", "candidates", len(orders), "limit", limit)
	for _, order := range orders {
		if order == nil {
			continue
		}
		if err := s.HandleBalanceRechargeCompleted(ctx, order); err != nil {
			res.Failed++
			res.FailedOrderIDs = append(res.FailedOrderIDs, order.ID)
			slog.Warn("[SalesCommission] recompute: order failed",
				"orderID", order.ID, "userID", order.UserID, "error", err)
			continue
		}
		res.Processed++
	}
	slog.Info("[SalesCommission] recompute: done",
		"scanned", res.Scanned, "processed", res.Processed,
		"failed", res.Failed, "failedOrderIDs", res.FailedOrderIDs)
	return res, nil
}

func normalizeRecomputeLimit(limit int) int {
	if limit <= 0 {
		return salesCommissionRecomputeDefaultLimit
	}
	if limit > salesCommissionRecomputeMaxLimit {
		return salesCommissionRecomputeMaxLimit
	}
	return limit
}

func (s *SalesCommissionService) ListSummaries(ctx context.Context, params SalesCommissionSummaryListParams) ([]SalesCommissionSummary, int, error) {
	return s.repo.ListSummaries(ctx, params)
}

func (s *SalesCommissionService) GetSummaryBySalesUser(ctx context.Context, salesUserID int64) (*SalesCommissionSummary, error) {
	return s.repo.GetSummaryBySalesUser(ctx, salesUserID)
}

func (s *SalesCommissionService) GetSummary(ctx context.Context, salesUserID int64) (*SalesCommissionSummary, error) {
	return s.GetSummaryBySalesUser(ctx, salesUserID)
}

func (s *SalesCommissionService) IsSalesUser(ctx context.Context, userID int64) (bool, error) {
	if s == nil || s.userRepo == nil {
		return false, nil
	}
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return false, err
	}
	return user != nil && user.IsSales, nil
}

func (s *SalesCommissionService) ListRecords(ctx context.Context, params SalesCommissionRecordListParams) ([]SalesCommissionRecord, int, error) {
	return s.repo.ListRecords(ctx, params)
}

func (s *SalesCommissionService) ListSettlements(ctx context.Context, params SalesCommissionSettlementListParams) ([]SalesCommissionSettlement, int, error) {
	return s.repo.ListSettlements(ctx, params)
}

func (s *SalesCommissionService) CreateSettlement(ctx context.Context, input *SalesCommissionSettlementCreate) (*SalesCommissionSettlement, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("sales commission service not initialized")
	}
	if input == nil || input.SalesUserID <= 0 || input.AmountCNY <= 0 {
		return nil, errors.New("invalid settlement input: sales_user_id and amount_cny are required")
	}

	// 校验结算金额不超过实际可结算余额
	summary, err := s.repo.GetSummaryBySalesUser(ctx, input.SalesUserID)
	if err != nil {
		return nil, err
	}
	if summary.SettleableCNY <= 0 {
		return nil, ErrSalesCommissionNoSettleable
	}
	if input.AmountCNY > summary.SettleableCNY {
		return nil, ErrSalesCommissionSettleAmountExceeds
	}

	return s.repo.CreateSettlement(ctx, input)
}

// GetMonthlyProgress 返回销售用户当月梯度进度（spec §9）。
//
//   - 当月已经产生过返佣事件 → 使用 sales_commission_monthly_snapshots 已冻结的规则展示。
//   - 当月还没有事件 → 用 user 当前规则做 "下笔订单将使用的预期规则" 展示，
//     并在响应里通过 snapshot_frozen=false 让前端区分两种语义（spec §8.4）。
//
// 当用户既没有销售身份也没有任何梯度配置时返回 nil，由调用方决定是否当作未授权处理。
func (s *SalesCommissionService) GetMonthlyProgress(ctx context.Context, salesUserID int64) (*SalesCommissionMonthlyProgress, error) {
	if s == nil || s.repo == nil || s.userRepo == nil {
		return nil, nil
	}
	user, err := s.userRepo.GetByID(ctx, salesUserID)
	if err != nil {
		return nil, err
	}
	if user == nil || !user.IsSales {
		return nil, nil
	}

	now := s.now()
	commissionMonth := salesCommissionMonthStart(now)

	data, err := s.repo.GetMonthlyProgress(ctx, salesUserID, commissionMonth)
	if err != nil {
		return nil, err
	}

	progress := &SalesCommissionMonthlyProgress{
		SalesUserID:      salesUserID,
		CommissionMonth:  commissionMonth,
		CurrentTierIndex: -1,
		NextTierIndex:    -1,
		Tiers:            []SalesCommissionTier{},
	}

	var snapshot SalesCommissionSnapshot
	if data != nil && data.Snapshot != nil {
		snapshot = *data.Snapshot
		progress.SnapshotFrozen = true
		progress.MonthlySalesCNY = data.MonthlySalesCNY
		progress.MonthlyCommissionCNY = data.MonthlyCommissionCNY
	} else {
		// 当月无 snapshot，按 user 现行规则做 "预期" 展示。
		snapshot = SalesCommissionSnapshot{
			CommissionMode:      NormalizeSalesCommissionMode(user.SalesCommissionMode),
			FixedCommissionRate: user.SalesCommissionRate,
			MinMonthlySalesCNY:  user.SalesCommissionMinMonthlySales,
			Tiers:               CloneSalesCommissionTiers(user.SalesCommissionTiers),
		}
		progress.SnapshotFrozen = false
	}
	progress.CommissionMode = NormalizeSalesCommissionMode(snapshot.CommissionMode)
	if progress.CommissionMode == SalesCommissionModeTiered {
		// tiered 模式下 fixed_commission_rate 不参与计算，统一返回 0 给前端避免误读。
		progress.FixedCommissionRate = 0
	} else {
		progress.FixedCommissionRate = snapshot.FixedCommissionRate
	}
	progress.MinMonthlySalesCNY = snapshot.MinMonthlySalesCNY
	progress.Tiers = CloneSalesCommissionTiers(snapshot.Tiers)

	progress.ThresholdMet = progress.MonthlySalesCNY >= snapshot.MinMonthlySalesCNY
	if !progress.ThresholdMet {
		progress.ToThresholdCNY = roundSalesCommissionMoney(snapshot.MinMonthlySalesCNY - progress.MonthlySalesCNY)
		if progress.ToThresholdCNY < 0 {
			progress.ToThresholdCNY = 0
		}
	}

	if progress.CommissionMode == SalesCommissionModeTiered && len(progress.Tiers) > 0 {
		idx, next := locateMonthlyTierProgress(progress.MonthlySalesCNY, progress.Tiers)
		progress.CurrentTierIndex = idx
		progress.NextTierIndex = next
		if next >= 0 && next < len(progress.Tiers) {
			progress.NextTierRate = progress.Tiers[next].CommissionRate
			diff := progress.Tiers[next].MonthSalesFromCNY - progress.MonthlySalesCNY
			if diff < 0 {
				diff = 0
			}
			progress.ToNextTierCNY = roundSalesCommissionMoney(diff)
		}
	}

	return progress, nil
}

// locateMonthlyTierProgress 给定当月销售额与已规范排序的梯度档，定位 (current, next) 索引。
//   - current = 销售额命中的档位索引；销售额为 0 或不足以进入第 1 档时为 -1。
//   - next = 当前所在档之后的下一档索引；已位于最高（开口）档时为 -1。
func locateMonthlyTierProgress(monthlySales float64, tiers []SalesCommissionTier) (int, int) {
	current := -1
	for i, tier := range tiers {
		if monthlySales >= tier.MonthSalesFromCNY {
			current = i
		} else {
			break
		}
	}
	if current < 0 {
		// 未达任何档位入口（通常代表第 1 档下界 > 0），下一档就是第 1 档。
		if len(tiers) > 0 {
			return -1, 0
		}
		return -1, -1
	}
	next := current + 1
	if next >= len(tiers) {
		next = -1
	}
	// 命中开口档（无 to 上限）时也视为已在最高档。
	if current >= 0 && tiers[current].MonthSalesToCNY == nil {
		next = -1
	}
	return current, next
}

func salesCommissionEventTimeFromOrder(order *dbent.PaymentOrder) time.Time {
	if order == nil {
		return time.Now().UTC()
	}
	if order.PaidAt != nil && !order.PaidAt.IsZero() {
		return order.PaidAt.UTC()
	}
	if order.CompletedAt != nil && !order.CompletedAt.IsZero() {
		return order.CompletedAt.UTC()
	}
	if !order.CreatedAt.IsZero() {
		return order.CreatedAt.UTC()
	}
	return time.Now().UTC()
}

func salesCommissionMonthStart(eventAt time.Time) time.Time {
	local := eventAt.In(salesCommissionMonthLocation)
	return time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// GetOverview 计算并返回 admin 端数据看板（spec §15）。
//
// 流程：
//  1. 按 RangeKey 解析事件时间区间（custom 时需要 Start/End）。
//  2. 用当前佣金月起算近 12 个月作为 trend 窗口。
//  3. 调 repo.GetOverview 拿原始聚合，再按 12 月窗口补零成 monthly_trend。
func (s *SalesCommissionService) GetOverview(ctx context.Context, params SalesCommissionOverviewParams) (*SalesCommissionOverview, error) {
	if s == nil || s.repo == nil {
		return nil, nil
	}
	rng, err := resolveSalesCommissionOverviewRange(params, s.now())
	if err != nil {
		return nil, err
	}

	trendEnd := salesCommissionMonthStart(s.now())
	// 12 个月窗口：往前推 11 个月作为起点，保证含当月共 12 个月。
	trendStart := salesCommissionMonthStart(trendEnd.AddDate(0, -11, 0))

	data, err := s.repo.GetOverview(ctx, SalesCommissionOverviewQuery{
		Start:             rng.Start,
		End:               rng.End,
		MonthlyTrendStart: trendStart,
		MonthlyTrendEnd:   trendEnd,
	})
	if err != nil {
		return nil, err
	}

	return &SalesCommissionOverview{
		Range:           rng,
		KPI:             data.KPI,
		MonthlyTrend:    fillSalesCommissionMonthlyTrend(trendStart, trendEnd, data.MonthlyTrend),
		TopSales:        data.TopSales,
		StatusBreakdown: data.StatusBreakdown,
		ModeBreakdown:   data.ModeBreakdown,
	}, nil
}

// resolveSalesCommissionOverviewRange 把 RangeKey 解析为闭区间 [start, end]。
//
// 所有边界以 Asia/Shanghai 时区计算然后转回 UTC，与 salesCommissionMonthStart 保持一致。
func resolveSalesCommissionOverviewRange(params SalesCommissionOverviewParams, now time.Time) (SalesCommissionOverviewRange, error) {
	loc := salesCommissionMonthLocation
	local := now.In(loc)
	key := strings.TrimSpace(params.RangeKey)
	if key == "" {
		key = SalesCommissionOverviewRangeThisMonth
	}

	var start, end time.Time
	switch key {
	case SalesCommissionOverviewRangeToday:
		start = time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
		end = start.Add(24 * time.Hour).Add(-time.Nanosecond)
	case SalesCommissionOverviewRangeThisWeek:
		// 周一为本周起点（符合中文场景）。
		weekday := int(local.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		monday := time.Date(local.Year(), local.Month(), local.Day()-(weekday-1), 0, 0, 0, 0, loc)
		start = monday
		end = monday.AddDate(0, 0, 7).Add(-time.Nanosecond)
	case SalesCommissionOverviewRangeThisMonth:
		start = time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, loc)
		end = start.AddDate(0, 1, 0).Add(-time.Nanosecond)
	case SalesCommissionOverviewRangeQuarter:
		quarterStartMonth := time.Month(((int(local.Month())-1)/3)*3 + 1)
		start = time.Date(local.Year(), quarterStartMonth, 1, 0, 0, 0, 0, loc)
		end = start.AddDate(0, 3, 0).Add(-time.Nanosecond)
	case SalesCommissionOverviewRangeThisYear:
		start = time.Date(local.Year(), 1, 1, 0, 0, 0, 0, loc)
		end = start.AddDate(1, 0, 0).Add(-time.Nanosecond)
	case SalesCommissionOverviewRangeLast30Days:
		today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
		end = today.Add(24 * time.Hour).Add(-time.Nanosecond)
		start = today.AddDate(0, 0, -29)
	case SalesCommissionOverviewRangeLast90Days:
		today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
		end = today.Add(24 * time.Hour).Add(-time.Nanosecond)
		start = today.AddDate(0, 0, -89)
	case SalesCommissionOverviewRangeCustom:
		if params.Start == nil || params.End == nil {
			return SalesCommissionOverviewRange{}, ErrSalesCommissionInvalidRange
		}
		s := params.Start.In(loc)
		e := params.End.In(loc)
		if e.Before(s) {
			return SalesCommissionOverviewRange{}, ErrSalesCommissionInvalidRange
		}
		start = time.Date(s.Year(), s.Month(), s.Day(), 0, 0, 0, 0, loc)
		end = time.Date(e.Year(), e.Month(), e.Day(), 23, 59, 59, int(time.Second-time.Nanosecond), loc)
	default:
		return SalesCommissionOverviewRange{}, ErrSalesCommissionInvalidRange
	}

	return SalesCommissionOverviewRange{Key: key, Start: start.UTC(), End: end.UTC()}, nil
}

// fillSalesCommissionMonthlyTrend 按 12 月窗口补零；返回值长度严格 = 月份数。
func fillSalesCommissionMonthlyTrend(start, end time.Time, hits []SalesCommissionMonthlyTrend) []SalesCommissionMonthlyTrend {
	indexByMonth := make(map[time.Time]SalesCommissionMonthlyTrend, len(hits))
	for _, h := range hits {
		key := salesCommissionMonthStart(h.Month)
		indexByMonth[key] = SalesCommissionMonthlyTrend{
			Month:                 key,
			RelatedOrderAmountCNY: h.RelatedOrderAmountCNY,
			CommissionTotalCNY:    h.CommissionTotalCNY,
		}
	}
	out := make([]SalesCommissionMonthlyTrend, 0, 12)
	cursor := salesCommissionMonthStart(start)
	endMonth := salesCommissionMonthStart(end)
	for !cursor.After(endMonth) {
		if hit, ok := indexByMonth[cursor]; ok {
			out = append(out, hit)
		} else {
			out = append(out, SalesCommissionMonthlyTrend{Month: cursor})
		}
		cursor = cursor.AddDate(0, 1, 0)
	}
	return out
}
