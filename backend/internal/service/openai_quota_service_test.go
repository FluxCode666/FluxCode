package service

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/imroc/req/v3"
	"github.com/stretchr/testify/require"
)

func TestGenerateRedeemRequestIDUsesUUIDv4Shape(t *testing.T) {
	id, err := generateRedeemRequestID()
	require.NoError(t, err)
	require.Regexp(t, regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`), id)
}

func TestOpenAIQuotaMapUpstreamStatus(t *testing.T) {
	require.Equal(t, http.StatusUnauthorized, mapUpstreamStatus(http.StatusUnauthorized))
	require.Equal(t, http.StatusForbidden, mapUpstreamStatus(http.StatusForbidden))
	require.Equal(t, http.StatusTooManyRequests, mapUpstreamStatus(http.StatusTooManyRequests))
	require.Equal(t, http.StatusBadGateway, mapUpstreamStatus(http.StatusBadRequest))
	require.Equal(t, http.StatusBadGateway, mapUpstreamStatus(http.StatusInternalServerError))
}

type quotaAgentIdentityWSRecorder struct {
	accountIDs []int64
}

func (r *quotaAgentIdentityWSRecorder) InvalidateAgentIdentityWSConnections(accountID int64) {
	r.accountIDs = append(r.accountIDs, accountID)
}

func newQuotaTestRedirectingFactory(server *httptest.Server) PrivacyClientFactory {
	target, err := url.Parse(server.URL)
	if err != nil {
		panic(err)
	}
	return func(_ string) (*req.Client, error) {
		return req.C().WrapRoundTripFunc(func(next req.RoundTripper) req.RoundTripFunc {
			return func(request *req.Request) (*req.Response, error) {
				request.URL.Scheme = target.Scheme
				request.URL.Host = target.Host
				return next.RoundTrip(request)
			}
		}), nil
	}
}

func TestOpenAIQuotaQueryAgentIdentityUsesAssertionAndFedRAMP(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)
	account := &Account{ID: 94001, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{
		"auth_mode": OpenAIAuthModeAgentIdentity, "agent_runtime_id": "runtime-quota", "agent_private_key": base64.StdEncoding.EncodeToString(der),
		"task_id": "task-quota", "chatgpt_account_id": "team-quota", "chatgpt_account_is_fedramp": true,
	}}
	repo := &agentIdentityTestCredentialsRepo{account: account}
	var authorization, accountID, fedramp string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("authorization")
		accountID = r.Header.Get("chatgpt-account-id")
		fedramp = r.Header.Get("x-openai-fedramp")
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"plan_type":"pro","rate_limit":{"allowed":true}}`))
	}))
	defer server.Close()

	svc := NewOpenAIQuotaService(repo, nil, nil, newQuotaTestRedirectingFactory(server))
	usage, err := svc.QueryUsage(context.Background(), account.ID)
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.True(t, strings.HasPrefix(authorization, "AgentAssertion "))
	require.Equal(t, "team-quota", accountID)
	require.Equal(t, "true", fedramp)
}

func TestOpenAIQuotaResetAgentIdentityRecoversInvalidTaskOnce(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)
	account := &Account{ID: 94002, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{
		"auth_mode": OpenAIAuthModeAgentIdentity, "agent_runtime_id": "runtime-reset", "agent_private_key": base64.StdEncoding.EncodeToString(der),
		"task_id": "task-old", "chatgpt_account_id": "team-reset",
	}}
	repo := &agentIdentityTestCredentialsRepo{account: account}
	resetCalls := 0
	registerCalls := 0
	var assertions []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		if strings.Contains(r.URL.Path, "/task/register") {
			registerCalls++
			_, _ = w.Write([]byte(`{"task_id":"task-new"}`))
			return
		}
		resetCalls++
		assertions = append(assertions, r.Header.Get("authorization"))
		if resetCalls == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"code":"invalid_task_id"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":"ok","windows_reset":2}`))
	}))
	defer server.Close()
	oldBase := openAIAgentIdentityAuthAPIBaseURL
	openAIAgentIdentityAuthAPIBaseURL = server.URL
	t.Cleanup(func() { openAIAgentIdentityAuthAPIBaseURL = oldBase })

	invalidator := &quotaAgentIdentityWSRecorder{}
	svc := NewOpenAIQuotaService(repo, nil, nil, newQuotaTestRedirectingFactory(server))
	svc.agentIdentityWS = invalidator
	result, err := svc.ResetCredit(context.Background(), account.ID)
	require.NoError(t, err)
	require.Equal(t, "ok", result.Code)
	require.Equal(t, 2, resetCalls)
	require.Equal(t, 1, registerCalls)
	require.Len(t, assertions, 2)
	require.True(t, strings.HasPrefix(assertions[0], "AgentAssertion "))
	require.True(t, strings.HasPrefix(assertions[1], "AgentAssertion "))
	require.NotEqual(t, assertions[0], assertions[1])
	require.Equal(t, "task-new", account.GetCredential("task_id"))
	require.Equal(t, []int64{account.ID}, invalidator.accountIDs)
}
