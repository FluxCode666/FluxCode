package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
)

const (
	MediaPricingSnapshotVersion = 1
	MediaPricingSourceChannel   = "channel"
	MediaPricingCurrencyUSD     = "USD"
	MediaBillingUnitImage       = "image"
	MediaBillingUnitSecond      = "second"
)

var (
	ErrMediaPricingUnavailable = infraerrors.BadRequest(
		"MEDIA_PRICING_UNAVAILABLE",
		"media pricing is not configured for the requested model",
	)
	ErrMediaPricingUnsupportedUnit = infraerrors.BadRequest(
		"MEDIA_PRICING_UNSUPPORTED_UNIT",
		"media billing unit cannot be estimated safely",
	)
	ErrInvalidMediaPricingConfiguration = errors.New("invalid media pricing configuration")
	ErrInvalidMediaPricingUsage         = errors.New("invalid media pricing usage")
)

// MediaChannelPricingProvider exposes the group-scoped channel pricing used by
// media requests. The production ChannelService satisfies this interface.
type MediaChannelPricingProvider interface {
	GetChannelModelPricing(ctx context.Context, groupID int64, model string) *ChannelModelPricing
}

// MediaUserGroupRateProvider exposes the optional per-user override for a
// group's default rate. A non-nil zero value is a valid free rate and must not
// be treated as an absent override.
type MediaUserGroupRateProvider interface {
	GetByUserAndGroup(ctx context.Context, userID, groupID int64) (*float64, error)
}

type MediaPricingTierSnapshot struct {
	TierLabel string  `json:"tier_label"`
	UnitPrice float64 `json:"unit_price"`
}

type MediaPricingCandidateContext struct {
	AccountID     int64  `json:"account_id"`
	Provider      string `json:"provider"`
	UpstreamModel string `json:"upstream_model"`
}

// MediaPricingSettlement is the deterministic monetary result calculated from
// a frozen pricing snapshot and actual provider usage. AccountBaseAmount is the
// pre-multiplier model cost used for account-side quota/cost accounting;
// UserAmount is the amount charged to the user after the frozen effective rate.
type MediaPricingSettlement struct {
	BillingUnit       string  `json:"billing_unit"`
	Quantity          float64 `json:"quantity"`
	Tier              string  `json:"tier"`
	UnitPrice         float64 `json:"unit_price"`
	AccountBaseAmount float64 `json:"account_base_amount"`
	UserAmount        float64 `json:"user_amount"`
	EffectiveRate     float64 `json:"effective_rate"`
}

// MediaPricingSnapshotV1 is the immutable pricing contract stored inside
// MediaBillingSnapshot.PricingSnapshot. It intentionally carries both the
// selected estimate and the complete relevant unit-price table so successful
// settlement can use actual MediaUsage without consulting mutable settings.
type MediaPricingSnapshotV1 struct {
	Version                  int                            `json:"version"`
	Source                   string                         `json:"source"`
	Currency                 string                         `json:"currency"`
	UserID                   int64                          `json:"user_id"`
	APIKeyID                 int64                          `json:"api_key_id"`
	GroupID                  int64                          `json:"group_id"`
	ChannelID                int64                          `json:"channel_id"`
	PricingID                int64                          `json:"pricing_id"`
	Platform                 string                         `json:"platform"`
	Vendor                   string                         `json:"vendor"`
	RequestedModel           string                         `json:"requested_model"`
	CanonicalModel           string                         `json:"canonical_model"`
	MediaType                MediaType                      `json:"media_type"`
	Operation                MediaOperation                 `json:"operation"`
	BillingUnit              string                         `json:"billing_unit"`
	BillingMode              BillingMode                    `json:"billing_mode"`
	DefaultUnitPrice         *float64                       `json:"default_unit_price"`
	Tiers                    []MediaPricingTierSnapshot     `json:"tiers"`
	EstimatedQuantity        float64                        `json:"estimated_quantity"`
	EstimatedTier            string                         `json:"estimated_tier"`
	EstimatedUnitPrice       float64                        `json:"estimated_unit_price"`
	BaseEstimatedAmount      float64                        `json:"base_estimated_amount"`
	GroupDefaultMultiplier   float64                        `json:"group_default_multiplier"`
	UserMultiplier           *float64                       `json:"user_multiplier"`
	UserMultiplierOverridden bool                           `json:"user_multiplier_overridden"`
	EffectiveMultiplier      float64                        `json:"effective_multiplier"`
	Candidates               []MediaPricingCandidateContext `json:"candidates"`
}

