package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type providerConcurrencyStub struct {
	loads    map[int64]*AccountLoadInfo
	results  map[int64]bool
	acquired []int64
}

func (s *providerConcurrencyStub) GetAccountsLoadBatch(context.Context, []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	return s.loads, nil
}

func (s *providerConcurrencyStub) AcquireAccountSlot(_ context.Context, accountID int64, _ int) (*AcquireResult, error) {
	s.acquired = append(s.acquired, accountID)
	acquired := true
	if s.results != nil {
		acquired = s.results[accountID]
	}
	return &AcquireResult{Acquired: acquired, ReleaseFunc: func() {}}, nil
}

func TestProviderSchedulerUsesStableRouteIdentityAsFinalTieBreaker(t *testing.T) {
	left := NewNativeRouteCandidate(routeCapability(2, 202, ProtocolChatCompletions, false), ProtocolChatCompletions)
	right := NewNativeRouteCandidate(routeCapability(1, 101, ProtocolChatCompletions, false), ProtocolChatCompletions)
	left.GroupPriority, right.GroupPriority = 10, 10
	left.Account.Priority, right.Account.Priority = 20, 20
	runtime := &providerConcurrencyStub{loads: map[int64]*AccountLoadInfo{
		1: {AccountID: 1, LoadRate: 0},
		2: {AccountID: 2, LoadRate: 0},
	}}
	scheduler := NewProviderScheduler(runtime)

	selection, err := scheduler.Select(context.Background(), ProviderScheduleRequest{
		Tier: RouteTierNative, Candidates: []RouteCandidate{left, right},
	})

	require.NoError(t, err)
	require.Equal(t, right.Identity, selection.Candidate.Identity)
}

func TestProviderSchedulerStickyCannotCrossTier(t *testing.T) {
	native := NewNativeRouteCandidate(routeCapability(2, 202, ProtocolResponses, false), ProtocolResponses)
	convertedCapability := routeCapability(1, 101, ProtocolChatCompletions, true)
	converted := NewConversionRouteCandidate(convertedCapability, ProtocolResponses, "responses_to_chat", "v1", "non_stream_text_v1")
	runtime := &providerConcurrencyStub{}
	scheduler := NewProviderScheduler(runtime)

	selection, err := scheduler.Select(context.Background(), ProviderScheduleRequest{
		Tier: RouteTierNative, Candidates: []RouteCandidate{native}, StickyRoute: &converted.Identity,
	})

	require.NoError(t, err)
	require.Equal(t, native.Identity, selection.Candidate.Identity)
}

func TestProviderSchedulerStickyAtCapacityFallsBackToAnotherCandidate(t *testing.T) {
	sticky := NewNativeRouteCandidate(routeCapability(1, 101, ProtocolChatCompletions, false), ProtocolChatCompletions)
	fallback := NewNativeRouteCandidate(routeCapability(2, 202, ProtocolChatCompletions, false), ProtocolChatCompletions)
	runtime := &providerConcurrencyStub{
		loads: map[int64]*AccountLoadInfo{
			1: {AccountID: 1, LoadRate: 1},
			2: {AccountID: 2, LoadRate: 0},
		},
		results: map[int64]bool{1: false, 2: true},
	}
	scheduler := NewProviderScheduler(runtime)

	selection, err := scheduler.Select(context.Background(), ProviderScheduleRequest{
		Tier: RouteTierNative, Candidates: []RouteCandidate{sticky, fallback}, StickyRoute: &sticky.Identity,
	})

	require.NoError(t, err)
	require.Equal(t, fallback.Identity, selection.Candidate.Identity)
	require.Equal(t, []int64{1, 2}, runtime.acquired)
}

func TestRouteSwitchBudgetIsSharedAcrossTiers(t *testing.T) {
	budget := NewRouteSwitchBudget(2)

	require.True(t, budget.TrySwitch(RouteTierNative))
	require.True(t, budget.TrySwitch(RouteTierConversion))
	require.False(t, budget.TrySwitch(RouteTierConversion))
	require.Equal(t, 2, budget.Used())
}
