package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

func TestCodexImageGenerationBridgeOverridePrecedence(t *testing.T) {
	groupID := int64(7)
	tests := []struct {
		name    string
		global  bool
		channel *Channel
		account *Account
		want    bool
	}{
		{name: "global enabled", global: true, account: &Account{Platform: PlatformOpenAI}, want: true},
		{name: "channel disables global", global: true, channel: &Channel{FeaturesConfig: map[string]any{"codex_image_generation_bridge": map[string]any{PlatformOpenAI: false}}}, account: &Account{Platform: PlatformOpenAI}, want: false},
		{name: "channel enables global disabled", global: false, channel: &Channel{FeaturesConfig: map[string]any{"codex_image_generation_bridge": map[string]any{PlatformOpenAI: true}}}, account: &Account{Platform: PlatformOpenAI}, want: true},
		{name: "account disables channel", global: true, channel: &Channel{FeaturesConfig: map[string]any{"codex_image_generation_bridge": true}}, account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{"codex_image_generation_bridge": false}}, want: false},
		{name: "account nested enables", global: false, account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{PlatformOpenAI: map[string]any{"codex_image_generation_bridge_enabled": true}}}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var channelService *ChannelService
			if tt.channel != nil {
				tt.channel.GroupIDs = []int64{groupID}
				tt.channel.Status = StatusActive
				channelService = NewChannelService(newCodexImageBridgeChannelRepo(*tt.channel, map[int64]string{groupID: PlatformOpenAI}), nil)
			}
			svc := &OpenAIGatewayService{
				cfg:            &config.Config{Gateway: config.GatewayConfig{CodexImageGenerationBridgeEnabled: tt.global}},
				channelService: channelService,
			}
			apiKey := &APIKey{GroupID: &groupID}
			if got := svc.isCodexImageGenerationBridgeEnabled(context.Background(), tt.account, apiKey); got != tt.want {
				t.Fatalf("isCodexImageGenerationBridgeEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

type codexImageBridgeChannelRepo struct {
	channel        Channel
	groupPlatforms map[int64]string
}

func newCodexImageBridgeChannelRepo(channel Channel, groupPlatforms map[int64]string) ChannelRepository {
	return &codexImageBridgeChannelRepo{
		channel:        channel,
		groupPlatforms: groupPlatforms,
	}
}

func (r *codexImageBridgeChannelRepo) Create(context.Context, *Channel) error { return nil }
func (r *codexImageBridgeChannelRepo) GetByID(context.Context, int64) (*Channel, error) {
	return nil, nil
}
func (r *codexImageBridgeChannelRepo) Update(context.Context, *Channel) error { return nil }
func (r *codexImageBridgeChannelRepo) Delete(context.Context, int64) error    { return nil }
func (r *codexImageBridgeChannelRepo) List(context.Context, pagination.PaginationParams, string, string) ([]Channel, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *codexImageBridgeChannelRepo) ListAll(context.Context) ([]Channel, error) {
	return []Channel{r.channel}, nil
}
func (r *codexImageBridgeChannelRepo) ExistsByName(context.Context, string) (bool, error) {
	return false, nil
}
func (r *codexImageBridgeChannelRepo) ExistsByNameExcluding(context.Context, string, int64) (bool, error) {
	return false, nil
}
func (r *codexImageBridgeChannelRepo) GetGroupIDs(context.Context, int64) ([]int64, error) {
	return nil, nil
}
func (r *codexImageBridgeChannelRepo) SetGroupIDs(context.Context, int64, []int64) error { return nil }
func (r *codexImageBridgeChannelRepo) GetChannelIDByGroupID(context.Context, int64) (int64, error) {
	return 0, nil
}
func (r *codexImageBridgeChannelRepo) GetGroupsInOtherChannels(context.Context, int64, []int64) ([]int64, error) {
	return nil, nil
}
func (r *codexImageBridgeChannelRepo) GetGroupPlatforms(context.Context, []int64) (map[int64]string, error) {
	return r.groupPlatforms, nil
}
func (r *codexImageBridgeChannelRepo) ListModelPricing(context.Context, int64) ([]ChannelModelPricing, error) {
	return nil, nil
}
func (r *codexImageBridgeChannelRepo) CreateModelPricing(context.Context, *ChannelModelPricing) error {
	return nil
}
func (r *codexImageBridgeChannelRepo) UpdateModelPricing(context.Context, *ChannelModelPricing) error {
	return nil
}
func (r *codexImageBridgeChannelRepo) DeleteModelPricing(context.Context, int64) error { return nil }
func (r *codexImageBridgeChannelRepo) ReplaceModelPricing(context.Context, int64, []ChannelModelPricing) error {
	return nil
}
