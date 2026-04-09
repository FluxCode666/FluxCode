package service

import (
	"time"
)

// SubscriptionCacheData represents cached subscription data
type SubscriptionCacheData struct {
	Status             string
	ExpiresAt          time.Time
	DailyUsage         float64
	WeeklyUsage        float64
	MonthlyUsage       float64
	QuotaMultiplier    int
	NextGrantExpiresAt *time.Time
	Version            int64
}
