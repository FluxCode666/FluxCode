package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type gatewayMessagesSchedulerCacheStub struct {
	accounts []*service.Account
}

func (s *gatewayMessagesSchedulerCacheStub) GetSnapshot(_ context.Context, bucket service.SchedulerBucket) ([]*service.Account, bool, error) {
	if bucket.GroupID == 0 {
		return s.accounts, true, nil
	}
	filtered := make([]*service.Account, 0, len(s.accounts))
	for _, account := range s.accounts {
		if account == nil {
			continue
		}
		for _, accountGroup := range account.AccountGroups {
			if accountGroup.GroupID == bucket.GroupID {
				filtered = append(filtered, account)
				break
			}
		}
	}
	return filtered, true, nil
}
func (s *gatewayMessagesSchedulerCacheStub) SetSnapshot(_ context.Context, _ service.SchedulerBucket, _ []service.Account) error {
	return nil
}
func (s *gatewayMessagesSchedulerCacheStub) SetSnapshotIndex(_ context.Context, _ service.SchedulerBucket, _ []service.Account) error {
	return nil
}
func (s *gatewayMessagesSchedulerCacheStub) WriteAccounts(_ context.Context, _ []service.Account) error {
	return nil
}
func (s *gatewayMessagesSchedulerCacheStub) GetAccount(_ context.Context, id int64) (*service.Account, error) {
	for _, account := range s.accounts {
		if account != nil && account.ID == id {
			return account, nil
		}
	}
	return nil, nil
}
func (s *gatewayMessagesSchedulerCacheStub) SetAccount(_ context.Context, _ *service.Account) error {
	return nil
}
func (s *gatewayMessagesSchedulerCacheStub) DeleteAccount(_ context.Context, _ int64) error {
	return nil
}
func (s *gatewayMessagesSchedulerCacheStub) UpdateLastUsed(_ context.Context, _ map[int64]time.Time) error {
	return nil
}
func (s *gatewayMessagesSchedulerCacheStub) TryLockBucket(_ context.Context, _ service.SchedulerBucket, _ time.Duration) (bool, error) {
	return true, nil
}
func (s *gatewayMessagesSchedulerCacheStub) ListBuckets(_ context.Context) ([]service.SchedulerBucket, error) {
	return nil, nil
}
func (s *gatewayMessagesSchedulerCacheStub) GetOutboxWatermark(_ context.Context) (int64, error) {
	return 0, nil
}
func (s *gatewayMessagesSchedulerCacheStub) SetOutboxWatermark(_ context.Context, _ int64) error {
	return nil
}

type gatewayMessagesGroupRepoStub struct {
	service.GroupRepository
	groups map[int64]*service.Group
}

func (s *gatewayMessagesGroupRepoStub) GetByID(_ context.Context, id int64) (*service.Group, error) {
	return s.groups[id], nil
}

func (s *gatewayMessagesGroupRepoStub) GetByIDLite(_ context.Context, id int64) (*service.Group, error) {
	return s.groups[id], nil
}

type gatewayMessagesAccountRepoStub struct {
	service.AccountRepository
	setErrorCalls int
}

func (s *gatewayMessagesAccountRepoStub) SetError(_ context.Context, _ int64, _ string) error {
	s.setErrorCalls++
	return nil
}

func (s *gatewayMessagesAccountRepoStub) SetBanned(_ context.Context, _ int64, _ string) error {
	return nil
}

func (s *gatewayMessagesAccountRepoStub) SetTempUnschedulable(_ context.Context, _ int64, _ time.Time, _ string) error {
	return nil
}

func (s *gatewayMessagesAccountRepoStub) IncrementQuotaUsed(_ context.Context, _ int64, _ float64) error {
	return nil
}

type gatewayMessagesConcurrencyCacheStub struct{}

