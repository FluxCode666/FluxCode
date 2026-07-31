//go:build integration

package repository

import (
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type GatewayCacheSuite struct {
	IntegrationRedisSuite
	cache service.GatewayCache
}

func (s *GatewayCacheSuite) SetupTest() {
	s.IntegrationRedisSuite.SetupTest()
	s.cache = NewGatewayCache(s.rdb)
}

func (s *GatewayCacheSuite) TestGetSessionAccountID_Missing() {
	_, err := s.cache.GetSessionAccountID(s.ctx, 1, "nonexistent")
	require.True(s.T(), errors.Is(err, redis.Nil), "expected redis.Nil for missing session")
}

func (s *GatewayCacheSuite) TestSetAndGetSessionAccountID() {
	sessionID := "s1"
	accountID := int64(99)
	groupID := int64(1)
	sessionTTL := 1 * time.Minute

	require.NoError(s.T(), s.cache.SetSessionAccountID(s.ctx, groupID, sessionID, accountID, sessionTTL), "SetSessionAccountID")

	sid, err := s.cache.GetSessionAccountID(s.ctx, groupID, sessionID)
	require.NoError(s.T(), err, "GetSessionAccountID")
	require.Equal(s.T(), accountID, sid, "session id mismatch")
}

func (s *GatewayCacheSuite) TestSessionAccountID_TTL() {
	sessionID := "s2"
	accountID := int64(100)
	groupID := int64(1)
	sessionTTL := 1 * time.Minute

	require.NoError(s.T(), s.cache.SetSessionAccountID(s.ctx, groupID, sessionID, accountID, sessionTTL), "SetSessionAccountID")

	sessionKey := buildSessionKey(groupID, sessionID)
	ttl, err := s.rdb.TTL(s.ctx, sessionKey).Result()
	require.NoError(s.T(), err, "TTL sessionKey after Set")
	s.AssertTTLWithin(ttl, 1*time.Second, sessionTTL)
}

func (s *GatewayCacheSuite) TestRefreshSessionTTL() {
	sessionID := "s3"
	accountID := int64(101)
	groupID := int64(1)
	initialTTL := 1 * time.Minute
	refreshTTL := 3 * time.Minute

	require.NoError(s.T(), s.cache.SetSessionAccountID(s.ctx, groupID, sessionID, accountID, initialTTL), "SetSessionAccountID")

	require.NoError(s.T(), s.cache.RefreshSessionTTL(s.ctx, groupID, sessionID, refreshTTL), "RefreshSessionTTL")

	sessionKey := buildSessionKey(groupID, sessionID)
	ttl, err := s.rdb.TTL(s.ctx, sessionKey).Result()
	require.NoError(s.T(), err, "TTL after Refresh")
	s.AssertTTLWithin(ttl, 1*time.Second, refreshTTL)
}

func (s *GatewayCacheSuite) TestRefreshSessionTTL_MissingKey() {
	// RefreshSessionTTL on a missing key should not error (no-op)
	err := s.cache.RefreshSessionTTL(s.ctx, 1, "missing-session", 1*time.Minute)
	require.NoError(s.T(), err, "RefreshSessionTTL on missing key should not error")
}

func (s *GatewayCacheSuite) TestDeleteSessionAccountID() {
	sessionID := "openai:s4"
	accountID := int64(102)
	groupID := int64(1)
	sessionTTL := 1 * time.Minute

	require.NoError(s.T(), s.cache.SetSessionAccountID(s.ctx, groupID, sessionID, accountID, sessionTTL), "SetSessionAccountID")
	require.NoError(s.T(), s.cache.DeleteSessionAccountID(s.ctx, groupID, sessionID), "DeleteSessionAccountID")

	_, err := s.cache.GetSessionAccountID(s.ctx, groupID, sessionID)
	require.True(s.T(), errors.Is(err, redis.Nil), "expected redis.Nil after delete")
}

func (s *GatewayCacheSuite) TestGetSessionAccountID_CorruptedValue() {
	sessionID := "corrupted"
	groupID := int64(1)
	sessionKey := buildSessionKey(groupID, sessionID)

	// Set a non-integer value
	require.NoError(s.T(), s.rdb.Set(s.ctx, sessionKey, "not-a-number", 1*time.Minute).Err(), "Set invalid value")

	_, err := s.cache.GetSessionAccountID(s.ctx, groupID, sessionID)
	require.Error(s.T(), err, "expected error for corrupted value")
	require.False(s.T(), errors.Is(err, redis.Nil), "expected parsing error, not redis.Nil")
}

func (s *GatewayCacheSuite) TestProviderStickyRouteRoundTripIsScopedByTier() {
	nativeRoute := service.RouteIdentity{
		ProviderID: 1, ProviderVersion: 2, CapabilityID: 3, CapabilityVersion: 4,
		IngressProtocol: service.ProtocolResponses, UpstreamProtocol: service.ProtocolResponses,
	}
	conversionRoute := service.RouteIdentity{
		ProviderID: 5, ProviderVersion: 6, CapabilityID: 7, CapabilityVersion: 8,
		IngressProtocol: service.ProtocolResponses, UpstreamProtocol: service.ProtocolChatCompletions,
		Adapter: "responses_to_chat", AdapterVersion: "v1",
	}

	require.NoError(s.T(), s.cache.(service.ProviderRouteStateStore).SetProviderStickyRoute(
		s.ctx, 9, "deepseek-chat", service.ProtocolResponses, service.RouteTierNative,
		"session", nativeRoute, time.Minute,
	))
	require.NoError(s.T(), s.cache.(service.ProviderRouteStateStore).SetProviderStickyRoute(
		s.ctx, 9, "deepseek-chat", service.ProtocolResponses, service.RouteTierConversion,
		"session", conversionRoute, time.Minute,
	))

	gotNative, err := s.cache.(service.ProviderRouteStateStore).GetProviderStickyRoute(
		s.ctx, 9, "deepseek-chat", service.ProtocolResponses, service.RouteTierNative, "session",
	)
	require.NoError(s.T(), err)
	require.Equal(s.T(), nativeRoute, *gotNative)

	gotConversion, err := s.cache.(service.ProviderRouteStateStore).GetProviderStickyRoute(
		s.ctx, 9, "deepseek-chat", service.ProtocolResponses, service.RouteTierConversion, "session",
	)
	require.NoError(s.T(), err)
	require.Equal(s.T(), conversionRoute, *gotConversion)
}

func (s *GatewayCacheSuite) TestProviderRouteBindingRoundTrip() {
	route := service.RouteIdentity{
		ProviderID: 11, ProviderVersion: 12, CapabilityID: 13, CapabilityVersion: 14,
		EndpointID: 15, EndpointVersion: 16,
		IngressProtocol: service.ProtocolResponses, UpstreamProtocol: service.ProtocolResponses,
	}
	store := s.cache.(service.ProviderRouteStateStore)

	binding := service.ProviderRouteBinding{Route: route, UserID: 1, APIKeyID: 2, GroupID: 3, LogicalModel: "model-a"}
	require.NoError(s.T(), store.SetProviderRouteBinding(s.ctx, "resp_123", binding, time.Minute))
	got, err := store.GetProviderRouteBinding(s.ctx, "resp_123")
	require.NoError(s.T(), err)
	require.Equal(s.T(), binding, *got)

	missing, err := store.GetProviderRouteBinding(s.ctx, "resp_missing")
	require.NoError(s.T(), err)
	require.Nil(s.T(), missing)
}

func TestGatewayCacheSuite(t *testing.T) {
	suite.Run(t, new(GatewayCacheSuite))
}
