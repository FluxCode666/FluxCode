package service

import "time"

// PricingPlanGroup represents a group(section) on the public pricing page.
type PricingPlanGroup struct {
	ID          int64
	Name        string
	Description *string
	SortOrder   int
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time

	Plans []PricingPlan
}

// PricingPlan represents a single plan card on the public pricing page.
type PricingPlan struct {
	ID          int64
	GroupID     int64
	Name        string
	Description *string

	IconURL   *string
	BadgeText *string
	Tagline   *string

	PriceAmount   *float64
	PriceCurrency string
	PricePeriod   string
	PriceText     *string

	Features       []string
	ContactMethods []PricingPlanContactMethod
	IsFeatured     bool
	SortOrder      int
	Status         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type PricingPlanContactMethod struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}