func (s *gatewayMessagesConcurrencyCacheStub) AcquireAccountSlot(context.Context, int64, int, string) (bool, error) {
	return true, nil
}
func (s *gatewayMessagesConcurrencyCacheStub) ReleaseAccountSlot(context.Context, int64, string) error {
	return nil
}
func (s *gatewayMessagesConcurrencyCacheStub) GetAccountConcurrency(context.Context, int64) (int, error) {
	return 0, nil
}
func (s *gatewayMessagesConcurrencyCacheStub) IncrementAccountWaitCount(context.Context, int64, int) (bool, error) {
	return true, nil
}
func (s *gatewayMessagesConcurrencyCacheStub) DecrementAccountWaitCount(context.Context, int64) error {
	return nil
}
func (s *gatewayMessagesConcurrencyCacheStub) GetAccountWaitingCount(context.Context, int64) (int, error) {
	return 0, nil
}
func (s *gatewayMessagesConcurrencyCacheStub) AcquireUserSlot(context.Context, int64, int, string) (bool, error) {
	return true, nil
}
func (s *gatewayMessagesConcurrencyCacheStub) ReleaseUserSlot(context.Context, int64, string) error {
	return nil
}
func (s *gatewayMessagesConcurrencyCacheStub) GetUserConcurrency(context.Context, int64) (int, error) {
	return 0, nil
}
func (s *gatewayMessagesConcurrencyCacheStub) IncrementWaitCount(context.Context, int64, int) (bool, error) {
	return true, nil
}
func (s *gatewayMessagesConcurrencyCacheStub) DecrementWaitCount(context.Context, int64) error {
	return nil
}
func (s *gatewayMessagesConcurrencyCacheStub) GetAccountsLoadBatch(context.Context, []service.AccountWithConcurrency) (map[int64]*service.AccountLoadInfo, error) {
	return map[int64]*service.AccountLoadInfo{}, nil
}
func (s *gatewayMessagesConcurrencyCacheStub) GetUsersLoadBatch(context.Context, []service.UserWithConcurrency) (map[int64]*service.UserLoadInfo, error) {
	return map[int64]*service.UserLoadInfo{}, nil
}
func (s *gatewayMessagesConcurrencyCacheStub) GetAccountConcurrencyBatch(_ context.Context, accountIDs []int64) (map[int64]int, error) {
	result := make(map[int64]int, len(accountIDs))
	for _, id := range accountIDs {
		result[id] = 0
	}
	return result, nil
}
func (s *gatewayMessagesConcurrencyCacheStub) CleanupExpiredAccountSlots(context.Context, int64) error {
	return nil
}
func (s *gatewayMessagesConcurrencyCacheStub) CleanupExpiredSlotsByScan(context.Context) error {
	return nil
}
func (s *gatewayMessagesConcurrencyCacheStub) CleanupStaleProcessSlots(context.Context, string) error {
	return nil
}

type gatewayMessagesBillingCacheStub struct {
	service.BillingCache
	mu       sync.Mutex
	balances map[int64][]float64
}

func (s *gatewayMessagesBillingCacheStub) GetUserBalance(_ context.Context, userID int64) (float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	series := s.balances[userID]
	if len(series) == 0 {
		return 0, nil
	}
	balance := series[0]
	if len(series) > 1 {
		s.balances[userID] = series[1:]
	}
	return balance, nil
}

type gatewayMessagesHTTPUpstreamStub struct {
	mu          sync.Mutex
	calls       []int64
	requestBody map[int64][][]byte
	responses   map[int64][]*http.Response
}

func (s *gatewayMessagesHTTPUpstreamStub) Do(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return nil, nil
}

func (s *gatewayMessagesHTTPUpstreamStub) DoWithTLS(req *http.Request, _ string, accountID int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.calls = append(s.calls, accountID)
	if req != nil && req.Body != nil {
		body, _ := io.ReadAll(req.Body)
		if s.requestBody == nil {
			s.requestBody = make(map[int64][][]byte)
		}
		s.requestBody[accountID] = append(s.requestBody[accountID], body)
		req.Body = io.NopCloser(bytes.NewReader(body))
	}

	series := s.responses[accountID]
	if len(series) == 0 {
		return nil, nil
	}
	resp := series[0]
	if len(series) > 1 {
		s.responses[accountID] = series[1:]
	}
	return cloneGatewayMessagesResponse(resp), nil
}

type gatewayMessagesUsageLogRepoStub struct {
	service.UsageLogRepository
	lastLog *service.UsageLog
	calls   int
}