type ProductionMediaPricing struct {
	groups    MediaGroupProvider
	channels  MediaChannelPricingProvider
	userRates MediaUserGroupRateProvider
}

func NewProductionMediaPricing(
	groups MediaGroupProvider,
	channels MediaChannelPricingProvider,
	userRates MediaUserGroupRateProvider,
) *ProductionMediaPricing {
	return &ProductionMediaPricing{groups: groups, channels: channels, userRates: userRates}
}

func (p *ProductionMediaPricing) Snapshot(
	ctx context.Context,
	req MediaCreateRequest,
	definition *MediaModelDefinition,
	candidates []MediaAccountCandidateSnapshot,
) (MediaBillingSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return MediaBillingSnapshot{}, err
	}
	if p == nil || p.groups == nil || p.channels == nil || p.userRates == nil {
		return MediaBillingSnapshot{}, errors.New("production media pricing dependencies are incomplete")
	}
	if definition == nil || strings.TrimSpace(definition.ModelID) == "" || strings.TrimSpace(definition.Vendor) == "" {
		return MediaBillingSnapshot{}, fmt.Errorf("%w: media model definition is missing", ErrInvalidMediaPricingConfiguration)
	}
	if req.UserID <= 0 || req.APIKeyID <= 0 || req.GroupID <= 0 {
		return MediaBillingSnapshot{}, fmt.Errorf("%w: media pricing identity is invalid", ErrInvalidMediaPricingConfiguration)
	}
	operationMediaType, validOperation := mediaTypeForOperation(req.Operation)
	if strings.TrimSpace(req.RequestedModel) == "" || !validOperation || operationMediaType != req.MediaType ||
		definition.MediaType != req.MediaType || !definition.Supports(req.Operation) {
		return MediaBillingSnapshot{}, fmt.Errorf("%w: media pricing request does not match the model definition", ErrInvalidMediaPricingConfiguration)
	}

	canonicalModel := normalizeMediaModelID(definition.ModelID)
	billingUnit := strings.ToLower(strings.TrimSpace(definition.BillingUnit))
	estimatedQuantity, estimatedTier, expectedMode, err := estimateMediaPricingUnits(req, billingUnit)
	if err != nil {
		return MediaBillingSnapshot{}, err
	}

	group, err := p.groups.GetByID(ctx, req.GroupID)
	if err != nil {
		return MediaBillingSnapshot{}, fmt.Errorf("load media pricing group: %w", err)
	}
	if group == nil || group.ID != req.GroupID || group.Platform != PlatformMedia {
		return MediaBillingSnapshot{}, fmt.Errorf("%w: media pricing group is invalid", ErrInvalidMediaPricingConfiguration)
	}
	if err := validateMediaPricingMultiplier(group.RateMultiplier, "group default"); err != nil {
		return MediaBillingSnapshot{}, err
	}

	pricing := p.channels.GetChannelModelPricing(ctx, req.GroupID, canonicalModel)
	if pricing == nil {
		return MediaBillingSnapshot{}, ErrMediaPricingUnavailable.WithMetadata(map[string]string{
			"group_id": fmt.Sprintf("%d", req.GroupID),
			"model":    canonicalModel,
		})
	}
	if pricing.ChannelID <= 0 || pricing.ID <= 0 || strings.ToLower(strings.TrimSpace(pricing.Platform)) != PlatformMedia {
		return MediaBillingSnapshot{}, fmt.Errorf("%w: channel pricing provenance is invalid", ErrInvalidMediaPricingConfiguration)
	}
	if pricing.BillingMode != expectedMode {
		return MediaBillingSnapshot{}, fmt.Errorf(
			"%w: billing_unit %q requires channel billing mode %q, got %q",
			ErrInvalidMediaPricingConfiguration,
			billingUnit,
			expectedMode,
			pricing.BillingMode,
		)
	}

	defaultUnitPrice, tiers, err := freezeMediaUnitPrices(pricing)
	if err != nil {
		return MediaBillingSnapshot{}, err
	}
	estimatedUnitPrice, selectedTier, ok := resolveFrozenMediaUnitPrice(defaultUnitPrice, tiers, estimatedTier)
	if !ok {
		return MediaBillingSnapshot{}, ErrMediaPricingUnavailable.WithMetadata(map[string]string{
			"group_id":     fmt.Sprintf("%d", req.GroupID),
			"model":        canonicalModel,
			"billing_unit": billingUnit,
			"tier":         strings.TrimSpace(estimatedTier),
		})
	}

	userMultiplier, err := p.userRates.GetByUserAndGroup(ctx, req.UserID, req.GroupID)
	if err != nil {
		return MediaBillingSnapshot{}, fmt.Errorf("load media user group rate: %w", err)
	}
	effectiveMultiplier := group.RateMultiplier
	if userMultiplier != nil {
		if err := validateMediaPricingMultiplier(*userMultiplier, "user override"); err != nil {
			return MediaBillingSnapshot{}, err
		}
		effectiveMultiplier = *userMultiplier
	}

	baseEstimated, err := calculateFrozenMediaAmount(estimatedUnitPrice, estimatedQuantity, 1)
	if err != nil {
		return MediaBillingSnapshot{}, fmt.Errorf("%w: calculate base estimate: %v", ErrInvalidMediaPricingConfiguration, err)
	}
	estimatedAmount, err := calculateFrozenMediaAmount(estimatedUnitPrice, estimatedQuantity, effectiveMultiplier)
	if err != nil {
		return MediaBillingSnapshot{}, fmt.Errorf("%w: calculate estimate: %v", ErrInvalidMediaPricingConfiguration, err)
	}

	candidateModels, candidateContext, err := freezeMediaPricingCandidates(candidates)
	if err != nil {
		return MediaBillingSnapshot{}, err
	}
	frozenUserMultiplier := cloneFloat64Pointer(userMultiplier)
	frozen := MediaPricingSnapshotV1{
		Version:                  MediaPricingSnapshotVersion,
		Source:                   MediaPricingSourceChannel,
		Currency:                 MediaPricingCurrencyUSD,
		UserID:                   req.UserID,
		APIKeyID:                 req.APIKeyID,
		GroupID:                  req.GroupID,
		ChannelID:                pricing.ChannelID,
		PricingID:                pricing.ID,
		Platform:                 strings.ToLower(strings.TrimSpace(pricing.Platform)),
		Vendor:                   strings.ToLower(strings.TrimSpace(definition.Vendor)),
		RequestedModel:           strings.TrimSpace(req.RequestedModel),
		CanonicalModel:           canonicalModel,
		MediaType:                req.MediaType,
		Operation:                req.Operation,
		BillingUnit:              billingUnit,
		BillingMode:              pricing.BillingMode,
		DefaultUnitPrice:         cloneFloat64Pointer(defaultUnitPrice),
		Tiers:                    append([]MediaPricingTierSnapshot(nil), tiers...),
		EstimatedQuantity:        estimatedQuantity,
		EstimatedTier:            selectedTier,
		EstimatedUnitPrice:       estimatedUnitPrice,
		BaseEstimatedAmount:      baseEstimated,
		GroupDefaultMultiplier:   group.RateMultiplier,
		UserMultiplier:           frozenUserMultiplier,
		UserMultiplierOverridden: frozenUserMultiplier != nil,
		EffectiveMultiplier:      effectiveMultiplier,
		Candidates:               candidateContext,
	}
	encoded, err := json.Marshal(frozen)
	if err != nil {
		return MediaBillingSnapshot{}, fmt.Errorf("encode media pricing snapshot: %w", err)
	}

	return MediaBillingSnapshot{
		RequestedModel:  strings.TrimSpace(req.RequestedModel),
		CandidateModels: candidateModels,
		EstimatedAmount: estimatedAmount,
		GroupMultiplier: effectiveMultiplier,
		PricingSnapshot: encoded,
	}, nil
}

