package service

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

type providerGatewayUpstreamResponse struct {
	status int
	body   string
}

type providerGatewayUpstreamStub struct {
	responses map[string][]providerGatewayUpstreamResponse
	requests  []*http.Request
	bodies    []string
}

func (s *providerGatewayUpstreamStub) Do(*http.Request, string, int64, int) (*http.Response, error) {
	panic("provider gateway must use pinned transport")
}

func (s *providerGatewayUpstreamStub) DoWithTLS(*http.Request, string, int64, int, *tlsfingerprint.Profile) (*http.Response, error) {
	panic("not used")
}

func (s *providerGatewayUpstreamStub) DoProvider(req *http.Request, _ ProviderUpstreamPolicy) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	s.requests = append(s.requests, req)
	s.bodies = append(s.bodies, string(body))
	queue := s.responses[req.URL.Host]
	if len(queue) == 0 {
		return &http.Response{StatusCode: http.StatusBadGateway, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"missing fixture"}}`))}, nil
	}
	response := queue[0]
	s.responses[req.URL.Host] = queue[1:]
	return &http.Response{
		StatusCode: response.status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(response.body)),
	}, nil
}

type providerAttemptRecorderStub struct {
	attempts []ProviderRouteAttempt
}

type providerRouteStateStoreStub struct {
	bindings map[string]ProviderRouteBinding
	sticky   map[RouteTier]RouteIdentity
}

func (s *providerRouteStateStoreStub) GetProviderStickyRoute(_ context.Context, _ int64, _ string, _ ProtocolFamily, tier RouteTier, _ string) (*RouteIdentity, error) {
	route, ok := s.sticky[tier]
	if !ok {
		return nil, nil
	}
	return &route, nil
}

func (s *providerRouteStateStoreStub) SetProviderStickyRoute(_ context.Context, _ int64, _ string, _ ProtocolFamily, tier RouteTier, _ string, route RouteIdentity, _ time.Duration) error {
	if s.sticky == nil {
		s.sticky = make(map[RouteTier]RouteIdentity)
	}
	s.sticky[tier] = route
	return nil
}

func (s *providerRouteStateStoreStub) GetProviderRouteBinding(_ context.Context, responseID string) (*ProviderRouteBinding, error) {
	binding, ok := s.bindings[responseID]
	if !ok {
		return nil, nil
	}
	return &binding, nil
}

func (s *providerRouteStateStoreStub) SetProviderRouteBinding(_ context.Context, responseID string, binding ProviderRouteBinding, _ time.Duration) error {
	if s.bindings == nil {
		s.bindings = make(map[string]ProviderRouteBinding)
	}
	s.bindings[responseID] = binding
	return nil
}

func (s *providerAttemptRecorderStub) RecordProviderRouteAttempt(_ context.Context, attempt ProviderRouteAttempt) error {
	s.attempts = append(s.attempts, attempt)
	return nil
}

func gatewayRouteCapability(providerID, capabilityID int64, protocol ProtocolFamily, allowConversion bool, host string) ProviderRouteCapability {
	capability := routeCapability(providerID, capabilityID, protocol, allowConversion)
	capability.Profile.Connection.BaseURL = "https://" + host
	capability.Profile.Connection.AuthType = "bearer"
	capability.Account.Credentials = map[string]any{"api_key": "secret"}
	return capability
}

func newProviderGatewayForTest(
	capabilities []ProviderRouteCapability,
	upstream *providerGatewayUpstreamStub,
	recorder ProviderRouteAttemptRecorder,
) *ProviderGatewayService {
	repo := &providerRouteRepositoryStub{capabilities: capabilities}
	resolver := NewProviderRouteResolver(repo, apicompat.NewRegistry())
	scheduler := NewProviderScheduler(nil)
	forwarder := NewProviderForwarder(upstream, ProviderForwarderOptions{})
	return NewProviderGatewayService(resolver, scheduler, forwarder, nil, recorder)
}

func TestProviderGatewayNativeFailureFallsThroughToConversionTier(t *testing.T) {
	capabilities := []ProviderRouteCapability{
		gatewayRouteCapability(1, 101, ProtocolResponses, false, "native.example"),
		gatewayRouteCapability(2, 202, ProtocolChatCompletions, true, "chat.example"),
	}
	upstream := &providerGatewayUpstreamStub{responses: map[string][]providerGatewayUpstreamResponse{
		"native.example": {{status: http.StatusServiceUnavailable, body: `{"error":{"message":"overloaded"}}`}},
		"chat.example": {{status: http.StatusOK, body: `{
			"id":"chatcmpl_2","object":"chat.completion","model":"vendor-model",
			"choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}
		}`}},
	}}
	recorder := &providerAttemptRecorderStub{}
	gateway := newProviderGatewayForTest(capabilities, upstream, recorder)
	restore := stubProviderDNS(t, net.ParseIP("203.0.113.20"))
	defer restore()

	result, err := gateway.Execute(context.Background(), ProviderGatewayRequest{
		GroupID: 1, LogicalModel: "model-a", Protocol: ProtocolResponses,
		Body:        []byte(`{"model":"model-a","input":"hello","store":false}`),
		MaxSwitches: 1,
	})

	require.NoError(t, err)
	require.True(t, result.Converted)
	require.Equal(t, RouteTierConversion, result.Candidate.Tier)
	require.Contains(t, string(result.Body), `"object":"response"`)
	require.Contains(t, string(result.Body), `"model":"model-a"`)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "/v1/responses", upstream.requests[0].URL.Path)
	require.Equal(t, "/v1/chat/completions", upstream.requests[1].URL.Path)
	require.Contains(t, upstream.bodies[1], `"messages"`)
	require.Len(t, recorder.attempts, 2)
	require.Equal(t, ProviderRouteAttemptFailed, recorder.attempts[0].Outcome)
	require.Equal(t, ProviderRouteAttemptSucceeded, recorder.attempts[1].Outcome)
}

func TestProviderGatewayDoesNotEnterConversionWhenSwitchBudgetIsExhausted(t *testing.T) {
	capabilities := []ProviderRouteCapability{
		gatewayRouteCapability(1, 101, ProtocolResponses, false, "native.example"),
		gatewayRouteCapability(2, 202, ProtocolChatCompletions, true, "chat.example"),
	}
	upstream := &providerGatewayUpstreamStub{responses: map[string][]providerGatewayUpstreamResponse{
		"native.example": {{status: http.StatusServiceUnavailable, body: `{"error":{}}`}},
		"chat.example":   {{status: http.StatusOK, body: `{}`}},
	}}
	gateway := newProviderGatewayForTest(capabilities, upstream, nil)
	restore := stubProviderDNS(t, net.ParseIP("203.0.113.21"))
	defer restore()

	_, err := gateway.Execute(context.Background(), ProviderGatewayRequest{
		GroupID: 1, LogicalModel: "model-a", Protocol: ProtocolResponses,
		Body: []byte(`{"model":"model-a","input":"hello"}`), MaxSwitches: 0,
	})

	require.ErrorIs(t, err, ErrProviderAttemptsExhausted)
	require.Len(t, upstream.requests, 1)
}

func TestProviderGatewayConversionDisabledNeverCallsUpstream(t *testing.T) {
	capabilities := []ProviderRouteCapability{
		gatewayRouteCapability(1, 101, ProtocolChatCompletions, false, "chat.example"),
	}
	upstream := &providerGatewayUpstreamStub{responses: map[string][]providerGatewayUpstreamResponse{}}
	gateway := newProviderGatewayForTest(capabilities, upstream, nil)

	_, err := gateway.Execute(context.Background(), ProviderGatewayRequest{
		GroupID: 1, LogicalModel: "model-a", Protocol: ProtocolResponses,
		Body: []byte(`{"model":"model-a","input":"hello"}`), MaxSwitches: 3,
	})

	require.ErrorIs(t, err, ErrNoProviderRoute)
	require.Empty(t, upstream.requests)
}

func TestProviderGatewayNativeChatStreamUsesChatEndpointAndRecordsCompletion(t *testing.T) {
	capability := gatewayRouteCapability(1, 101, ProtocolChatCompletions, false, "chat.example")
	capability.Capability.FeatureProfile = FeatureProfileStreamText
	upstream := &providerGatewayUpstreamStub{responses: map[string][]providerGatewayUpstreamResponse{
		"chat.example": {{status: http.StatusOK, body: "data: {\"id\":\"chatcmpl_1\",\"model\":\"vendor-model\",\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n" +
			"data: {\"id\":\"chatcmpl_1\",\"model\":\"vendor-model\",\"choices\":[],\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":2,\"total_tokens\":6}}\n\n" +
			"data: [DONE]\n\n"}},
	}}
	recorder := &providerAttemptRecorderStub{}
	state := &providerRouteStateStoreStub{}
	repo := &providerRouteRepositoryStub{capabilities: []ProviderRouteCapability{capability}}
	gateway := NewProviderGatewayService(
		NewProviderRouteResolver(repo, apicompat.NewRegistry()), NewProviderScheduler(nil),
		NewProviderForwarder(upstream, ProviderForwarderOptions{}), state, recorder,
	)
	restore := stubProviderDNS(t, net.ParseIP("203.0.113.24"))
	defer restore()

	result, err := gateway.ExecuteStream(context.Background(), ProviderGatewayRequest{
		GroupID: 1, LogicalModel: "model-a", Protocol: ProtocolChatCompletions,
		Body:        []byte(`{"model":"model-a","messages":[{"role":"user","content":"hello"}],"stream":true}`),
		SessionHash: "session-a", MaxSwitches: 1,
	})
	require.NoError(t, err)
	body, err := io.ReadAll(result.Body)
	require.NoError(t, err)
	require.NoError(t, result.Body.Close())
	require.Contains(t, string(body), `"model":"model-a"`)
	require.NotContains(t, string(body), `"model":"vendor-model"`)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "/v1/chat/completions", upstream.requests[0].URL.Path)
	require.Contains(t, upstream.bodies[0], `"model":"upstream-model"`)
	require.Equal(t, 4, result.Usage().InputTokens)
	require.Equal(t, 2, result.Usage().OutputTokens)
	require.True(t, result.Usage().Complete)
	require.Len(t, recorder.attempts, 1)
	require.Equal(t, ProviderRouteAttemptSucceeded, recorder.attempts[0].Outcome)
	require.Greater(t, recorder.attempts[0].BytesCommitted, int64(0))
	require.Contains(t, state.sticky, RouteTierNative)
}

func TestProviderGatewayResponsesContinuationUsesHardRouteBinding(t *testing.T) {
	capabilities := []ProviderRouteCapability{
		gatewayRouteCapability(1, 101, ProtocolResponses, false, "first.example"),
		gatewayRouteCapability(2, 202, ProtocolResponses, false, "second.example"),
	}
	upstream := &providerGatewayUpstreamStub{responses: map[string][]providerGatewayUpstreamResponse{
		"first.example":  {{status: http.StatusOK, body: `{"id":"resp_first","object":"response","status":"completed","model":"vendor-model","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`}},
		"second.example": {{status: http.StatusOK, body: `{"id":"resp_next","object":"response","status":"completed","model":"vendor-model","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`}},
	}}
	state := &providerRouteStateStoreStub{bindings: map[string]ProviderRouteBinding{
		"resp_previous": {
			Route:   NewRouteIdentity(capabilities[1], ProtocolResponses, "", ""),
			GroupID: 1, LogicalModel: "model-a",
		},
	}}
	repo := &providerRouteRepositoryStub{capabilities: capabilities}
	gateway := NewProviderGatewayService(
		NewProviderRouteResolver(repo, apicompat.NewRegistry()),
		NewProviderScheduler(nil),
		NewProviderForwarder(upstream, ProviderForwarderOptions{}),
		state,
		nil,
	)
	restore := stubProviderDNS(t, net.ParseIP("203.0.113.22"))
	defer restore()

	result, err := gateway.Execute(context.Background(), ProviderGatewayRequest{
		GroupID: 1, LogicalModel: "model-a", Protocol: ProtocolResponses,
		Body:        []byte(`{"model":"model-a","input":"next","previous_response_id":"resp_previous"}`),
		MaxSwitches: 2,
	})

	require.NoError(t, err)
	require.Equal(t, int64(2), result.Candidate.Identity.ProviderID)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "second.example", upstream.requests[0].URL.Host)
	require.Equal(t, result.Candidate.Identity, state.bindings["resp_next"].Route)
}