func (s *gatewayMessagesUsageLogRepoStub) Create(_ context.Context, log *service.UsageLog) (bool, error) {
	s.calls++
	s.lastLog = log
	return true, nil
}

type gatewayMessagesTestFixture struct {
	handler   *GatewayHandler
	usageRepo *gatewayMessagesUsageLogRepoStub
	upstream  *gatewayMessagesHTTPUpstreamStub
	cleanup   func()
}

func newGatewayMessagesTestFixture(t *testing.T, groups map[int64]*service.Group, accounts []*service.Account, balances map[int64][]float64, responses map[int64][]*http.Response, maxSwitches int) *gatewayMessagesTestFixture {
	t.Helper()

	cfg := &config.Config{}
	billingCacheSvc := service.NewBillingCacheService(&gatewayMessagesBillingCacheStub{balances: balances}, nil, nil, nil, nil, cfg)
	snapshotSvc := service.NewSchedulerSnapshotService(&gatewayMessagesSchedulerCacheStub{accounts: accounts}, nil, nil, nil, nil)
	upstream := &gatewayMessagesHTTPUpstreamStub{responses: responses}
	usageRepo := &gatewayMessagesUsageLogRepoStub{}
	accountRepo := &gatewayMessagesAccountRepoStub{}
	rateLimitSvc := service.NewRateLimitService(accountRepo, usageRepo, cfg, nil, nil)

	gatewaySvc := service.NewGatewayService(
		accountRepo,
		&gatewayMessagesGroupRepoStub{groups: groups},
		usageRepo,
		nil,
		nil,
		nil,
		nil,
		nil,
		cfg,
		snapshotSvc,
		service.NewConcurrencyService(&gatewayMessagesConcurrencyCacheStub{}),
		service.NewBillingService(cfg, nil),
		rateLimitSvc,
		billingCacheSvc,
		nil,
		upstream,
		&service.DeferredService{},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	handler := &GatewayHandler{
		gatewayService:      gatewaySvc,
		billingCacheService: billingCacheSvc,
		concurrencyHelper:   NewConcurrencyHelper(service.NewConcurrencyService(&gatewayMessagesConcurrencyCacheStub{}), SSEPingFormatClaude, 0),
		maxAccountSwitches:  maxSwitches,
	}

	return &gatewayMessagesTestFixture{
		handler:   handler,
		usageRepo: usageRepo,
		upstream:  upstream,
		cleanup: func() {
			billingCacheSvc.Stop()
		},
	}
}

func cloneGatewayMessagesResponse(resp *http.Response) *http.Response {
	if resp == nil {
		return nil
	}
	cloned := *resp
	if resp.Header != nil {
		cloned.Header = resp.Header.Clone()
	}
	if resp.Body == nil {
		return &cloned
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(body))
	cloned.Body = io.NopCloser(bytes.NewReader(body))
	return &cloned
}

func newGatewayMessagesJSONResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func newGatewayMessagesContext(t *testing.T, group *service.Group, apiKey *service.APIKey) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":32,"stream":false,"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), ctxkey.Group, group))
	c.Request = req
	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.UserID, Concurrency: 8})
	return c, rec
}

func TestGatewayEnsureForwardErrorResponse_WritesFallbackWhenNotWritten(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	h := &GatewayHandler{}
	wrote := h.ensureForwardErrorResponse(c, false)

	require.True(t, wrote)
	require.Equal(t, http.StatusBadGateway, w.Code)

	var parsed map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &parsed)
	require.NoError(t, err)
	assert.Equal(t, "error", parsed["type"])
	errorObj, ok := parsed["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "upstream_error", errorObj["type"])
	assert.Equal(t, "Upstream request failed", errorObj["message"])
}

func TestGatewayEnsureForwardErrorResponse_DoesNotOverrideWrittenResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.String(http.StatusTeapot, "already written")

	h := &GatewayHandler{}
	wrote := h.ensureForwardErrorResponse(c, false)

	require.False(t, wrote)
	require.Equal(t, http.StatusTeapot, w.Code)
	assert.Equal(t, "already written", w.Body.String())
}