func estimateMediaPricingUnits(req MediaCreateRequest, billingUnit string) (float64, string, BillingMode, error) {
	switch billingUnit {
	case MediaBillingUnitImage:
		if req.MediaType != MediaTypeImage || req.Spec.Image == nil || req.Spec.Video != nil ||
			req.Spec.Image.Count <= 0 || req.Spec.Image.Count > MaxMediaImageCount {
			return 0, "", "", fmt.Errorf("%w: image pricing request is invalid", ErrInvalidMediaPricingConfiguration)
		}
		return float64(req.Spec.Image.Count), strings.TrimSpace(req.Spec.Image.Size), BillingModeImage, nil
	case MediaBillingUnitSecond:
		if req.MediaType != MediaTypeVideo || req.Spec.Video == nil || req.Spec.Image != nil ||
			req.Spec.Video.DurationSeconds <= 0 || req.Spec.Video.DurationSeconds > MaxMediaVideoDurationSeconds {
			return 0, "", "", ErrMediaPricingUnsupportedUnit.WithMetadata(map[string]string{
				"billing_unit": billingUnit,
				"reason":       "video duration is not explicit",
			})
		}
		return float64(req.Spec.Video.DurationSeconds), strings.TrimSpace(req.Spec.Video.Resolution), BillingModePerRequest, nil
	default:
		return 0, "", "", ErrMediaPricingUnsupportedUnit.WithMetadata(map[string]string{
			"billing_unit": billingUnit,
		})
	}
}