func TestProviderGatewayResponsesContinuationFailsWhenCapabilityVersionChanged(t *testing.T) {
	capability := gatewayRouteCapability(1, 101, ProtocolResponses, false, "first.example")
	stale := NewRouteIdentity(capability, ProtocolResponses, "", "")
	capability.Capability.Version++
	upstream := &providerGatewayUpstreamStub{responses: map[string][]providerGatewayUpstreamResponse{}}
	state := &providerRouteStateStoreStub{bindings: map[string]ProviderRouteBinding{
		"resp_stale": {Route: stale, GroupID: 1, LogicalModel: "model-a"},
	}}
	repo := &providerRouteRepositoryStub{capabilities: []ProviderRouteCapability{capability}}
	gateway := NewProviderGatewayService(
		NewProviderRouteResolver(repo, apicompat.NewRegistry()), NewProviderScheduler(nil),
		NewProviderForwarder(upstream, ProviderForwarderOptions{}), state, nil,
	)

	_, err := gateway.Execute(context.Background(), ProviderGatewayRequest{
		GroupID: 1, LogicalModel: "model-a", Protocol: ProtocolResponses,
		Body:        []byte(`{"model":"model-a","input":"next","previous_response_id":"resp_stale"}`),
		MaxSwitches: 2,
	})

	require.ErrorIs(t, err, ErrProviderContinuationUnavailable)
	require.Empty(t, upstream.requests)
}