func TestGatewayHandleStreamingAwareError_IncludesTraceIDInSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), ctxkey.TraceID, "trace-claude-stream-1")
	ctx = context.WithValue(ctx, ctxkey.RequestID, "request-claude-stream-1")
	c.Request = req.WithContext(ctx)

	h := &GatewayHandler{}
	h.handleStreamingAwareError(c, http.StatusBadGateway, "upstream_error", "stream failed", true)

	jsonStr := strings.TrimPrefix(strings.TrimSuffix(w.Body.String(), "\n\n"), "data: ")
	var parsed map[string]any
	err := json.Unmarshal([]byte(jsonStr), &parsed)
	require.NoError(t, err)
	assert.Equal(t, "trace-claude-stream-1", parsed["trace_id"])
	assert.Equal(t, "request-claude-stream-1", parsed["request_id"])
}

func TestGatewayTrySwitchToClaudeFallbackGroup_NoFallbackConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	h := &GatewayHandler{}
	apiKey := &service.APIKey{
		Group: &service.Group{ID: 1, Platform: service.PlatformAnthropic},
	}

	got, result := h.trySwitchToClaudeFallbackGroup(c, zap.NewNop(), apiKey, false)

	require.Equal(t, claudeFallbackUnavailable, result)
	require.Nil(t, got)
}

func TestGatewayHandleClaudeFallbackBillingFailure_ReturnsHandledTerminal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	h := &GatewayHandler{}

	result := h.handleClaudeFallbackBillingFailure(c, service.ErrInsufficientBalance, false)

	require.Equal(t, claudeFallbackHandled, result)
	require.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "insufficient balance")
}

func TestGatewayShouldAttemptClaudeRuntimeFallback_NonRetryableFailoverExhausted(t *testing.T) {
	shouldFallback := shouldAttemptClaudeRuntimeFallback(false, 0, 0, &service.UpstreamFailoverError{
		StatusCode:   http.StatusBadRequest,
		ResponseBody: []byte(`{"error":{"message":"bad request"}}`),
	})

	require.False(t, shouldFallback)
}

func TestGatewayShouldAttemptClaudeRuntimeFallback_StreamAlreadyWritten(t *testing.T) {
	shouldFallback := shouldAttemptClaudeRuntimeFallback(false, 0, 32, &service.UpstreamFailoverError{
		StatusCode:   http.StatusTooManyRequests,
		ResponseBody: []byte(`{"error":{"message":"rate limited"}}`),
	})

	require.False(t, shouldFallback)
}

func TestGatewayShouldRetryClaudeRuntimeFallback_AllowsTransportLikeErrors(t *testing.T) {
	require.True(t, shouldRetryClaudeRuntimeFallback(&service.UpstreamFailoverError{StatusCode: 0}))
	require.True(t, shouldRetryClaudeRuntimeFallback(&service.UpstreamFailoverError{StatusCode: http.StatusRequestTimeout}))
	require.True(t, shouldRetryClaudeRuntimeFallback(&service.UpstreamFailoverError{StatusCode: http.StatusTooManyRequests}))
	require.True(t, shouldRetryClaudeRuntimeFallback(&service.UpstreamFailoverError{StatusCode: http.StatusBadGateway}))
	require.False(t, shouldRetryClaudeRuntimeFallback(&service.UpstreamFailoverError{StatusCode: http.StatusUnauthorized}))
	require.False(t, shouldRetryClaudeRuntimeFallback(&service.UpstreamFailoverError{StatusCode: http.StatusBadRequest}))
	require.False(t, shouldRetryClaudeRuntimeFallback(&service.UpstreamFailoverError{StatusCode: http.StatusForbidden}))
	require.False(t, shouldRetryClaudeRuntimeFallback(nil))
}