func freezeMediaUnitPrices(pricing *ChannelModelPricing) (*float64, []MediaPricingTierSnapshot, error) {
	if pricing == nil {
		return nil, nil, ErrMediaPricingUnavailable
	}
	var defaultUnitPrice *float64
	if pricing.PerRequestPrice != nil {
		if err := validateMediaUnitPrice(*pricing.PerRequestPrice, "default"); err != nil {
			return nil, nil, err
		}
		defaultUnitPrice = cloneFloat64Pointer(pricing.PerRequestPrice)
	}

	tiers := make([]MediaPricingTierSnapshot, 0, len(pricing.Intervals))
	seen := make(map[string]struct{}, len(pricing.Intervals))
	for index := range pricing.Intervals {
		interval := pricing.Intervals[index]
		if interval.PerRequestPrice == nil {
			continue
		}
		label := normalizeMediaPricingTier(interval.TierLabel)
		if label == "" {
			return nil, nil, fmt.Errorf(
				"%w: media price tier at index %d has no tier label",
				ErrInvalidMediaPricingConfiguration,
				index,
			)
		}
		if _, duplicate := seen[label]; duplicate {
			return nil, nil, fmt.Errorf("%w: duplicate media price tier %q", ErrInvalidMediaPricingConfiguration, label)
		}
		if err := validateMediaUnitPrice(*interval.PerRequestPrice, "tier "+label); err != nil {
			return nil, nil, err
		}
		seen[label] = struct{}{}
		tiers = append(tiers, MediaPricingTierSnapshot{TierLabel: label, UnitPrice: *interval.PerRequestPrice})
	}
	sort.Slice(tiers, func(i, j int) bool { return tiers[i].TierLabel < tiers[j].TierLabel })
	if defaultUnitPrice == nil && len(tiers) == 0 {
		return nil, nil, ErrMediaPricingUnavailable
	}
	return defaultUnitPrice, tiers, nil
}

