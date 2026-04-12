package service

import (
	"context"
	"fmt"
	"time"
)

// SubscriptionGrantUsageWindow 子订阅单窗口用量进度
type SubscriptionGrantUsageWindow struct {
	LimitUSD   float64 `json:"limit_usd"`
	UsedUSD    float64 `json:"used_usd"`
	Percentage float64 `json:"percentage"`
	Unlimited  bool    `json:"unlimited"`
}

// SubscriptionGrantUsageDetail 子订阅用量明细
type SubscriptionGrantUsageDetail struct {
	GrantID         int64                         `json:"grant_id"`
	StartsAt        time.Time                     `json:"starts_at"`
	ExpiresAt       time.Time                     `json:"expires_at"`
	DailyUsageUSD   float64                       `json:"daily_usage_usd"`
	WeeklyUsageUSD  float64                       `json:"weekly_usage_usd"`
	MonthlyUsageUSD float64                       `json:"monthly_usage_usd"`
	Daily           *SubscriptionGrantUsageWindow `json:"daily,omitempty"`
	Weekly          *SubscriptionGrantUsageWindow `json:"weekly,omitempty"`
	Monthly         *SubscriptionGrantUsageWindow `json:"monthly,omitempty"`
}

// ActiveSubscriptionGrantUsageResponse 订阅未过期子订阅用量响应（包含未来排期）
type ActiveSubscriptionGrantUsageResponse struct {
	SubscriptionID int64                          `json:"subscription_id"`
	GroupID        int64                          `json:"group_id"`
	GroupName      string                         `json:"group_name"`
	Grants         []SubscriptionGrantUsageDetail `json:"grants"`
}

func calcGrantUsageWindow(used float64, limit *float64) *SubscriptionGrantUsageWindow {
	if limit == nil {
		return &SubscriptionGrantUsageWindow{UsedUSD: used, Unlimited: true}
	}

	percentage := 0.0
	if *limit > 0 {
		percentage = (used / *limit) * 100
		if percentage > 100 {
			percentage = 100
		}
		if percentage < 0 {
			percentage = 0
		}
	}

	return &SubscriptionGrantUsageWindow{
		LimitUSD:   *limit,
		UsedUSD:    used,
		Percentage: percentage,
		Unlimited:  false,
	}
}

// GetUserGrantUsageDetails 获取用户可见的未过期子订阅用量明细（包含未来排期）
func (s *SubscriptionService) GetUserGrantUsageDetails(ctx context.Context, userID, subscriptionID int64) (*ActiveSubscriptionGrantUsageResponse, error) {
	sub, err := s.userSubRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return nil, err
	}
	if sub.UserID != userID {
		return nil, ErrSubscriptionNotFound
	}

	group := sub.Group
	if group == nil {
		if s.groupRepo == nil {
			return nil, fmt.Errorf("group repo not configured")
		}
		group, err = s.groupRepo.GetByID(ctx, sub.GroupID)
		if err != nil {
			return nil, err
		}
	}

	if s.grantRepo == nil {
		return &ActiveSubscriptionGrantUsageResponse{
			SubscriptionID: sub.ID,
			GroupID:        sub.GroupID,
			GroupName:      group.Name,
			Grants:         []SubscriptionGrantUsageDetail{},
		}, nil
	}

	grants, err := s.grantRepo.ListUnexpiredBySubscriptionID(ctx, sub.ID, time.Now())
	if err != nil {
		return nil, err
	}

	out := make([]SubscriptionGrantUsageDetail, 0, len(grants))
	for i := range grants {
		g := grants[i]
		detail := SubscriptionGrantUsageDetail{
			GrantID:         g.ID,
			StartsAt:        g.StartsAt,
			ExpiresAt:       g.ExpiresAt,
			DailyUsageUSD:   g.DailyUsageUSD,
			WeeklyUsageUSD:  g.WeeklyUsageUSD,
			MonthlyUsageUSD: g.MonthlyUsageUSD,
		}

		if group.HasDailyLimit() {
			detail.Daily = calcGrantUsageWindow(g.DailyUsageUSD, group.DailyLimitUSD)
		}
		if group.HasWeeklyLimit() {
			detail.Weekly = calcGrantUsageWindow(g.WeeklyUsageUSD, group.WeeklyLimitUSD)
		}
		if group.HasMonthlyLimit() {
			detail.Monthly = calcGrantUsageWindow(g.MonthlyUsageUSD, group.MonthlyLimitUSD)
		}
		out = append(out, detail)
	}

	return &ActiveSubscriptionGrantUsageResponse{
		SubscriptionID: sub.ID,
		GroupID:        sub.GroupID,
		GroupName:      group.Name,
		Grants:         out,
	}, nil
}
