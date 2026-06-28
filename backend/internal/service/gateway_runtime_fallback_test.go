//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGatewayServiceResolveRuntimeFallbackGroup(t *testing.T) {
	entryID := int64(10)
	fallbackID := int64(20)

	tests := []struct {
		name        string
		group       *Group
		repoGroups  map[int64]*Group
		wantNil     bool
		wantErr     string
		wantCalls   int
		wantGroupID int64
	}{
		{
			name:       "未配置 fallback group 返回 nil",
			group:      &Group{ID: entryID, Platform: PlatformOpenAI},
			repoGroups: map[int64]*Group{},
			wantNil:    true,
			wantCalls:  0,
		},
		{
			name:  "成功解析可用 fallback group",
			group: &Group{ID: entryID, Platform: PlatformOpenAI, FallbackGroupID: &fallbackID},
			repoGroups: map[int64]*Group{
				fallbackID: {
					ID:               fallbackID,
					Platform:         PlatformOpenAI,
					Status:           StatusActive,
					SubscriptionType: SubscriptionTypeStandard,
					IsFallbackGroup:  true,
				},
			},
			wantCalls:   1,
			wantGroupID: fallbackID,
		},
		{
			name:  "fallback group inactive",
			group: &Group{ID: entryID, Platform: PlatformOpenAI, FallbackGroupID: &fallbackID},
			repoGroups: map[int64]*Group{
				fallbackID: {
					ID:               fallbackID,
					Platform:         PlatformOpenAI,
					Status:           StatusDisabled,
					SubscriptionType: SubscriptionTypeStandard,
					IsFallbackGroup:  true,
				},
			},
			wantErr:   "fallback group must be active",
			wantCalls: 1,
		},
		{
			name:  "fallback group must be standard",
			group: &Group{ID: entryID, Platform: PlatformOpenAI, FallbackGroupID: &fallbackID},
			repoGroups: map[int64]*Group{
				fallbackID: {
					ID:               fallbackID,
					Platform:         PlatformOpenAI,
					Status:           StatusActive,
					SubscriptionType: SubscriptionTypeSubscription,
					IsFallbackGroup:  true,
				},
			},
			wantErr:   "fallback group must be standard billing type",
			wantCalls: 1,
		},
		{
			name:  "fallback group must be enabled",
			group: &Group{ID: entryID, Platform: PlatformOpenAI, FallbackGroupID: &fallbackID},
			repoGroups: map[int64]*Group{
				fallbackID: {
					ID:               fallbackID,
					Platform:         PlatformOpenAI,
					Status:           StatusActive,
					SubscriptionType: SubscriptionTypeStandard,
				},
			},
			wantErr:   "fallback group must be enabled as fallback group",
			wantCalls: 1,
		},
		{
			name:  "fallback group platform mismatch",
			group: &Group{ID: entryID, Platform: PlatformOpenAI, FallbackGroupID: &fallbackID},
			repoGroups: map[int64]*Group{
				fallbackID: {
					ID:               fallbackID,
					Platform:         PlatformAnthropic,
					Status:           StatusActive,
					SubscriptionType: SubscriptionTypeStandard,
					IsFallbackGroup:  true,
				},
			},
			wantErr:   "fallback group platform mismatch",
			wantCalls: 1,
		},
		{
			name:  "fallback group cannot chain",
			group: &Group{ID: entryID, Platform: PlatformOpenAI, FallbackGroupID: &fallbackID},
			repoGroups: map[int64]*Group{
				fallbackID: {
					ID:               fallbackID,
					Platform:         PlatformOpenAI,
					Status:           StatusActive,
					SubscriptionType: SubscriptionTypeStandard,
					IsFallbackGroup:  true,
					FallbackGroupID:  ptr(int64(30)),
				},
			},
			wantErr:   "fallback group cannot have fallback_group_id configured",
			wantCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			groupRepo := &mockGroupRepoForGateway{groups: tt.repoGroups}
			svc := &GatewayService{groupRepo: groupRepo}

			got, err := svc.ResolveRuntimeFallbackGroup(context.Background(), tt.group)

			if tt.wantErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErr)
				require.Nil(t, got)
			} else {
				require.NoError(t, err)
				if tt.wantNil {
					require.Nil(t, got)
				} else {
					require.NotNil(t, got)
					require.Equal(t, tt.wantGroupID, got.ID)
				}
			}
			require.Equal(t, tt.wantCalls, groupRepo.getByIDLiteCalls)
		})
	}
}

func TestOpenAIGatewayServiceResolveRuntimeFallbackGroup(t *testing.T) {
	entryID := int64(10)
	fallbackID := int64(20)

	t.Run("成功解析可用 fallback group", func(t *testing.T) {
		groupRepo := &mockGroupRepoForGateway{groups: map[int64]*Group{
			fallbackID: {
				ID:               fallbackID,
				Platform:         PlatformOpenAI,
				Status:           StatusActive,
				SubscriptionType: SubscriptionTypeStandard,
				IsFallbackGroup:  true,
			},
		}}
		svc := &OpenAIGatewayService{groupRepo: groupRepo}

		got, err := svc.ResolveRuntimeFallbackGroup(context.Background(), &Group{
			ID:              entryID,
			Platform:        PlatformOpenAI,
			FallbackGroupID: &fallbackID,
		})

		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, fallbackID, got.ID)
		require.Equal(t, 1, groupRepo.getByIDLiteCalls)
	})

	t.Run("group repo 为空时报错", func(t *testing.T) {
		svc := &OpenAIGatewayService{}

		got, err := svc.ResolveRuntimeFallbackGroup(context.Background(), &Group{
			ID:              entryID,
			Platform:        PlatformOpenAI,
			FallbackGroupID: &fallbackID,
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "group repository unavailable")
		require.Nil(t, got)
	})
}