func freezeMediaPricingCandidates(candidates []MediaAccountCandidateSnapshot) ([]string, []MediaPricingCandidateContext, error) {
	if len(candidates) == 0 {
		return nil, nil, fmt.Errorf("%w: media pricing candidates are empty", ErrInvalidMediaPricingConfiguration)
	}
	models := make([]string, 0, len(candidates))
	contexts := make([]MediaPricingCandidateContext, 0, len(candidates))
	seenAccounts := make(map[int64]struct{}, len(candidates))
	seenModels := make(map[string]struct{}, len(candidates))
	for index := range candidates {
		candidate := candidates[index]
		upstreamModel := strings.TrimSpace(candidate.ResolvedModel.UpstreamModel)
		if candidate.AccountID <= 0 || upstreamModel == "" {
			return nil, nil, fmt.Errorf("%w: media pricing candidate at index %d is invalid", ErrInvalidMediaPricingConfiguration, index)
		}
		if _, duplicate := seenAccounts[candidate.AccountID]; duplicate {
			return nil, nil, fmt.Errorf("%w: duplicate media pricing candidate account %d", ErrInvalidMediaPricingConfiguration, candidate.AccountID)
		}
		seenAccounts[candidate.AccountID] = struct{}{}
		if _, duplicate := seenModels[upstreamModel]; !duplicate {
			models = append(models, upstreamModel)
			seenModels[upstreamModel] = struct{}{}
		}
		contexts = append(contexts, MediaPricingCandidateContext{
			AccountID:     candidate.AccountID,
			Provider:      strings.TrimSpace(candidate.ResolvedModel.Provider),
			UpstreamModel: upstreamModel,
		})
	}
	return models, contexts, nil
}

func validateMediaPricingMultiplier(value float64, label string) error {
	if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Errorf("%w: %s multiplier=%v", ErrInvalidMediaPricingConfiguration, label, value)
	}
	return nil
}

func validateMediaUnitPrice(value float64, label string) error {
	if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Errorf("%w: %s unit price=%v", ErrInvalidMediaPricingConfiguration, label, value)
	}
	if decimal.NewFromFloat(value).GreaterThan(mediaBillingMaximumAmount) {
		return fmt.Errorf("%w: %s unit price exceeds maximum", ErrInvalidMediaPricingConfiguration, label)
	}
	return nil
}

func normalizeMediaPricingTier(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func resolveFrozenMediaUnitPrice(defaultPrice *float64, tiers []MediaPricingTierSnapshot, tier string) (float64, string, bool) {
	normalizedTier := normalizeMediaPricingTier(tier)
	if normalizedTier != "" {
		for index := range tiers {
			if tiers[index].TierLabel == normalizedTier {
				return tiers[index].UnitPrice, normalizedTier, true
			}
		}
	}
	if defaultPrice != nil {
		return *defaultPrice, normalizedTier, true
	}
	return 0, normalizedTier, false
}

func calculateFrozenMediaAmount(unitPrice, quantity, multiplier float64) (float64, error) {
	if err := validateMediaUnitPrice(unitPrice, "selected"); err != nil {
		return 0, err
	}
	if quantity <= 0 || math.IsNaN(quantity) || math.IsInf(quantity, 0) {
		return 0, fmt.Errorf("%w: quantity=%v", ErrInvalidMediaPricingUsage, quantity)
	}
	if err := validateMediaPricingMultiplier(multiplier, "effective"); err != nil {
		return 0, err
	}
	amount := decimal.NewFromFloat(unitPrice).
		Mul(decimal.NewFromFloat(quantity)).
		Mul(decimal.NewFromFloat(multiplier))
	normalized, err := normalizeMediaDecimalAmount(amount)
	if err != nil {
		return 0, err
	}
	return normalized.InexactFloat64(), nil
}

func cloneFloat64Pointer(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func DecodeMediaPricingSnapshot(raw json.RawMessage) (MediaPricingSnapshotV1, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return MediaPricingSnapshotV1{}, fmt.Errorf("%w: pricing snapshot is empty", ErrInvalidMediaPricingConfiguration)
	}
	var snapshot MediaPricingSnapshotV1
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return MediaPricingSnapshotV1{}, fmt.Errorf("decode media pricing snapshot: %w", err)
	}
	if err := validateFrozenMediaPricingSnapshot(snapshot); err != nil {
		return MediaPricingSnapshotV1{}, err
	}
	return snapshot, nil
}