func TestGatewayMessagesFallback_FailoverExhaustedRetryableSwitchesGroupAndAttributesUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	primaryGroupID := int64(6101)
	fallbackGroupID := int64(6102)
	primaryAccountID := int64(7101)
	fallbackAccountID := int64(7102)
	userID := int64(8101)
	apiKeyID := int64(9101)

	primaryGroup := &service.Group{
		ID:               primaryGroupID,
		Hydrated:         true,
		Platform:         service.PlatformAnthropic,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
		FallbackGroupID:  &fallbackGroupID,
	}
	fallbackGroup := &service.Group{
		ID:               fallbackGroupID,
		Hydrated:         true,
		Platform:         service.PlatformAnthropic,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
		IsFallbackGroup:  true,
	}
	apiKey := &service.APIKey{
		ID:      apiKeyID,
		UserID:  userID,
		GroupID: &primaryGroupID,
		Status:  service.StatusActive,
		User: &service.User{
			ID:          userID,
			Balance:     10,
			Concurrency: 8,
		},
		Group: primaryGroup,
	}

	fixture := newGatewayMessagesTestFixture(
		t,
		map[int64]*service.Group{
			primaryGroupID:  primaryGroup,
			fallbackGroupID: fallbackGroup,
		},
		[]*service.Account{
			{
				ID:          primaryAccountID,
				Name:        "primary-500",
				Platform:    service.PlatformAnthropic,
				Type:        service.AccountTypeAPIKey,
				Status:      service.StatusActive,
				Schedulable: true,
				Concurrency: 1,
				Priority:    1,
				Credentials: map[string]any{"api_key": "sk-primary"},
				AccountGroups: []service.AccountGroup{
					{AccountID: primaryAccountID, GroupID: primaryGroupID},
				},
			},
			{
				ID:          fallbackAccountID,
				Name:        "fallback-200",
				Platform:    service.PlatformAnthropic,
				Type:        service.AccountTypeAPIKey,
				Status:      service.StatusActive,
				Schedulable: true,
				Concurrency: 1,
				Priority:    1,
				Credentials: map[string]any{"api_key": "sk-fallback"},
				AccountGroups: []service.AccountGroup{
					{AccountID: fallbackAccountID, GroupID: fallbackGroupID},
				},
			},
		},
		map[int64][]float64{
			userID: {10, 10},
		},
		map[int64][]*http.Response{
			primaryAccountID: {
				newGatewayMessagesJSONResponse(http.StatusInternalServerError, `{"type":"error","error":{"type":"api_error","message":"primary exhausted"}}`),
			},
			fallbackAccountID: {
				newGatewayMessagesJSONResponse(http.StatusOK, `{"id":"msg_fallback","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"text","text":"from fallback"}],"usage":{"input_tokens":0,"output_tokens":0}}`),
			},
		},
		0,
	)
	defer fixture.cleanup()

	c, rec := newGatewayMessagesContext(t, primaryGroup, apiKey)

	fixture.handler.Messages(c)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"id":"msg_fallback","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"text","text":"from fallback"}],"usage":{"input_tokens":0,"output_tokens":0}}`, rec.Body.String())
	require.Equal(t, []int64{primaryAccountID, fallbackAccountID}, fixture.upstream.calls)
	selected, ok := c.Get(opsAccountIDKey)
	require.True(t, ok)
	require.Equal(t, fallbackAccountID, selected)
	require.NotNil(t, fixture.usageRepo.lastLog)
	require.NotNil(t, fixture.usageRepo.lastLog.GroupID)
	require.Equal(t, fallbackGroupID, *fixture.usageRepo.lastLog.GroupID)
	require.Equal(t, fallbackAccountID, fixture.usageRepo.lastLog.AccountID)
	require.Equal(t, apiKeyID, fixture.usageRepo.lastLog.APIKeyID)
}

