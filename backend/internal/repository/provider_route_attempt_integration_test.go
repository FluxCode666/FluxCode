package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestProviderRouteAttemptRepositoryPersistsRouteDimensionsWithoutPayload(t *testing.T) {
	_, client := newProviderRepositorySQLite(t)
	repo := NewProviderRouteAttemptRepository(client)
	startedAt := time.Now().UTC().Truncate(time.Millisecond)
	route := service.RouteIdentity{
		ProviderID: 10, ProviderVersion: 2, CapabilityID: 20, CapabilityVersion: 3,
		EndpointID: 30, EndpointVersion: 4,
		IngressProtocol:  service.ProtocolResponses,
		UpstreamProtocol: service.ProtocolChatCompletions,
		Adapter:          "responses_to_chat_completions", AdapterVersion: "v1",
	}

	err := repo.RecordProviderRouteAttempt(context.Background(), service.ProviderRouteAttempt{
		TraceID: "trace-1", GroupID: 40, LogicalModel: "deepseek-chat",
		IngressProtocol:  service.ProtocolResponses,
		UpstreamProtocol: service.ProtocolChatCompletions,
		Route:            route, Tier: service.RouteTierConversion,
		Outcome: service.ProviderRouteAttemptSucceeded, StatusCode: 200,
		UpstreamRequestID: "upstream-1", UpstreamModel: "deepseek-ai/DeepSeek-V3",
		WireProfile: service.WireProfileCanonical, ConversionUsed: true,
		BytesCommitted: 128, FinalReason: "completed",
		StartedAt: startedAt, Duration: 1250 * time.Millisecond,
	})
	require.NoError(t, err)

	rows, err := client.ProviderRouteAttempt.Query().All(context.Background())
	require.NoError(t, err)
	require.Len(t, rows, 1)
	got := rows[0]
	require.Equal(t, "trace-1", got.TraceID)
	require.Equal(t, route.String(), got.RouteIdentity)
	require.Equal(t, "deepseek-chat", got.LogicalModel)
	require.Equal(t, "deepseek-ai/DeepSeek-V3", got.UpstreamModel)
	require.Equal(t, string(service.ProtocolResponses), got.IngressProtocol)
	require.Equal(t, string(service.ProtocolChatCompletions), got.UpstreamProtocol)
	require.Equal(t, string(service.RouteTierConversion), got.RouteTier)
	require.True(t, got.ConversionUsed)
	require.Equal(t, int64(1250), got.DurationMs)
	require.Equal(t, int64(128), got.BytesCommitted)
	require.Equal(t, "completed", *got.FinalReason)
}

func TestProviderRouteAttemptRepositoryRejectsUnstructuredInvalidAttempt(t *testing.T) {
	_, client := newProviderRepositorySQLite(t)
	repo := NewProviderRouteAttemptRepository(client)

	err := repo.RecordProviderRouteAttempt(context.Background(), service.ProviderRouteAttempt{
		TraceID: "trace-invalid", GroupID: 1,
		IngressProtocol:  service.ProtocolResponses,
		UpstreamProtocol: service.ProtocolChatCompletions,
		Tier:             service.RouteTierConversion,
		Outcome:          service.ProviderRouteAttemptFailed,
	})
	require.Error(t, err)
	count, countErr := client.ProviderRouteAttempt.Query().Count(context.Background())
	require.NoError(t, countErr)
	require.Zero(t, count)
}

func TestProviderRouteAttemptRepositoryFiltersDiagnosticsDimensions(t *testing.T) {
	_, client := newProviderRepositorySQLite(t)
	repo := NewProviderRouteAttemptRepository(client)
	ctx := context.Background()
	for _, item := range []struct {
		trace    string
		provider int64
		ingress  service.ProtocolFamily
		tier     service.RouteTier
	}{
		{"native", 10, service.ProtocolChatCompletions, service.RouteTierNative},
		{"conversion", 20, service.ProtocolResponses, service.RouteTierConversion},
	} {
		route := service.RouteIdentity{ProviderID: item.provider, CapabilityID: item.provider + 1, EndpointID: item.provider + 2, IngressProtocol: item.ingress, UpstreamProtocol: service.ProtocolChatCompletions}
		require.NoError(t, repo.RecordProviderRouteAttempt(ctx, service.ProviderRouteAttempt{
			TraceID: item.trace, GroupID: 40, LogicalModel: "deepseek-chat", Route: route,
			IngressProtocol: item.ingress, UpstreamProtocol: service.ProtocolChatCompletions,
			Tier: item.tier, Outcome: service.ProviderRouteAttemptSucceeded,
		}))
	}
	items, err := repo.ListProviderRouteAttempts(ctx, service.ProviderRouteAttemptFilter{GroupID: 40, ProviderID: 20, IngressProtocol: service.ProtocolResponses, Tier: service.RouteTierConversion})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "conversion", items[0].TraceID)
	require.NotEmpty(t, items[0].RouteIdentity)
}