func validateFrozenMediaPricingSnapshot(snapshot MediaPricingSnapshotV1) error {
	if snapshot.Version != MediaPricingSnapshotVersion ||
		snapshot.Source != MediaPricingSourceChannel ||
		snapshot.Currency != MediaPricingCurrencyUSD ||
		snapshot.UserID <= 0 || snapshot.APIKeyID <= 0 || snapshot.GroupID <= 0 ||
		snapshot.ChannelID <= 0 || snapshot.PricingID <= 0 ||
		snapshot.Platform != PlatformMedia || snapshot.Vendor == "" ||
		snapshot.RequestedModel == "" || snapshot.CanonicalModel == "" ||
		len(snapshot.Candidates) == 0 {
		return fmt.Errorf("%w: pricing snapshot identity is invalid", ErrInvalidMediaPricingConfiguration)
	}
	operationMediaType, validOperation := mediaTypeForOperation(snapshot.Operation)
	if !validOperation || operationMediaType != snapshot.MediaType {
		return fmt.Errorf("%w: pricing snapshot operation is invalid", ErrInvalidMediaPricingConfiguration)
	}
	seenAccounts := make(map[int64]struct{}, len(snapshot.Candidates))
	for index := range snapshot.Candidates {
		candidate := snapshot.Candidates[index]
		if candidate.AccountID <= 0 || strings.TrimSpace(candidate.UpstreamModel) == "" {
			return fmt.Errorf("%w: pricing snapshot candidate is invalid", ErrInvalidMediaPricingConfiguration)
		}
		if _, duplicate := seenAccounts[candidate.AccountID]; duplicate {
			return fmt.Errorf("%w: duplicate pricing snapshot candidate account %d", ErrInvalidMediaPricingConfiguration, candidate.AccountID)
		}
		seenAccounts[candidate.AccountID] = struct{}{}
	}
	if snapshot.UserMultiplierOverridden != (snapshot.UserMultiplier != nil) {
		return fmt.Errorf("%w: user multiplier presence is inconsistent", ErrInvalidMediaPricingConfiguration)
	}
	if err := validateMediaPricingMultiplier(snapshot.GroupDefaultMultiplier, "group default"); err != nil {
		return err
	}
	if snapshot.UserMultiplier != nil {
		if err := validateMediaPricingMultiplier(*snapshot.UserMultiplier, "user override"); err != nil {
			return err
		}
	}
	if err := validateMediaPricingMultiplier(snapshot.EffectiveMultiplier, "effective"); err != nil {
		return err
	}
	expectedMultiplier := snapshot.GroupDefaultMultiplier
	if snapshot.UserMultiplier != nil {
		expectedMultiplier = *snapshot.UserMultiplier
	}
	if !decimal.NewFromFloat(snapshot.EffectiveMultiplier).Equal(decimal.NewFromFloat(expectedMultiplier)) {
		return fmt.Errorf("%w: effective multiplier is inconsistent", ErrInvalidMediaPricingConfiguration)
	}
	if snapshot.DefaultUnitPrice != nil {
		if err := validateMediaUnitPrice(*snapshot.DefaultUnitPrice, "default"); err != nil {
			return err
		}
	}
	seenTiers := make(map[string]struct{}, len(snapshot.Tiers))
	for index := range snapshot.Tiers {
		tier := snapshot.Tiers[index]
		if tier.TierLabel == "" || tier.TierLabel != normalizeMediaPricingTier(tier.TierLabel) {
			return fmt.Errorf("%w: pricing snapshot tier is invalid", ErrInvalidMediaPricingConfiguration)
		}
		if _, duplicate := seenTiers[tier.TierLabel]; duplicate {
			return fmt.Errorf("%w: duplicate pricing snapshot tier %q", ErrInvalidMediaPricingConfiguration, tier.TierLabel)
		}
		if err := validateMediaUnitPrice(tier.UnitPrice, "tier "+tier.TierLabel); err != nil {
			return err
		}
		seenTiers[tier.TierLabel] = struct{}{}
	}
	selectedPrice, selectedTier, ok := resolveFrozenMediaUnitPrice(snapshot.DefaultUnitPrice, snapshot.Tiers, snapshot.EstimatedTier)
	if !ok || selectedTier != snapshot.EstimatedTier ||
		!decimal.NewFromFloat(selectedPrice).Equal(decimal.NewFromFloat(snapshot.EstimatedUnitPrice)) {
		return fmt.Errorf("%w: estimated unit price is inconsistent", ErrInvalidMediaPricingConfiguration)
	}
	base, err := calculateFrozenMediaAmount(snapshot.EstimatedUnitPrice, snapshot.EstimatedQuantity, 1)
	if err != nil {
		return err
	}
	if !decimal.NewFromFloat(base).Equal(decimal.NewFromFloat(snapshot.BaseEstimatedAmount)) {
		return fmt.Errorf("%w: base estimated amount is inconsistent", ErrInvalidMediaPricingConfiguration)
	}
	switch snapshot.BillingUnit {
	case MediaBillingUnitImage:
		if snapshot.BillingMode != BillingModeImage || snapshot.MediaType != MediaTypeImage ||
			snapshot.EstimatedQuantity > MaxMediaImageCount || math.Trunc(snapshot.EstimatedQuantity) != snapshot.EstimatedQuantity {
			return fmt.Errorf("%w: image pricing mode is inconsistent", ErrInvalidMediaPricingConfiguration)
		}
	case MediaBillingUnitSecond:
		if snapshot.BillingMode != BillingModePerRequest || snapshot.MediaType != MediaTypeVideo ||
			snapshot.EstimatedQuantity > MaxMediaVideoDurationSeconds {
			return fmt.Errorf("%w: second pricing mode is inconsistent", ErrInvalidMediaPricingConfiguration)
		}
	default:
		return fmt.Errorf("%w: unsupported frozen billing unit %q", ErrInvalidMediaPricingConfiguration, snapshot.BillingUnit)
	}
	return nil
}