func TestGatewayMessagesFallback_FailoverExhaustedNonRetryableKeepsOriginalTerminal(t *testing.T) {
	gin.SetMode(gin.TestMode)

	primaryGroupID := int64(6201)
	fallbackGroupID := int64(6202)
	primaryAccountID := int64(7201)
	fallbackAccountID := int64(7202)
	userID := int64(8201)

	primaryGroup := &service.Group{
		ID:               primaryGroupID,
		Hydrated:         true,
		Platform:         service.PlatformAnthropic,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
		FallbackGroupID:  &fallbackGroupID,
	}
	fallbackGroup := &service.Group{
		ID:               fallbackGroupID,
		Hydrated:         true,
		Platform:         service.PlatformAnthropic,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
		IsFallbackGroup:  true,
	}
	apiKey := &service.APIKey{
		ID:      9201,
		UserID:  userID,
		GroupID: &primaryGroupID,
		Status:  service.StatusActive,
		User: &service.User{
			ID:          userID,
			Balance:     10,
			Concurrency: 8,
		},
		Group: primaryGroup,
	}

	fixture := newGatewayMessagesTestFixture(
		t,
		map[int64]*service.Group{
			primaryGroupID:  primaryGroup,
			fallbackGroupID: fallbackGroup,
		},
		[]*service.Account{
			{
				ID:          primaryAccountID,
				Name:        "primary-403",
				Platform:    service.PlatformAnthropic,
				Type:        service.AccountTypeAPIKey,
				Status:      service.StatusActive,
				Schedulable: true,
				Concurrency: 1,
				Priority:    1,
				Credentials: map[string]any{"api_key": "sk-primary"},
				AccountGroups: []service.AccountGroup{
					{AccountID: primaryAccountID, GroupID: primaryGroupID},
				},
			},
			{
				ID:          fallbackAccountID,
				Name:        "fallback-unused",
				Platform:    service.PlatformAnthropic,
				Type:        service.AccountTypeAPIKey,
				Status:      service.StatusActive,
				Schedulable: true,
				Concurrency: 1,
				Priority:    1,
				Credentials: map[string]any{"api_key": "sk-fallback"},
				AccountGroups: []service.AccountGroup{
					{AccountID: fallbackAccountID, GroupID: fallbackGroupID},
				},
			},
		},
		map[int64][]float64{
			userID: {10},
		},
		map[int64][]*http.Response{
			primaryAccountID: {
				newGatewayMessagesJSONResponse(http.StatusForbidden, `{"type":"error","error":{"type":"permission_error","message":"forbidden upstream"}}`),
			},
			fallbackAccountID: {
				newGatewayMessagesJSONResponse(http.StatusOK, `{"id":"msg_should_not_be_used","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"text","text":"unexpected"}],"usage":{"input_tokens":0,"output_tokens":0}}`),
			},
		},
		0,
	)
	defer fixture.cleanup()

	c, rec := newGatewayMessagesContext(t, primaryGroup, apiKey)

	fixture.handler.Messages(c)

	require.Equal(t, http.StatusBadGateway, rec.Code)
	assert.Contains(t, rec.Body.String(), "Upstream access forbidden, please contact administrator")
	require.Equal(t, []int64{primaryAccountID}, fixture.upstream.calls)
	assert.Nil(t, fixture.usageRepo.lastLog)
}

func TestGatewayMessagesFallback_BillingFailureWritesOnlyOneTerminalResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	primaryGroupID := int64(6301)
	fallbackGroupID := int64(6302)
	userID := int64(8301)

	primaryGroup := &service.Group{
		ID:               primaryGroupID,
		Hydrated:         true,
		Platform:         service.PlatformAnthropic,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
		FallbackGroupID:  &fallbackGroupID,
	}
	fallbackGroup := &service.Group{
		ID:               fallbackGroupID,
		Hydrated:         true,
		Platform:         service.PlatformAnthropic,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
		IsFallbackGroup:  true,
	}
	apiKey := &service.APIKey{
		ID:      9301,
		UserID:  userID,
		GroupID: &primaryGroupID,
		Status:  service.StatusActive,
		User: &service.User{
			ID:          userID,
			Balance:     1,
			Concurrency: 8,
		},
		Group: primaryGroup,
	}

	fixture := newGatewayMessagesTestFixture(
		t,
		map[int64]*service.Group{
			primaryGroupID:  primaryGroup,
			fallbackGroupID: fallbackGroup,
		},
		nil,
		map[int64][]float64{
			userID: {1, 0},
		},
		map[int64][]*http.Response{},
		0,
	)
	defer fixture.cleanup()

	c, rec := newGatewayMessagesContext(t, primaryGroup, apiKey)

	fixture.handler.Messages(c)

	require.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "insufficient balance")
	assert.NotContains(t, rec.Body.String(), "No available accounts")
	assert.NotContains(t, rec.Body.String(), "Upstream rate limit exceeded")
	require.Empty(t, fixture.upstream.calls)
}
