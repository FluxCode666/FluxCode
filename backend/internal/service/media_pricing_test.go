package service

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

type mediaPricingGroupStub struct {
	group *Group
	err   error
	calls int
}

func (s *mediaPricingGroupStub) GetByID(context.Context, int64) (*Group, error) {
	s.calls++
	return s.group, s.err
}

type mediaPricingChannelStub struct {
	pricing *ChannelModelPricing
	calls   int
	groupID int64
	model   string
}

func (s *mediaPricingChannelStub) GetChannelModelPricing(_ context.Context, groupID int64, model string) *ChannelModelPricing {
	s.calls++
	s.groupID = groupID
	s.model = model
	return s.pricing
}

type mediaPricingUserRateStub struct {
	rate  *float64
	err   error
	calls int
}

func (s *mediaPricingUserRateStub) GetByUserAndGroup(context.Context, int64, int64) (*float64, error) {
	s.calls++
	return s.rate, s.err
}

type mediaPricingFixture struct {
	pricing    *ProductionMediaPricing
	groups     *mediaPricingGroupStub
	channels   *mediaPricingChannelStub
	userRates  *mediaPricingUserRateStub
	req        MediaCreateRequest
	definition MediaModelDefinition
	candidates []MediaAccountCandidateSnapshot
}

func newMediaPricingFixture() *mediaPricingFixture {
	defaultPrice := 0.09
	price2K := 0.08
	price4K := 0.16
	userRate := 0.5
	groups := &mediaPricingGroupStub{group: &Group{
		ID:             303,
		Platform:       PlatformMedia,
		RateMultiplier: 1.25,
	}}
	channels := &mediaPricingChannelStub{pricing: &ChannelModelPricing{
		ID:              11,
		ChannelID:       22,
		Platform:        PlatformMedia,
		Models:          []string{"nano-banana-pro"},
		BillingMode:     BillingModeImage,
		PerRequestPrice: &defaultPrice,
		Intervals: []PricingInterval{
			{TierLabel: "4K", PerRequestPrice: &price4K},
			{TierLabel: "2K", PerRequestPrice: &price2K},
		},
	}}
	userRates := &mediaPricingUserRateStub{rate: &userRate}
	return &mediaPricingFixture{
		pricing:   NewProductionMediaPricing(groups, channels, userRates),
		groups:    groups,
		channels:  channels,
		userRates: userRates,
		req: MediaCreateRequest{
			UserID:         101,
			APIKeyID:       202,
			GroupID:        303,
			MediaType:      MediaTypeImage,
			Operation:      MediaOperationTextToImage,
			RequestedModel: "banana-alias",
			Spec: MediaSpec{Image: &ImageSpec{
				Prompt: "draw a banana",
				Size:   "2K",
				Count:  3,
			}},
		},
		definition: MediaModelDefinition{
			ID:          404,
			ModelID:     "nano-banana-pro",
			Vendor:      "google",
			MediaType:   MediaTypeImage,
			Operations:  []MediaOperation{MediaOperationTextToImage},
			BillingUnit: MediaBillingUnitImage,
		},
		candidates: []MediaAccountCandidateSnapshot{
			{AccountID: 1, ResolvedModel: ResolvedMediaAccountModel{Provider: "gemini", UpstreamModel: "gemini-3-pro-image-preview"}},
			{AccountID: 2, ResolvedModel: ResolvedMediaAccountModel{Provider: "vertex", UpstreamModel: "gemini-3-pro-image-preview"}},
			{AccountID: 3, ResolvedModel: ResolvedMediaAccountModel{Provider: "relay", UpstreamModel: "vendor-image-v2"}},
		},
	}
}

