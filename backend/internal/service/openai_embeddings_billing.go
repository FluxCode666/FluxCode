package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

var (
	ErrEmbeddingBillingUnavailable = errors.New("embedding billing is unavailable")
	ErrEmbeddingPricingInvalid     = errors.New("embedding pricing is invalid")
)

// EmbeddingBillingInput contains only metadata required for accounting. It
// deliberately has no request input or response body/vector field.
type EmbeddingBillingInput struct {
	Result             *EmbeddingForwardResult
	APIKey             *APIKey
	User               *User
	Subscription       *UserSubscription
	UserAgent          string
	IPAddress          string
	RequestPayloadHash string
	APIKeyService      APIKeyQuotaUpdater
}

// BillEmbedding atomically commits every billing effect and the type-4 usage
// row. Callers may expose Result.Body only after this method returns nil.
func (s *OpenAIGatewayService) BillEmbedding(ctx context.Context, input *EmbeddingBillingInput) error {
	if s == nil || input == nil || input.Result == nil || input.APIKey == nil || input.User == nil {
		return ErrEmbeddingBillingUnavailable
	}
	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		return ErrEmbeddingUnsupportedMode
	}
	if s.usageBillingRepo == nil {
		return ErrEmbeddingBillingUnavailable
	}

	result := input.Result
	if result.PromptTokens <= 0 {
		return ErrEmbeddingPricingInvalid
	}
	pricing, ok := result.Eligibility.PricingForPromptTokens(result.PromptTokens)
	if !ok || pricing == nil || pricing.InputPricePerToken <= 0 {
		return ErrEmbeddingPricingInvalid
	}

	apiKey := input.APIKey
	account := &result.Eligibility.Account
	multiplier := 1.0
	if s.cfg != nil {
		multiplier = s.cfg.Default.RateMultiplier
	}
	if apiKey.GroupID != nil && apiKey.Group != nil {
		resolver := s.userGroupRateResolver
		if resolver == nil {
			resolver = newUserGroupRateResolver(nil, nil, resolveUserGroupRateCacheTTL(s.cfg), nil, "service.openai_embedding")
		}
		multiplier = resolver.Resolve(ctx, input.User.ID, *apiKey.GroupID, apiKey.Group.RateMultiplier)
	}

	inputCost := float64(result.PromptTokens) * pricing.InputPricePerToken
	cost := &CostBreakdown{
		InputCost:   inputCost,
		TotalCost:   inputCost,
		ActualCost:  inputCost * multiplier,
		BillingMode: string(BillingModeToken),
	}
	isSubscriptionBilling := input.Subscription != nil && apiKey.Group != nil && apiKey.Group.IsSubscriptionType()
	billingType := BillingTypeBalance
	if isSubscriptionBilling {
		billingType = BillingTypeSubscription
	}

	billedAt := time.Now()
	requestID := resolveUsageBillingRequestID(ctx, "")
	durationMs := int(result.Duration.Milliseconds())
	accountMultiplier := account.BillingRateMultiplier()
	publicModel := strings.TrimSpace(result.Eligibility.PublicModel)
	upstreamModel := strings.TrimSpace(result.Eligibility.UpstreamModel)
	inboundEndpoint := "/v1/embeddings"
	upstreamEndpoint := "/v1/embeddings"
	billingMode := string(BillingModeToken)
	usage := &UsageLog{
		UserID:                input.User.ID,
		APIKeyID:              apiKey.ID,
		AccountID:             account.ID,
		TraceID:               resolveUsageLogTraceID(ctx),
		RequestID:             requestID,
		Model:                 publicModel,
		RequestedModel:        publicModel,
		UpstreamModel:         optionalNonEqualStringPtr(upstreamModel, publicModel),
		GroupID:               apiKey.GroupID,
		InputTokens:           result.PromptTokens,
		InputCost:             cost.InputCost,
		TotalCost:             cost.TotalCost,
		ActualCost:            cost.ActualCost,
		RateMultiplier:        multiplier,
		AccountRateMultiplier: &accountMultiplier,
		BillingType:           billingType,
		RequestType:           RequestTypeEmbedding,
		DurationMs:            &durationMs,
		InboundEndpoint:       &inboundEndpoint,
		UpstreamEndpoint:      &upstreamEndpoint,
		BillingMode:           &billingMode,
		CreatedAt:             billedAt,
	}
	if input.Subscription != nil {
		usage.SubscriptionID = &input.Subscription.ID
	}
	if userAgent := strings.TrimSpace(input.UserAgent); userAgent != "" {
		usage.UserAgent = &userAgent
	}
	if ipAddress := strings.TrimSpace(input.IPAddress); ipAddress != "" {
		usage.IPAddress = &ipAddress
	}
	usageFields := result.Eligibility.ChannelMapping.ToUsageFields(publicModel, upstreamModel)
	usage.ChannelID = optionalInt64Ptr(usageFields.ChannelID)
	usage.ModelMappingChain = optionalTrimmedStringPtr(usageFields.ModelMappingChain)
	if apiKey.GroupID != nil {
		applyAccountStatsCost(ctx, usage, s.channelService, s.billingService,
			account.ID, *apiKey.GroupID, upstreamModel, publicModel,
			UsageTokens{InputTokens: result.PromptTokens}, cost.TotalCost,
		)
	}

	params := &postUsageBillingParams{
		Cost:                  cost,
		User:                  input.User,
		APIKey:                apiKey,
		Account:               account,
		Subscription:          input.Subscription,
		BilledAt:              billedAt,
		RequestPayloadHash:    strings.TrimSpace(input.RequestPayloadHash),
		IsSubscriptionBill:    isSubscriptionBilling,
		AccountRateMultiplier: accountMultiplier,
		APIKeyService:         input.APIKeyService,
	}
	cmd := buildUsageBillingCommand(requestID, usage, params)
	if cmd == nil {
		return ErrEmbeddingBillingUnavailable
	}
	cmd.UsageLog = usage

	billingCtx, cancel := detachedBillingContext(ctx)
	defer cancel()
	applyResult, err := s.usageBillingRepo.Apply(billingCtx, cmd)
	if err != nil {
		return err
	}
	if applyResult == nil {
		return ErrEmbeddingBillingUnavailable
	}
	if applyResult.Applied {
		s.finalizeEmbeddingBilling(params, applyResult)
	} else if s.deferredService != nil {
		s.deferredService.ScheduleLastUsedUpdate(account.ID)
	}
	return nil
}

