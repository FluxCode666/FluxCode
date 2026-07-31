package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

var (
	ErrProviderBillingUnavailable = errors.New("provider billing is unavailable")
	ErrProviderUsageIncomplete    = errors.New("provider usage is incomplete")
)

type ProviderRecordUsageInput struct {
	Result             *ProviderGatewayResult
	APIKey             *APIKey
	User               *User
	Subscription       *UserSubscription
	UserAgent          string
	IPAddress          string
	RequestPayloadHash string
	APIKeyService      APIKeyQuotaUpdater
}

// RecordProviderUsage atomically applies billing and writes the successful
// provider-route usage row. The command fingerprint makes a request billable
// at most once even when several route attempts preceded the success.
func (s *GatewayService) RecordProviderUsage(ctx context.Context, input *ProviderRecordUsageInput) error {
	if s == nil || input == nil || input.Result == nil || input.APIKey == nil ||
		input.User == nil || input.Result.Candidate.Account == nil || s.billingService == nil {
		return ErrProviderBillingUnavailable
	}
	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		if input.Result.Candidate.Identity.IngressProtocol == ProtocolEmbeddings {
			return ErrEmbeddingUnsupportedMode
		}
	}

	result := input.Result
	if !result.Usage.Complete {
		return ErrProviderUsageIncomplete
	}
	apiKey := input.APIKey
	account := result.Candidate.Account
	logicalModel := strings.TrimSpace(result.Candidate.LogicalModel.Name)
	upstreamModel := strings.TrimSpace(result.Candidate.Capability.UpstreamModel)
	multiplier := 1.0
	if s.cfg != nil {
		multiplier = s.cfg.Default.RateMultiplier
	}
	if apiKey.GroupID != nil && apiKey.Group != nil {
		multiplier = s.getUserGroupRateMultiplier(ctx, input.User.ID, *apiKey.GroupID, apiKey.Group.RateMultiplier)
	}

	ordinaryInputTokens := result.Usage.InputTokens
	if result.Candidate.Identity.UpstreamProtocol == ProtocolChatCompletions ||
		result.Candidate.Identity.UpstreamProtocol == ProtocolResponses {
		ordinaryInputTokens -= result.Usage.CacheCreationTokens + result.Usage.CacheReadTokens
		if ordinaryInputTokens < 0 {
			ordinaryInputTokens = 0
		}
	}
	tokens := UsageTokens{
		InputTokens: ordinaryInputTokens, OutputTokens: result.Usage.OutputTokens,
		CacheCreationTokens:   result.Usage.CacheCreationTokens,
		CacheReadTokens:       result.Usage.CacheReadTokens,
		CacheCreation5mTokens: result.Usage.CacheCreation5mTokens,
		CacheCreation1hTokens: result.Usage.CacheCreation1hTokens,
	}
	var (
		cost *CostBreakdown
		err  error
	)
	if s.resolver != nil && apiKey.Group != nil {
		groupID := apiKey.Group.ID
		cost, err = s.billingService.CalculateCostUnified(CostInput{
			Ctx: ctx, Model: logicalModel, GroupID: &groupID, Tokens: tokens,
			RequestCount: 1, RateMultiplier: multiplier, Resolver: s.resolver,
		})
	} else {
		cost, err = s.billingService.CalculateCost(logicalModel, tokens, multiplier)
	}
	if err != nil || cost == nil {
		if result.Candidate.Identity.IngressProtocol == ProtocolEmbeddings {
			return ErrEmbeddingPricingInvalid
		}
		cost = &CostBreakdown{ActualCost: 0}
	}

	isSubscriptionBilling := input.Subscription != nil && apiKey.Group != nil && apiKey.Group.IsSubscriptionType()
	billingType := BillingTypeBalance
	if isSubscriptionBilling {
		billingType = BillingTypeSubscription
	}
	billedAt := time.Now()
	durationMillis := int(result.Duration.Milliseconds())
	requestID := resolveUsageBillingRequestID(ctx, result.UpstreamRequestID)
	accountMultiplier := account.BillingRateMultiplier()
	billingMode := string(BillingModeToken)
	inboundEndpoint := result.Candidate.Identity.IngressProtocol.DefaultPath()
	upstreamEndpoint := result.Candidate.Identity.UpstreamProtocol.DefaultPath()
	if result.Candidate.Endpoint != nil && strings.TrimSpace(result.Candidate.Endpoint.Path) != "" {
		upstreamEndpoint = strings.TrimSpace(result.Candidate.Endpoint.Path)
	}
	usage := &UsageLog{
		UserID: input.User.ID, APIKeyID: apiKey.ID, AccountID: account.ID,
		TraceID: resolveUsageLogTraceID(ctx), RequestID: requestID,
		Model: logicalModel, RequestedModel: logicalModel,
		UpstreamModel:     optionalNonEqualStringPtr(upstreamModel, logicalModel),
		LogicalModel:      logicalModel,
		IngressProtocol:   result.Candidate.Identity.IngressProtocol,
		UpstreamProtocol:  result.Candidate.Identity.UpstreamProtocol,
		RouteIdentity:     result.Candidate.Identity.String(),
		WireProfile:       result.Candidate.Capability.WireProfile,
		ConversionUsed:    result.Converted,
		RawUpstreamUsage:  append([]byte(nil), result.Usage.Raw...),
		UsageCompleteness: providerUsageCompleteness(result.Usage),
		GroupID:           apiKey.GroupID, SubscriptionID: optionalSubscriptionID(input.Subscription),
		InputTokens: ordinaryInputTokens, OutputTokens: result.Usage.OutputTokens,
		CacheCreationTokens:   result.Usage.CacheCreationTokens,
		CacheReadTokens:       result.Usage.CacheReadTokens,
		CacheCreation5mTokens: result.Usage.CacheCreation5mTokens,
		CacheCreation1hTokens: result.Usage.CacheCreation1hTokens,
		InputCost:             cost.InputCost, OutputCost: cost.OutputCost,
		CacheCreationCost: cost.CacheCreationCost, CacheReadCost: cost.CacheReadCost,
		TotalCost: cost.TotalCost, ActualCost: cost.ActualCost,
		RateMultiplier: multiplier, AccountRateMultiplier: &accountMultiplier,
		BillingType: billingType, RequestType: RequestTypeSync,
		DurationMs: &durationMillis, InboundEndpoint: &inboundEndpoint,
		UpstreamEndpoint: &upstreamEndpoint, BillingMode: &billingMode,
		UserAgent: optionalTrimmedStringPtr(input.UserAgent),
		IPAddress: optionalTrimmedStringPtr(input.IPAddress), CreatedAt: billedAt,
	}
	if result.Candidate.Identity.IngressProtocol == ProtocolEmbeddings {
		usage.RequestType = RequestTypeEmbedding
	} else if result.Stream {
		usage.RequestType = RequestTypeStream
	}

	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		writeUsageLogBestEffort(ctx, s.usageLogRepo, usage, "service.provider_gateway")
		if s.deferredService != nil {
			s.deferredService.ScheduleLastUsedUpdate(account.ID)
		}
		return nil
	}
	if s.usageBillingRepo == nil {
		return ErrProviderBillingUnavailable
	}
	params := &postUsageBillingParams{
		Cost: cost, User: input.User, APIKey: apiKey, Account: account,
		Subscription: input.Subscription, BilledAt: billedAt,
		RequestPayloadHash:    resolveUsageBillingPayloadFingerprint(ctx, input.RequestPayloadHash),
		IsSubscriptionBill:    isSubscriptionBilling,
		AccountRateMultiplier: accountMultiplier, APIKeyService: input.APIKeyService,
	}
	command := buildUsageBillingCommand(requestID, usage, params)
	if command == nil {
		return ErrProviderBillingUnavailable
	}
	command.UsageLog = usage
	billingCtx, cancel := detachedBillingContext(ctx)
	defer cancel()
	applyResult, err := s.usageBillingRepo.Apply(billingCtx, command)
	if err != nil {
		return err
	}
	if applyResult == nil {
		return ErrProviderBillingUnavailable
	}
	if applyResult.Applied {
		finalizePostUsageBilling(params, s.billingDeps(), applyResult)
	} else if s.deferredService != nil {
		s.deferredService.ScheduleLastUsedUpdate(account.ID)
	}
	return nil
}

func providerUsageCompleteness(usage ProviderUsage) string {
	if usage.Complete {
		return "complete"
	}
	if len(usage.Raw) > 0 || usage.InputTokens > 0 || usage.OutputTokens > 0 ||
		usage.CacheCreationTokens > 0 || usage.CacheReadTokens > 0 {
		return "partial"
	}
	return "missing"
}