func TestProductionMediaPricingSnapshotsImagePriceAndActualUsage(t *testing.T) {
	fixture := newMediaPricingFixture()

	snapshot, err := fixture.pricing.Snapshot(context.Background(), fixture.req, &fixture.definition, fixture.candidates)
	require.NoError(t, err)
	require.Equal(t, "banana-alias", snapshot.RequestedModel)
	require.Equal(t, []string{"gemini-3-pro-image-preview", "vendor-image-v2"}, snapshot.CandidateModels)
	require.InDelta(t, 0.12, snapshot.EstimatedAmount, 1e-12)
	require.InDelta(t, 0.5, snapshot.GroupMultiplier, 1e-12)
	require.Equal(t, int64(303), fixture.channels.groupID)
	require.Equal(t, "nano-banana-pro", fixture.channels.model)

	frozen, err := DecodeMediaPricingSnapshot(snapshot.PricingSnapshot)
	require.NoError(t, err)
	require.Equal(t, MediaPricingSnapshotVersion, frozen.Version)
	require.Equal(t, MediaPricingSourceChannel, frozen.Source)
	require.Equal(t, MediaPricingCurrencyUSD, frozen.Currency)
	require.Equal(t, int64(22), frozen.ChannelID)
	require.Equal(t, int64(11), frozen.PricingID)
	require.Equal(t, "banana-alias", frozen.RequestedModel)
	require.Equal(t, "nano-banana-pro", frozen.CanonicalModel)
	require.Equal(t, "google", frozen.Vendor)
	require.Equal(t, MediaBillingUnitImage, frozen.BillingUnit)
	require.Equal(t, BillingModeImage, frozen.BillingMode)
	require.InDelta(t, 3, frozen.EstimatedQuantity, 1e-12)
	require.Equal(t, "2k", frozen.EstimatedTier)
	require.InDelta(t, 0.08, frozen.EstimatedUnitPrice, 1e-12)
	require.InDelta(t, 0.24, frozen.BaseEstimatedAmount, 1e-12)
	require.InDelta(t, 1.25, frozen.GroupDefaultMultiplier, 1e-12)
	require.True(t, frozen.UserMultiplierOverridden)
	require.NotNil(t, frozen.UserMultiplier)
	require.InDelta(t, 0.5, *frozen.UserMultiplier, 1e-12)
	require.Equal(t, []MediaPricingTierSnapshot{
		{TierLabel: "2k", UnitPrice: 0.08},
		{TierLabel: "4k", UnitPrice: 0.16},
	}, frozen.Tiers)
	require.Len(t, frozen.Candidates, 3)

	settlement, err := CalculateMediaPricingSettlement(snapshot, MediaUsage{ImageCount: 2, ImageSize: "4K"})
	require.NoError(t, err)
	require.Equal(t, MediaBillingUnitImage, settlement.BillingUnit)
	require.InDelta(t, 2, settlement.Quantity, 1e-12)
	require.Equal(t, "4k", settlement.Tier)
	require.InDelta(t, 0.16, settlement.UnitPrice, 1e-12)
	require.InDelta(t, 0.32, settlement.AccountBaseAmount, 1e-12)
	require.InDelta(t, 0.16, settlement.UserAmount, 1e-12)
	require.InDelta(t, 0.5, settlement.EffectiveRate, 1e-12)

	actualWithoutSize, err := CalculateMediaUsageAmount(snapshot, MediaUsage{ImageCount: 2})
	require.NoError(t, err)
	require.InDelta(t, 0.08, actualWithoutSize, 1e-12)
}

func TestProductionMediaPricingSnapshotIsIndependentFromMutableConfiguration(t *testing.T) {
	fixture := newMediaPricingFixture()
	snapshot, err := fixture.pricing.Snapshot(context.Background(), fixture.req, &fixture.definition, fixture.candidates)
	require.NoError(t, err)

	*fixture.channels.pricing.PerRequestPrice = 99
	*fixture.channels.pricing.Intervals[0].PerRequestPrice = 88
	fixture.groups.group.RateMultiplier = 77
	*fixture.userRates.rate = 66
	fixture.candidates[0].ResolvedModel.UpstreamModel = "mutated"

	frozen, err := DecodeMediaPricingSnapshot(snapshot.PricingSnapshot)
	require.NoError(t, err)
	require.InDelta(t, 0.09, *frozen.DefaultUnitPrice, 1e-12)
	require.InDelta(t, 0.16, frozen.Tiers[1].UnitPrice, 1e-12)
	require.InDelta(t, 1.25, frozen.GroupDefaultMultiplier, 1e-12)
	require.InDelta(t, 0.5, frozen.EffectiveMultiplier, 1e-12)
	require.Equal(t, "gemini-3-pro-image-preview", frozen.Candidates[0].UpstreamModel)
}

