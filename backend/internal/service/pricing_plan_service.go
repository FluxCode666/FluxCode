package service

import (
	"context"
	"fmt"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var (
	ErrPricingPlanGroupNotFound = infraerrors.NotFound("PRICING_PLAN_GROUP_NOT_FOUND", "pricing plan group not found")
	ErrPricingPlanGroupExists   = infraerrors.Conflict("PRICING_PLAN_GROUP_EXISTS", "pricing plan group already exists")
	ErrPricingPlanNotFound      = infraerrors.NotFound("PRICING_PLAN_NOT_FOUND", "pricing plan not found")
)

type PricingPlanRepository interface {
	ListGroupsWithPlans(ctx context.Context, onlyActive bool) ([]PricingPlanGroup, error)

	GetGroupByID(ctx context.Context, id int64) (*PricingPlanGroup, error)
	CreateGroup(ctx context.Context, group *PricingPlanGroup) error
	UpdateGroup(ctx context.Context, group *PricingPlanGroup) error
	DeleteGroup(ctx context.Context, id int64) error

	GetPlanByID(ctx context.Context, id int64) (*PricingPlan, error)
	CreatePlan(ctx context.Context, plan *PricingPlan) error
	UpdatePlan(ctx context.Context, plan *PricingPlan) error
	DeletePlan(ctx context.Context, id int64) error
}

type CreatePricingPlanGroupInput struct {
	Name        string
	Description *string
	SortOrder   int
	Status      string // active/inactive
}

type UpdatePricingPlanGroupInput struct {
	Name        *string
	Description *string
	SortOrder   *int
	Status      *string // active/inactive
}

type CreatePricingPlanInput struct {
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
	Status         string // active/inactive
}

type UpdatePricingPlanInput struct {
	GroupID     *int64
	Name        *string
	Description *string

	IconURL   *string
	BadgeText *string
	Tagline   *string

	PriceAmount   *float64
	PriceCurrency *string
	PricePeriod   *string
	PriceText     *string

	Features       *[]string
	ContactMethods *[]PricingPlanContactMethod
	IsFeatured     *bool
	SortOrder      *int
	Status         *string // active/inactive
}

type PricingPlanService struct {
	repo PricingPlanRepository
}

func NewPricingPlanService(repo PricingPlanRepository) *PricingPlanService {
	return &PricingPlanService{repo: repo}
}

// ListPublicGroups returns active groups with active plans, ordered by sort_order.
func (s *PricingPlanService) ListPublicGroups(ctx context.Context) ([]PricingPlanGroup, error) {
	groups, err := s.repo.ListGroupsWithPlans(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("list pricing plan groups: %w", err)
	}

	// Guard public output even if the repository returns mixed-status plans.
	filtered := make([]PricingPlanGroup, 0, len(groups))
	for i := range groups {
		activePlans := make([]PricingPlan, 0, len(groups[i].Plans))
		for j := range groups[i].Plans {
			if strings.EqualFold(strings.TrimSpace(groups[i].Plans[j].Status), StatusActive) {
				activePlans = append(activePlans, groups[i].Plans[j])
			}
		}
		if len(activePlans) == 0 {
			continue
		}
		groups[i].Plans = activePlans
		filtered = append(filtered, groups[i])
	}
	return filtered, nil
}

// ListAdminGroups returns all groups with all plans (including inactive).
func (s *PricingPlanService) ListAdminGroups(ctx context.Context) ([]PricingPlanGroup, error) {
	groups, err := s.repo.ListGroupsWithPlans(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("list admin pricing plan groups: %w", err)
	}
	return groups, nil
}

func (s *PricingPlanService) CreateGroup(ctx context.Context, in CreatePricingPlanGroupInput) (*PricingPlanGroup, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, infraerrors.BadRequest("INVALID_PRICING_PLAN_GROUP_NAME", "name is required")
	}

	status := strings.TrimSpace(in.Status)
	if status == "" {
		status = StatusActive
	}

	group := &PricingPlanGroup{
		Name:        name,
		Description: in.Description,
		SortOrder:   in.SortOrder,
		Status:      status,
	}
	if err := s.repo.CreateGroup(ctx, group); err != nil {
		return nil, fmt.Errorf("create pricing plan group: %w", err)
	}
	return group, nil
}

func (s *PricingPlanService) UpdateGroup(ctx context.Context, id int64, in UpdatePricingPlanGroupInput) (*PricingPlanGroup, error) {
	group, err := s.repo.GetGroupByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get pricing plan group: %w", err)
	}

	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			return nil, infraerrors.BadRequest("INVALID_PRICING_PLAN_GROUP_NAME", "name is required")
		}
		group.Name = name
	}
	if in.Description != nil {
		group.Description = in.Description
	}
	if in.SortOrder != nil {
		group.SortOrder = *in.SortOrder
	}
	if in.Status != nil {
		group.Status = strings.TrimSpace(*in.Status)
	}

	if err := s.repo.UpdateGroup(ctx, group); err != nil {
		return nil, fmt.Errorf("update pricing plan group: %w", err)
	}
	return group, nil
}

func (s *PricingPlanService) DeleteGroup(ctx context.Context, id int64) error {
	if err := s.repo.DeleteGroup(ctx, id); err != nil {
		return fmt.Errorf("delete pricing plan group: %w", err)
	}
	return nil
}