// CalculateMediaPricingSettlement settles actual media usage exclusively from
// the immutable task snapshot. Missing or invalid actual quantities fail
// closed; an empty actual tier reuses the request's frozen selected tier/price.
func CalculateMediaPricingSettlement(snapshot MediaBillingSnapshot, usage MediaUsage) (MediaPricingSettlement, error) {
	frozen, err := DecodeMediaPricingSnapshot(snapshot.PricingSnapshot)
	if err != nil {
		return MediaPricingSettlement{}, err
	}
	if err := validateMediaPricingMultiplier(snapshot.GroupMultiplier, "outer"); err != nil {
		return MediaPricingSettlement{}, err
	}
	if !decimal.NewFromFloat(snapshot.GroupMultiplier).Equal(decimal.NewFromFloat(frozen.EffectiveMultiplier)) {
		return MediaPricingSettlement{}, fmt.Errorf("%w: outer group multiplier differs from frozen pricing", ErrInvalidMediaPricingConfiguration)
	}
	estimated, err := calculateFrozenMediaAmount(frozen.EstimatedUnitPrice, frozen.EstimatedQuantity, frozen.EffectiveMultiplier)
	if err != nil {
		return MediaPricingSettlement{}, err
	}
	normalizedOuterEstimate, err := normalizeMediaAmount(snapshot.EstimatedAmount)
	if err != nil || !normalizedOuterEstimate.Equal(decimal.NewFromFloat(estimated)) {
		return MediaPricingSettlement{}, fmt.Errorf("%w: outer estimate differs from frozen pricing", ErrInvalidMediaPricingConfiguration)
	}

	quantity := 0.0
	actualTier := ""
	switch frozen.BillingUnit {
	case MediaBillingUnitImage:
		if usage.ImageCount <= 0 || usage.ImageCount > MaxMediaImageCount {
			return MediaPricingSettlement{}, fmt.Errorf("%w: image_count=%d", ErrInvalidMediaPricingUsage, usage.ImageCount)
		}
		quantity = float64(usage.ImageCount)
		actualTier = strings.TrimSpace(usage.ImageSize)
	case MediaBillingUnitSecond:
		if usage.VideoSeconds <= 0 || usage.VideoSeconds > MaxMediaVideoDurationSeconds ||
			math.IsNaN(usage.VideoSeconds) || math.IsInf(usage.VideoSeconds, 0) {
			return MediaPricingSettlement{}, fmt.Errorf("%w: video_seconds=%v", ErrInvalidMediaPricingUsage, usage.VideoSeconds)
		}
		quantity = usage.VideoSeconds
		actualTier = strings.TrimSpace(usage.VideoResolution)
	default:
		return MediaPricingSettlement{}, fmt.Errorf("%w: billing_unit=%q", ErrInvalidMediaPricingUsage, frozen.BillingUnit)
	}

	unitPrice := frozen.EstimatedUnitPrice
	settledTier := frozen.EstimatedTier
	if actualTier != "" {
		var ok bool
		unitPrice, settledTier, ok = resolveFrozenMediaUnitPrice(frozen.DefaultUnitPrice, frozen.Tiers, actualTier)
		if !ok {
			return MediaPricingSettlement{}, ErrMediaPricingUnavailable.WithMetadata(map[string]string{
				"model": frozen.CanonicalModel,
				"tier":  actualTier,
			})
		}
	}
	accountBaseAmount, err := calculateFrozenMediaAmount(unitPrice, quantity, 1)
	if err != nil {
		return MediaPricingSettlement{}, fmt.Errorf("calculate media account base amount: %w", err)
	}
	userAmount, err := calculateFrozenMediaAmount(unitPrice, quantity, frozen.EffectiveMultiplier)
	if err != nil {
		return MediaPricingSettlement{}, fmt.Errorf("calculate media user amount: %w", err)
	}
	return MediaPricingSettlement{
		BillingUnit:       frozen.BillingUnit,
		Quantity:          quantity,
		Tier:              settledTier,
		UnitPrice:         unitPrice,
		AccountBaseAmount: accountBaseAmount,
		UserAmount:        userAmount,
		EffectiveRate:     frozen.EffectiveMultiplier,
	}, nil
}

// CalculateMediaUsageAmount is a compatibility convenience for callers that
// only need the final user charge. New billing code should use
// CalculateMediaPricingSettlement to retain account-side cost accounting.
func CalculateMediaUsageAmount(snapshot MediaBillingSnapshot, usage MediaUsage) (float64, error) {
	settlement, err := CalculateMediaPricingSettlement(snapshot, usage)
	if err != nil {
		return 0, err
	}
	return settlement.UserAmount, nil
}

var _ MediaPricingPort = (*ProductionMediaPricing)(nil)