func TestProductionMediaPricingPreservesConfiguredZeroValues(t *testing.T) {
	t.Run("explicit zero user override", func(t *testing.T) {
		fixture := newMediaPricingFixture()
		zero := 0.0
		fixture.userRates.rate = &zero
		fixture.channels.pricing.PerRequestPrice = nil
		fixture.req.Spec.Image.Size = "4K"
		fixture.channels.pricing.Intervals[0].PerRequestPrice = &zero

		snapshot, err := fixture.pricing.Snapshot(context.Background(), fixture.req, &fixture.definition, fixture.candidates)
		require.NoError(t, err)
		require.Zero(t, snapshot.EstimatedAmount)
		require.Zero(t, snapshot.GroupMultiplier)

		frozen, err := DecodeMediaPricingSnapshot(snapshot.PricingSnapshot)
		require.NoError(t, err)
		require.Nil(t, frozen.DefaultUnitPrice)
		require.True(t, frozen.UserMultiplierOverridden)
		require.NotNil(t, frozen.UserMultiplier)
		require.Zero(t, *frozen.UserMultiplier)
		require.Zero(t, frozen.EstimatedUnitPrice)
	})

	t.Run("zero free-subscription group without override", func(t *testing.T) {
		fixture := newMediaPricingFixture()
		fixture.groups.group.RateMultiplier = 0
		fixture.groups.group.SubscriptionType = SubscriptionTypeSubscription
		fixture.userRates.rate = nil

		snapshot, err := fixture.pricing.Snapshot(context.Background(), fixture.req, &fixture.definition, fixture.candidates)
		require.NoError(t, err)
		require.Zero(t, snapshot.EstimatedAmount)

		frozen, err := DecodeMediaPricingSnapshot(snapshot.PricingSnapshot)
		require.NoError(t, err)
		require.False(t, frozen.UserMultiplierOverridden)
		require.Nil(t, frozen.UserMultiplier)
		require.Zero(t, frozen.GroupDefaultMultiplier)
		require.Zero(t, frozen.EffectiveMultiplier)
		require.InDelta(t, 0.08, frozen.EstimatedUnitPrice, 1e-12)

		settlement, err := CalculateMediaPricingSettlement(snapshot, MediaUsage{ImageCount: 1, ImageSize: "2K"})
		require.NoError(t, err)
		require.InDelta(t, 0.08, settlement.AccountBaseAmount, 1e-12)
		require.Zero(t, settlement.UserAmount)
	})
}

func TestProductionMediaPricingSettlesAutoZeroEstimateAtActual4KTier(t *testing.T) {
	fixture := newMediaPricingFixture()
	zero := 0.0
	fixture.channels.pricing.PerRequestPrice = &zero
	fixture.groups.group.RateMultiplier = 1
	fixture.userRates.rate = nil
	fixture.req.Spec.Image.Size = "auto"
	fixture.req.Spec.Image.Count = 1

	snapshot, err := fixture.pricing.Snapshot(context.Background(), fixture.req, &fixture.definition, fixture.candidates)
	require.NoError(t, err)
	require.Zero(t, snapshot.EstimatedAmount)

	frozen, err := DecodeMediaPricingSnapshot(snapshot.PricingSnapshot)
	require.NoError(t, err)
	require.Equal(t, "auto", frozen.EstimatedTier)
	require.Zero(t, frozen.EstimatedUnitPrice)

	settlement, err := CalculateMediaPricingSettlement(snapshot, MediaUsage{ImageCount: 1, ImageSize: "4K"})
	require.NoError(t, err)
	require.Equal(t, "4k", settlement.Tier)
	require.InDelta(t, 0.16, settlement.UnitPrice, 1e-12)
	require.InDelta(t, 0.16, settlement.AccountBaseAmount, 1e-12)
	require.InDelta(t, 0.16, settlement.UserAmount, 1e-12)
}

func TestProductionMediaPricingFailsClosedWhenPriceIsMissing(t *testing.T) {
	t.Run("model has no channel price even when group is free", func(t *testing.T) {
		fixture := newMediaPricingFixture()
		fixture.groups.group.RateMultiplier = 0
		fixture.channels.pricing = nil

		_, err := fixture.pricing.Snapshot(context.Background(), fixture.req, &fixture.definition, fixture.candidates)
		require.ErrorIs(t, err, ErrMediaPricingUnavailable)
		require.Zero(t, fixture.userRates.calls)
	})

	t.Run("requested tier is absent and there is no default", func(t *testing.T) {
		fixture := newMediaPricingFixture()
		fixture.channels.pricing.PerRequestPrice = nil
		fixture.req.Spec.Image.Size = "8K"

		_, err := fixture.pricing.Snapshot(context.Background(), fixture.req, &fixture.definition, fixture.candidates)
		require.ErrorIs(t, err, ErrMediaPricingUnavailable)
	})

	t.Run("empty size cannot select a tier-only price", func(t *testing.T) {
		fixture := newMediaPricingFixture()
		fixture.channels.pricing.PerRequestPrice = nil
		fixture.req.Spec.Image.Size = ""

		_, err := fixture.pricing.Snapshot(context.Background(), fixture.req, &fixture.definition, fixture.candidates)
		require.ErrorIs(t, err, ErrMediaPricingUnavailable)
	})
}