func (s *OpenAIGatewayService) finalizeEmbeddingBilling(params *postUsageBillingParams, result *UsageBillingApplyResult) {
	if s == nil || params == nil || params.Cost == nil {
		return
	}
	if s.billingCacheService != nil {
		if params.IsSubscriptionBill && params.Cost.TotalCost > 0 && params.User != nil && params.APIKey != nil && params.APIKey.GroupID != nil {
			s.billingCacheService.QueueUpdateSubscriptionUsage(params.User.ID, *params.APIKey.GroupID, params.Cost.TotalCost)
		} else if !params.IsSubscriptionBill && params.Cost.ActualCost > 0 && params.User != nil {
			s.billingCacheService.QueueDeductBalance(params.User.ID, params.Cost.ActualCost)
		}
		if params.Cost.ActualCost > 0 && params.APIKey != nil && params.APIKey.HasRateLimits() {
			s.billingCacheService.QueueUpdateAPIKeyRateLimitUsage(params.APIKey.ID, params.Cost.ActualCost)
		}
	}
	if s.deferredService != nil && params.Account != nil {
		s.deferredService.ScheduleLastUsedUpdate(params.Account.ID)
	}
	if s.balanceNotifyService != nil {
		go notifyBalanceLow(params, s.billingDeps(), result)
		go notifyAccountQuota(params, s.billingDeps(), result)
	}
}