func (s *PricingPlanService) CreatePlan(ctx context.Context, in CreatePricingPlanInput) (*PricingPlan, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, infraerrors.BadRequest("INVALID_PRICING_PLAN_NAME", "name is required")
	}
	if in.GroupID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_PRICING_PLAN_GROUP_ID", "group_id is required")
	}

	// Ensure group exists for clearer error messages (instead of FK violation).
	if _, err := s.repo.GetGroupByID(ctx, in.GroupID); err != nil {
		return nil, fmt.Errorf("get pricing plan group: %w", err)
	}

	if in.PriceAmount != nil && *in.PriceAmount < 0 {
		return nil, infraerrors.BadRequest("INVALID_PRICING_PLAN_PRICE", "price_amount must be >= 0")
	}

	currency := strings.TrimSpace(in.PriceCurrency)
	if currency == "" {
		currency = "CNY"
	}
	period := strings.TrimSpace(in.PricePeriod)
	if period == "" {
		period = "month"
	}
	status := strings.TrimSpace(in.Status)
	if status == "" {
		status = StatusActive
	}

	plan := &PricingPlan{
		GroupID:        in.GroupID,
		Name:           name,
		Description:    normalizeOptionalStringPtr(in.Description),
		IconURL:        normalizeOptionalStringPtr(in.IconURL),
		BadgeText:      normalizeOptionalStringPtr(in.BadgeText),
		Tagline:        normalizeOptionalStringPtr(in.Tagline),
		PriceAmount:    in.PriceAmount,
		PriceCurrency:  currency,
		PricePeriod:    period,
		PriceText:      normalizeOptionalStringPtr(in.PriceText),
		Features:       normalizeStringSlice(in.Features),
		ContactMethods: normalizeContactMethods(in.ContactMethods),
		IsFeatured:     in.IsFeatured,
		SortOrder:      in.SortOrder,
		Status:         status,
	}
	if err := s.repo.CreatePlan(ctx, plan); err != nil {
		return nil, fmt.Errorf("create pricing plan: %w", err)
	}
	return plan, nil
}

func (s *PricingPlanService) UpdatePlan(ctx context.Context, id int64, in UpdatePricingPlanInput) (*PricingPlan, error) {
	plan, err := s.repo.GetPlanByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get pricing plan: %w", err)
	}

	if in.GroupID != nil {
		if *in.GroupID <= 0 {
			return nil, infraerrors.BadRequest("INVALID_PRICING_PLAN_GROUP_ID", "group_id must be > 0")
		}
		if _, err := s.repo.GetGroupByID(ctx, *in.GroupID); err != nil {
			return nil, fmt.Errorf("get pricing plan group: %w", err)
		}
		plan.GroupID = *in.GroupID
	}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			return nil, infraerrors.BadRequest("INVALID_PRICING_PLAN_NAME", "name is required")
		}
		plan.Name = name
	}
	if in.Description != nil {
		plan.Description = normalizeOptionalStringPtr(in.Description)
	}
	if in.IconURL != nil {
		plan.IconURL = normalizeOptionalStringPtr(in.IconURL)
	}
	if in.BadgeText != nil {
		plan.BadgeText = normalizeOptionalStringPtr(in.BadgeText)
	}
	if in.Tagline != nil {
		plan.Tagline = normalizeOptionalStringPtr(in.Tagline)
	}

	if in.PriceAmount != nil && *in.PriceAmount < 0 {
		return nil, infraerrors.BadRequest("INVALID_PRICING_PLAN_PRICE", "price_amount must be >= 0")
	}
	if in.PriceAmount != nil {
		plan.PriceAmount = in.PriceAmount
	}
	if in.PriceCurrency != nil {
		plan.PriceCurrency = strings.TrimSpace(*in.PriceCurrency)
	}
	if in.PricePeriod != nil {
		plan.PricePeriod = strings.TrimSpace(*in.PricePeriod)
	}
	if in.PriceText != nil {
		plan.PriceText = normalizeOptionalStringPtr(in.PriceText)
	}

	if in.Features != nil {
		plan.Features = normalizeStringSlice(*in.Features)
	}
	if in.ContactMethods != nil {
		plan.ContactMethods = normalizeContactMethods(*in.ContactMethods)
	}
	if in.IsFeatured != nil {
		plan.IsFeatured = *in.IsFeatured
	}
	if in.SortOrder != nil {
		plan.SortOrder = *in.SortOrder
	}
	if in.Status != nil {
		plan.Status = strings.TrimSpace(*in.Status)
	}

	if err := s.repo.UpdatePlan(ctx, plan); err != nil {
		return nil, fmt.Errorf("update pricing plan: %w", err)
	}
	return plan, nil
}

func (s *PricingPlanService) DeletePlan(ctx context.Context, id int64) error {
	if err := s.repo.DeletePlan(ctx, id); err != nil {
		return fmt.Errorf("delete pricing plan: %w", err)
	}
	return nil
}

func normalizeStringSlice(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, s := range in {
		v := strings.TrimSpace(s)
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	return out
}

func normalizeOptionalStringPtr(in *string) *string {
	if in == nil {
		return nil
	}
	v := strings.TrimSpace(*in)
	if v == "" {
		return nil
	}
	return &v
}

func normalizeContactMethods(in []PricingPlanContactMethod) []PricingPlanContactMethod {
	if len(in) == 0 {
		return nil
	}
	out := make([]PricingPlanContactMethod, 0, len(in))
	for i := range in {
		typ := strings.TrimSpace(in[i].Type)
		val := strings.TrimSpace(in[i].Value)
		if typ == "" || val == "" {
			continue
		}
		out = append(out, PricingPlanContactMethod{
			Type:  typ,
			Value: val,
		})
	}
	return out
}