func TestProductionMediaPricingRejectsUnitsThatCannotBeEstimated(t *testing.T) {
	for _, unit := range []string{"", "token", "output-token", "output_token", "request"} {
		t.Run(unit, func(t *testing.T) {
			fixture := newMediaPricingFixture()
			fixture.definition.BillingUnit = unit

			_, err := fixture.pricing.Snapshot(context.Background(), fixture.req, &fixture.definition, fixture.candidates)
			require.ErrorIs(t, err, ErrMediaPricingUnsupportedUnit)
			require.Zero(t, fixture.groups.calls)
		})
	}
}

func TestProductionMediaPricingSupportsFrozenSecondPricing(t *testing.T) {
	fixture := newMediaPricingFixture()
	defaultPrice := 0.02
	price1080P := 0.03
	fixture.userRates.rate = nil
	fixture.groups.group.RateMultiplier = 2
	fixture.channels.pricing.BillingMode = BillingModePerRequest
	fixture.channels.pricing.PerRequestPrice = &defaultPrice
	fixture.channels.pricing.Intervals = []PricingInterval{{TierLabel: "1080P", PerRequestPrice: &price1080P}}
	fixture.definition.MediaType = MediaTypeVideo
	fixture.definition.Operations = []MediaOperation{MediaOperationTextToVideo}
	fixture.definition.BillingUnit = MediaBillingUnitSecond
	fixture.req.MediaType = MediaTypeVideo
	fixture.req.Operation = MediaOperationTextToVideo
	fixture.req.Spec = MediaSpec{Video: &VideoSpec{
		Prompt:          "animate",
		DurationSeconds: 8,
		Resolution:      "1080p",
	}}

	snapshot, err := fixture.pricing.Snapshot(context.Background(), fixture.req, &fixture.definition, fixture.candidates)
	require.NoError(t, err)
	require.InDelta(t, 0.48, snapshot.EstimatedAmount, 1e-12)

	frozen, err := DecodeMediaPricingSnapshot(snapshot.PricingSnapshot)
	require.NoError(t, err)
	require.Equal(t, MediaBillingUnitSecond, frozen.BillingUnit)
	require.Equal(t, BillingModePerRequest, frozen.BillingMode)
	require.InDelta(t, 8, frozen.EstimatedQuantity, 1e-12)
	require.Equal(t, "1080p", frozen.EstimatedTier)

	settlement, err := CalculateMediaPricingSettlement(snapshot, MediaUsage{VideoSeconds: 6.5, VideoResolution: "720P"})
	require.NoError(t, err)
	require.Equal(t, MediaBillingUnitSecond, settlement.BillingUnit)
	require.InDelta(t, 6.5, settlement.Quantity, 1e-12)
	require.Equal(t, "720p", settlement.Tier)
	require.InDelta(t, 0.02, settlement.UnitPrice, 1e-12)
	require.InDelta(t, 0.13, settlement.AccountBaseAmount, 1e-12)
	require.InDelta(t, 0.26, settlement.UserAmount, 1e-12)
}

func TestProductionMediaPricingRejectsImplicitVideoDuration(t *testing.T) {
	fixture := newMediaPricingFixture()
	fixture.definition.MediaType = MediaTypeVideo
	fixture.definition.Operations = []MediaOperation{MediaOperationTextToVideo}
	fixture.definition.BillingUnit = MediaBillingUnitSecond
	fixture.req.MediaType = MediaTypeVideo
	fixture.req.Operation = MediaOperationTextToVideo
	fixture.req.Spec = MediaSpec{Video: &VideoSpec{Prompt: "animate", Resolution: "1080p"}}

	_, err := fixture.pricing.Snapshot(context.Background(), fixture.req, &fixture.definition, fixture.candidates)
	require.ErrorIs(t, err, ErrMediaPricingUnsupportedUnit)
	require.Zero(t, fixture.groups.calls)
}

