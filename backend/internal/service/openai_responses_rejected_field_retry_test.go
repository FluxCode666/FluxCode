package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type rejectedFieldRetryUpstream struct {
	responses []*http.Response
	bodies    [][]byte
}

func (u *rejectedFieldRetryUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	_ = req.Body.Close()
	u.bodies = append(u.bodies, body)
	response := u.responses[0]
	u.responses = u.responses[1:]
	return response, nil
}

func (u *rejectedFieldRetryUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

func TestOpenAIGatewayService_RetriesExplicitRejectedNamespaceField(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","stream":false,"input":[{"type":"function_call","name":"tool","namespace":"remove","arguments":"{}"}]}`)
	upstream := &rejectedFieldRetryUpstream{responses: []*http.Response{
		newRejectedFieldRetryResponse(http.StatusBadRequest, `{"error":{"code":"unknown_parameter","message":"Unknown parameter: 'input[0].namespace'.","param":"input[0].namespace"}}`),
		newRejectedFieldRetryResponse(http.StatusOK, `{"id":"resp_ok","output":[],"usage":{"input_tokens":1,"output_tokens":1,"input_tokens_details":{"cached_tokens":0}}}`),
	}}
	service := &OpenAIGatewayService{
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
		httpUpstream: upstream,
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("User-Agent", "curl/8.0")
	account := &Account{
		ID:          5107,
		Name:        "responses-compatible",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://compat.example"},
		Status:      StatusActive,
		Schedulable: true,
	}

	result, err := service.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 2)
	require.Equal(t, "remove", gjson.GetBytes(upstream.bodies[0], "input.0.namespace").String())
	require.False(t, gjson.GetBytes(upstream.bodies[1], "input.0.namespace").Exists())
}

func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyRejectsAmbiguousErrors(t *testing.T) {
	retryBody, _, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(
		http.StatusBadRequest,
		[]byte(`{"input":[{"type":"message","namespace":"keep"}]}`),
		[]byte(`{"error":{"code":"unknown_parameter","message":"Unknown parameter: 'input[0].namespace'.","param":"input[0].namespace"}}`),
	)
	require.NoError(t, err)
	require.False(t, changed)
	require.Nil(t, retryBody)
}

func TestShouldFailoverOpenAIUpstreamResponseForAccountSpecificBodyLimit(t *testing.T) {
	service := &OpenAIGatewayService{}
	capacityMessage := "Selected model is at capacity. Please try a different model."
	require.True(t, service.shouldFailoverOpenAIUpstreamResponse(nil,
		http.StatusBadRequest,
		capacityMessage,
		[]byte(`{"error":{"message":"Selected model is at capacity. Please try a different model."}}`),
	))
	require.True(t, shouldFailoverOpenAIPassthroughResponse(nil, http.StatusBadRequest, capacityMessage, nil))
	require.True(t, service.shouldFailoverOpenAIUpstreamResponse(nil,
		http.StatusRequestEntityTooLarge,
		"request body is too large",
		[]byte(`{"error":{"message":"request body is too large"}}`),
	))
	require.False(t, service.shouldFailoverOpenAIUpstreamResponse(nil,
		http.StatusRequestEntityTooLarge,
		"maximum context length exceeded",
		[]byte(`{"error":{"code":"context_length_exceeded"}}`),
	))
}

func newRejectedFieldRetryResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
