package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
)

// --- Dashboard & Analytics ---

func (s *PaymentService) GetDashboardStats(ctx context.Context, days int) (*DashboardStats, error) {
	if days <= 0 {
		days = 30
	}
	now := time.Now()
	todayStart := paymentStatsStartOfDay(now)
	startTime := todayStart.AddDate(0, 0, -days+1)
	endTime := todayStart.AddDate(0, 0, 1)

	return s.GetDashboardStatsForRange(ctx, startTime, endTime)
}

func (s *PaymentService) GetDashboardStatsForRange(ctx context.Context, startTime, endTime time.Time) (*DashboardStats, error) {
	if startTime.IsZero() || endTime.IsZero() || !endTime.After(startTime) {
		now := time.Now()
		endTime = paymentStatsStartOfDay(now).AddDate(0, 0, 1)
		startTime = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	}
	todayStart := paymentStatsStartOfDay(time.Now().In(startTime.Location()))

	paidStatuses := []string{OrderStatusCompleted, OrderStatusPaid, OrderStatusRecharging}

	orders, err := s.entClient.PaymentOrder.Query().
		Where(
			paymentorder.StatusIn(paidStatuses...),
			paymentorder.PaidAtGTE(startTime),
			paymentorder.PaidAtLT(endTime),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	st := &DashboardStats{}
	computeBasicStats(st, orders, todayStart)

	st.PendingOrders, err = s.entClient.PaymentOrder.Query().
		Where(paymentorder.StatusEQ(OrderStatusPending)).
		Count(ctx)
	if err != nil {
		return nil, err
	}

	st.DailySeries = buildDailySeries(orders, startTime, endTime)
	st.PaymentMethods = buildMethodDistribution(orders)
	st.TopUsers = buildTopUsers(orders)

	return st, nil
}

func computeBasicStats(st *DashboardStats, orders []*dbent.PaymentOrder, todayStart time.Time) {
	var totalAmount, todayAmount float64
	var todayCount int
	for _, o := range orders {
		totalAmount += o.PayAmount
		if o.PaidAt != nil && !o.PaidAt.Before(todayStart) {
			todayAmount += o.PayAmount
			todayCount++
		}
	}
	st.TotalAmount = math.Round(totalAmount*100) / 100
	st.TodayAmount = math.Round(todayAmount*100) / 100
	st.TotalCount = len(orders)
	st.TodayCount = todayCount
	if st.TotalCount > 0 {
		st.AvgAmount = math.Round(totalAmount/float64(st.TotalCount)*100) / 100
	}
}

func buildDailySeries(orders []*dbent.PaymentOrder, startTime, endTime time.Time) []DailyStats {
	loc := startTime.Location()
	if loc == nil {
		loc = time.Local
	}
	dailyMap := make(map[string]*DailyStats)
	for _, o := range orders {
		if o.PaidAt == nil {
			continue
		}
		date := o.PaidAt.In(loc).Format("2006-01-02")
		ds, ok := dailyMap[date]
		if !ok {
			ds = &DailyStats{Date: date}
			dailyMap[date] = ds
		}
		ds.Amount += o.PayAmount
		ds.Count++
	}
	startDay := paymentStatsStartOfDay(startTime.In(loc))
	endDay := endTime.In(loc)
	series := make([]DailyStats, 0)
	for current := startDay; current.Before(endDay); current = current.AddDate(0, 0, 1) {
		date := current.Format("2006-01-02")
		if ds, ok := dailyMap[date]; ok {
			ds.Amount = math.Round(ds.Amount*100) / 100
			series = append(series, *ds)
		} else {
			series = append(series, DailyStats{Date: date})
		}
	}
	return series
}

func paymentStatsStartOfDay(t time.Time) time.Time {
	loc := t.Location()
	if loc == nil {
		loc = time.Local
	}
	local := t.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
}

func buildMethodDistribution(orders []*dbent.PaymentOrder) []PaymentMethodStat {
	methodMap := make(map[string]*PaymentMethodStat)
	for _, o := range orders {
		ms, ok := methodMap[o.PaymentType]
		if !ok {
			ms = &PaymentMethodStat{Type: o.PaymentType}
			methodMap[o.PaymentType] = ms
		}
		ms.Amount += o.PayAmount
		ms.Count++
	}
	methods := make([]PaymentMethodStat, 0, len(methodMap))
	for _, ms := range methodMap {
		ms.Amount = math.Round(ms.Amount*100) / 100
		methods = append(methods, *ms)
	}
	return methods
}

func buildTopUsers(orders []*dbent.PaymentOrder) []TopUserStat {
	userMap := make(map[int64]*TopUserStat)
	for _, o := range orders {
		us, ok := userMap[o.UserID]
		if !ok {
			us = &TopUserStat{UserID: o.UserID, Email: o.UserEmail}
			userMap[o.UserID] = us
		}
		us.Amount += o.PayAmount
	}
	userList := make([]*TopUserStat, 0, len(userMap))
	for _, us := range userMap {
		us.Amount = math.Round(us.Amount*100) / 100
		userList = append(userList, us)
	}
	sort.Slice(userList, func(i, j int) bool {
		return userList[i].Amount > userList[j].Amount
	})
	limit := topUsersLimit
	if len(userList) < limit {
		limit = len(userList)
	}
	result := make([]TopUserStat, 0, limit)
	for i := 0; i < limit; i++ {
		result = append(result, *userList[i])
	}
	return result
}

// --- Audit Logs ---

func (s *PaymentService) writeAuditLog(ctx context.Context, oid int64, action, op string, detail map[string]any) {
	dj, _ := json.Marshal(detail)
	_, err := s.entClient.PaymentAuditLog.Create().SetOrderID(strconv.FormatInt(oid, 10)).SetAction(action).SetDetail(string(dj)).SetOperator(op).Save(ctx)
	if err != nil {
		slog.Error("audit log failed", "orderID", oid, "action", action, "error", err)
	}
}

func (s *PaymentService) GetOrderAuditLogs(ctx context.Context, oid int64) ([]*dbent.PaymentAuditLog, error) {
	return s.entClient.PaymentAuditLog.Query().Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(oid, 10))).Order(paymentauditlog.ByCreatedAt()).All(ctx)
}