func TestProductionMediaPricingFailsOnRateLookupAndInvalidConfiguration(t *testing.T) {
	t.Run("user rate lookup error", func(t *testing.T) {
		fixture := newMediaPricingFixture()
		fixture.userRates.err = errors.New("database unavailable")

		_, err := fixture.pricing.Snapshot(context.Background(), fixture.req, &fixture.definition, fixture.candidates)
		require.ErrorContains(t, err, "load media user group rate")
		require.ErrorContains(t, err, "database unavailable")
	})

	t.Run("negative group multiplier", func(t *testing.T) {
		fixture := newMediaPricingFixture()
		fixture.groups.group.RateMultiplier = -1

		_, err := fixture.pricing.Snapshot(context.Background(), fixture.req, &fixture.definition, fixture.candidates)
		require.ErrorIs(t, err, ErrInvalidMediaPricingConfiguration)
	})

	t.Run("non-finite user multiplier", func(t *testing.T) {
		fixture := newMediaPricingFixture()
		nan := math.NaN()
		fixture.userRates.rate = &nan

		_, err := fixture.pricing.Snapshot(context.Background(), fixture.req, &fixture.definition, fixture.candidates)
		require.ErrorIs(t, err, ErrInvalidMediaPricingConfiguration)
	})

	t.Run("negative unit price", func(t *testing.T) {
		fixture := newMediaPricingFixture()
		negative := -0.01
		fixture.channels.pricing.PerRequestPrice = &negative

		_, err := fixture.pricing.Snapshot(context.Background(), fixture.req, &fixture.definition, fixture.candidates)
		require.ErrorIs(t, err, ErrInvalidMediaPricingConfiguration)
	})

	t.Run("billing mode mismatch", func(t *testing.T) {
		fixture := newMediaPricingFixture()
		fixture.channels.pricing.BillingMode = BillingModeToken

		_, err := fixture.pricing.Snapshot(context.Background(), fixture.req, &fixture.definition, fixture.candidates)
		require.ErrorIs(t, err, ErrInvalidMediaPricingConfiguration)
	})

	t.Run("duplicate candidate account", func(t *testing.T) {
		fixture := newMediaPricingFixture()
		fixture.candidates[1].AccountID = fixture.candidates[0].AccountID

		_, err := fixture.pricing.Snapshot(context.Background(), fixture.req, &fixture.definition, fixture.candidates)
		require.ErrorIs(t, err, ErrInvalidMediaPricingConfiguration)
	})

	t.Run("image count exceeds request limit", func(t *testing.T) {
		fixture := newMediaPricingFixture()
		fixture.req.Spec.Image.Count = MaxMediaImageCount + 1

		_, err := fixture.pricing.Snapshot(context.Background(), fixture.req, &fixture.definition, fixture.candidates)
		require.ErrorIs(t, err, ErrInvalidMediaPricingConfiguration)
	})
}

func TestProductionMediaPricingHonorsCanceledContextBeforeDependencies(t *testing.T) {
	fixture := newMediaPricingFixture()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := fixture.pricing.Snapshot(ctx, fixture.req, &fixture.definition, fixture.candidates)
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, fixture.groups.calls)
	require.Zero(t, fixture.channels.calls)
	require.Zero(t, fixture.userRates.calls)
}

func TestCalculateMediaUsageAmountRejectsInvalidOrTamperedUsage(t *testing.T) {
	fixture := newMediaPricingFixture()
	snapshot, err := fixture.pricing.Snapshot(context.Background(), fixture.req, &fixture.definition, fixture.candidates)
	require.NoError(t, err)

	_, err = CalculateMediaUsageAmount(snapshot, MediaUsage{})
	require.ErrorIs(t, err, ErrInvalidMediaPricingUsage)

	tampered := snapshot
	tampered.EstimatedAmount++
	_, err = CalculateMediaUsageAmount(tampered, MediaUsage{ImageCount: 1})
	require.ErrorIs(t, err, ErrInvalidMediaPricingConfiguration)

	tampered = snapshot
	tampered.GroupMultiplier++
	_, err = CalculateMediaUsageAmount(tampered, MediaUsage{ImageCount: 1})
	require.ErrorIs(t, err, ErrInvalidMediaPricingConfiguration)

	tampered = snapshot
	tampered.GroupMultiplier = math.NaN()
	require.NotPanics(t, func() {
		_, err = CalculateMediaUsageAmount(tampered, MediaUsage{ImageCount: 1})
	})
	require.ErrorIs(t, err, ErrInvalidMediaPricingConfiguration)
}