func TestProviderGatewayResponsesContinuationIsTenantBound(t *testing.T) {
	capability := gatewayRouteCapability(1, 101, ProtocolResponses, false, "first.example")
	baseBinding := ProviderRouteBinding{
		Route:  NewRouteIdentity(capability, ProtocolResponses, "", ""),
		UserID: 11, APIKeyID: 22, GroupID: 33, LogicalModel: "model-a",
	}
	tests := []struct {
		name    string
		request ProviderGatewayRequest
	}{
		{name: "user", request: ProviderGatewayRequest{UserID: 12, APIKeyID: 22, GroupID: 33, LogicalModel: "model-a"}},
		{name: "api key", request: ProviderGatewayRequest{UserID: 11, APIKeyID: 23, GroupID: 33, LogicalModel: "model-a"}},
		{name: "group", request: ProviderGatewayRequest{UserID: 11, APIKeyID: 22, GroupID: 34, LogicalModel: "model-a"}},
		{name: "logical model", request: ProviderGatewayRequest{UserID: 11, APIKeyID: 22, GroupID: 33, LogicalModel: "model-b"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &providerGatewayUpstreamStub{responses: map[string][]providerGatewayUpstreamResponse{}}
			state := &providerRouteStateStoreStub{bindings: map[string]ProviderRouteBinding{"resp_previous": baseBinding}}
			gateway := NewProviderGatewayService(
				NewProviderRouteResolver(&providerRouteRepositoryStub{capabilities: []ProviderRouteCapability{capability}}, apicompat.NewRegistry()),
				NewProviderScheduler(nil), NewProviderForwarder(upstream, ProviderForwarderOptions{}), state, nil,
			)
			tt.request.Protocol = ProtocolResponses
			tt.request.Body = []byte(`{"model":"model-a","input":"next","previous_response_id":"resp_previous"}`)
			tt.request.MaxSwitches = 1

			_, err := gateway.Execute(context.Background(), tt.request)

			require.ErrorIs(t, err, ErrProviderContinuationUnavailable)
			require.Empty(t, upstream.requests)
		})
	}
}
