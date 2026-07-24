package integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type embeddingBillingCapture struct {
	command *service.UsageBillingCommand
}

func (r *embeddingBillingCapture) Apply(_ context.Context, command *service.UsageBillingCommand) (*service.UsageBillingApplyResult, error) {
	r.command = command
	return &service.UsageBillingApplyResult{Applied: true}, nil
}

func TestEmbeddingBillingCrossPackageContractIsAtomicAndContentFree(t *testing.T) {
	repository := &embeddingBillingCapture{}
	cfg := &config.Config{RunMode: config.RunModeStandard}
	cfg.Default.RateMultiplier = 1
	gateway := service.NewOpenAIGatewayService(
		nil, nil, nil, repository, nil, nil, nil, nil, cfg, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	groupID := int64(41)
	channelID := int64(59)
	price := 2e-6
	vectorCanary := `{"data":[{"embedding":[0.123456789]}]}`
	input := &service.EmbeddingBillingInput{
		Result: &service.EmbeddingForwardResult{
			Body:         []byte(vectorCanary),
			PromptTokens: 10,
			Duration:     125 * time.Millisecond,
			Eligibility: service.EmbeddingModelEligibility{
				Account: service.Account{
					ID:       71,
					Type:     service.AccountTypeAPIKey,
					Platform: service.PlatformEmbedding,
				},
				PublicModel:   "embed-public",
				BillingModel:  "embed-priced",
				UpstreamModel: "embed-upstream",
				ChannelMapping: service.ChannelMappingResult{
					ChannelID:          channelID,
					Mapped:             true,
					MappedModel:        "embed-priced",
					BillingModelSource: service.BillingModelSourceChannelMapped,
				},
				Pricing: &service.ResolvedPricing{
					Mode:        service.BillingModeToken,
					BasePricing: &service.ModelPricing{InputPricePerToken: price},
				},
			},
		},
		APIKey: &service.APIKey{
			ID:      23,
			UserID:  17,
			GroupID: &groupID,
			Group: &service.Group{
				ID:             groupID,
				Platform:       service.PlatformEmbedding,
				RateMultiplier: 1,
			},
		},
		User:               &service.User{ID: 17, Balance: 10},
		RequestPayloadHash: "input-payload-hash-only",
	}
	ctx := context.WithValue(context.Background(), ctxkey.ClientRequestID, "embedding-integration-request")

	require.NoError(t, gateway.BillEmbedding(ctx, input))
	require.NotNil(t, repository.command)
	require.NotNil(t, repository.command.UsageLog)
	require.Equal(t, service.RequestTypeEmbedding, repository.command.UsageLog.RequestType)
	require.Equal(t, "embed-public", repository.command.UsageLog.Model)
	require.Equal(t, 10, repository.command.UsageLog.InputTokens)
	require.NotNil(t, repository.command.UsageLog.ChannelID)
	require.Equal(t, channelID, *repository.command.UsageLog.ChannelID)
	require.InDelta(t, 0.00002, repository.command.BalanceCost, 1e-12)
	require.Equal(t, input.RequestPayloadHash, repository.command.RequestPayloadHash)

	encoded, err := json.Marshal(repository.command)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), vectorCanary)
	require.NotContains(t, string(encoded), "0.123456789")
}
