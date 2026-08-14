package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"go.uber.org/zap"
)

func TestNormalizeOpenAIResponsesCompactRequestFromBodySignal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := &OpenAIGatewayHandler{}
	tests := []struct {
		name          string
		body          string
		betaFeature   string
		wantPath      string
		wantClientSSE bool
		wantBodyEqual bool
	}{
		{name: "stream rewrites to compact and records SSE bridge", body: `{"stream":true,"input":[{"type":"compaction_trigger"}]}`, wantPath: "/v1/responses/compact", wantClientSSE: true},
		{name: "non-stream rewrites to compact", body: `{"stream":false,"input":[{"type":"compaction_trigger"}]}`, wantPath: "/v1/responses/compact"},
		{name: "remote v2 keeps responses path and body", body: `{"stream":true,"input":[{"type":"compaction_trigger"}]}`, betaFeature: "remote_compaction_v2", wantPath: "/v1/responses", wantBodyEqual: true},
		{name: "ordinary responses unchanged", body: `{"stream":true,"input":[{"role":"user","content":"hi"}]}`, wantPath: "/v1/responses", wantBodyEqual: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(tt.body))
			if tt.betaFeature != "" {
				c.Request.Header.Set("x-codex-beta-features", tt.betaFeature)
			}

			got, ok := handler.normalizeOpenAIResponsesCompactRequest(c, zap.NewNop(), []byte(tt.body))
			require.True(t, ok)
			require.Equal(t, tt.wantPath, c.Request.URL.Path)
			if tt.wantBodyEqual {
				require.JSONEq(t, tt.body, string(got))
			}
			_, marked := c.Get("openai_compact_client_stream")
			require.Equal(t, tt.wantClientSSE, marked)
		})
	}
}

func TestOpenAIHandleStreamingAwareError_JSONEscaping(t *testing.T) {
	tests := []struct {
		name    string
		errType string
		message string
	}{
		{
			name:    "包含双引号的消息",
			errType: "server_error",
			message: `upstream returned "invalid" response`,
		},
		{
			name:    "包含反斜杠的消息",
			errType: "server_error",
			message: `path C:\Users\test\file.txt not found`,
		},
		{
			name:    "包含双引号和反斜杠的消息",
			errType: "upstream_error",
			message: `error parsing "key\value": unexpected token`,
		},
		{
			name:    "包含换行符的消息",
			errType: "server_error",
			message: "line1\nline2\ttab",
		},
		{
			name:    "普通消息",
			errType: "upstream_error",
			message: "Upstream service temporarily unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

			h := &OpenAIGatewayHandler{}
			h.handleStreamingAwareError(c, http.StatusBadGateway, tt.errType, tt.message, true)

			body := w.Body.String()

			// 验证 SSE 格式：event: error\ndata: {JSON}\n\n
			assert.True(t, strings.HasPrefix(body, "event: error\n"), "应以 'event: error\\n' 开头")
			assert.True(t, strings.HasSuffix(body, "\n\n"), "应以 '\\n\\n' 结尾")

			// 提取 data 部分
			lines := strings.Split(strings.TrimSuffix(body, "\n\n"), "\n")
			require.Len(t, lines, 2, "应有 event 行和 data 行")
			dataLine := lines[1]
			require.True(t, strings.HasPrefix(dataLine, "data: "), "第二行应以 'data: ' 开头")
			jsonStr := strings.TrimPrefix(dataLine, "data: ")

			// 验证 JSON 合法性
			var parsed map[string]any
			err := json.Unmarshal([]byte(jsonStr), &parsed)
			require.NoError(t, err, "JSON 应能被成功解析，原始 JSON: %s", jsonStr)

			// 验证结构
			errorObj, ok := parsed["error"].(map[string]any)
			require.True(t, ok, "应包含 error 对象")
			assert.Equal(t, tt.errType, errorObj["type"])
			assert.Equal(t, tt.message, errorObj["message"])
		})
	}
}

func TestResolveRuntimeFallbackAPIKey_Success(t *testing.T) {
	originalGroupID := int64(1)
	fallbackID := int64(2)
	apiKey := &service.APIKey{
		ID:      10,
		UserID:  42,
		GroupID: &originalGroupID,
		Group: &service.Group{
			ID:               originalGroupID,
			Platform:         service.PlatformOpenAI,
			SubscriptionType: service.SubscriptionTypeStandard,
			FallbackGroupID:  &fallbackID,
		},
		User: &service.User{ID: 42},
	}
	fallbackGroup := &service.Group{
		ID:               fallbackID,
		Platform:         service.PlatformOpenAI,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
		IsFallbackGroup:  true,
	}

	got, ok := resolveRuntimeFallbackAPIKeyForTest(context.Background(), apiKey, fallbackGroup)

	require.True(t, ok)
	require.NotNil(t, got)
	require.NotNil(t, got.GroupID)
	require.Equal(t, fallbackID, *got.GroupID)
	require.Same(t, fallbackGroup, got.Group)
	require.Same(t, apiKey.User, got.User)
}

func TestCloneAPIKeyWithGroup(t *testing.T) {
	originalGroupID := int64(9)
	apiKey := &service.APIKey{
		ID:      10,
		UserID:  42,
		GroupID: &originalGroupID,
		Group:   &service.Group{ID: originalGroupID, Platform: service.PlatformOpenAI},
		User:    &service.User{ID: 42},
	}
	group := &service.Group{ID: 0, Platform: service.PlatformOpenAI, Name: "runtime-fallback"}

	got := cloneAPIKeyWithGroup(apiKey, group)

	require.NotNil(t, got)
	require.NotSame(t, apiKey, got)
	require.NotNil(t, got.GroupID)
	require.Equal(t, int64(0), *got.GroupID)
	require.Same(t, group, got.Group)
	require.Same(t, apiKey.User, got.User)
	require.Equal(t, originalGroupID, *apiKey.GroupID)
	require.NotSame(t, apiKey.Group, got.Group)
}

func TestOpenAIRuntimeFallbackContext_SwitchesGroupAndPlatform(t *testing.T) {
	originalID := int64(100)
	fallbackID := int64(101)
	apiKey := &service.APIKey{
		ID:      1,
		UserID:  42,
		GroupID: &originalID,
		Group: &service.Group{
			ID:               originalID,
			Platform:         service.PlatformCodex2API,
			SubscriptionType: service.SubscriptionTypeStandard,
			FallbackGroupID:  &fallbackID,
		},
		User: &service.User{ID: 42},
	}
	fallbackGroup := &service.Group{
		ID:               fallbackID,
		Platform:         service.PlatformOpenAI,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
		IsFallbackGroup:  true,
	}

	got := cloneAPIKeyWithFallbackGroup(apiKey, fallbackGroup)

	require.NotNil(t, got)
	require.Equal(t, fallbackID, *got.GroupID)
	require.Equal(t, service.PlatformOpenAI, got.Group.Platform)
	require.Equal(t, service.PlatformOpenAI, resolveOpenAICompatibleGroupPlatform(got))
}

type openAIHandlerAccountRepoStub struct {
	service.AccountRepository
	accounts []service.Account
}

func (s *openAIHandlerAccountRepoStub) GetByID(_ context.Context, id int64) (*service.Account, error) {
	for i := range s.accounts {
		if s.accounts[i].ID == id {
			account := s.accounts[i]
			return &account, nil
		}
	}
	return nil, errors.New("account not found")
}

func (s *openAIHandlerAccountRepoStub) ListSchedulableByGroupIDAndPlatform(_ context.Context, groupID int64, platform string) ([]service.Account, error) {
	return s.listByGroupAndPlatforms(groupID, []string{platform}), nil
}

func (s *openAIHandlerAccountRepoStub) ListSchedulableByGroupIDAndPlatforms(_ context.Context, groupID int64, platforms []string) ([]service.Account, error) {
	return s.listByGroupAndPlatforms(groupID, platforms), nil
}

func (s *openAIHandlerAccountRepoStub) ListSchedulableByPlatform(_ context.Context, platform string) ([]service.Account, error) {
	return s.listByPlatforms([]string{platform}), nil
}

func (s *openAIHandlerAccountRepoStub) ListSchedulableByPlatforms(_ context.Context, platforms []string) ([]service.Account, error) {
	return s.listByPlatforms(platforms), nil
}

func (s *openAIHandlerAccountRepoStub) ListSchedulableUngroupedByPlatform(_ context.Context, platform string) ([]service.Account, error) {
	return s.listByPlatforms([]string{platform}), nil
}

func (s *openAIHandlerAccountRepoStub) ListSchedulableUngroupedByPlatforms(_ context.Context, platforms []string) ([]service.Account, error) {
	return s.listByPlatforms(platforms), nil
}

func (s *openAIHandlerAccountRepoStub) SetError(context.Context, int64, string) error {
	return nil
}

func (s *openAIHandlerAccountRepoStub) SetBanned(context.Context, int64, string) error {
	return nil
}

func (s *openAIHandlerAccountRepoStub) SetTempUnschedulable(context.Context, int64, time.Time, string) error {
	return nil
}

func (s *openAIHandlerAccountRepoStub) IncrementQuotaUsed(context.Context, int64, float64) error {
	return nil
}

func (s *openAIHandlerAccountRepoStub) listByGroupAndPlatforms(groupID int64, platforms []string) []service.Account {
	platformSet := make(map[string]struct{}, len(platforms))
	for _, platform := range platforms {
		platformSet[platform] = struct{}{}
	}
	result := make([]service.Account, 0, len(s.accounts))
	for _, account := range s.accounts {
		if _, ok := platformSet[account.Platform]; !ok {
			continue
		}
		if !account.IsSchedulable() {
			continue
		}
		for _, accountGroup := range account.AccountGroups {
			if accountGroup.GroupID == groupID {
				result = append(result, account)
				break
			}
		}
	}
	return result
}

func (s *openAIHandlerAccountRepoStub) listByPlatforms(platforms []string) []service.Account {
	platformSet := make(map[string]struct{}, len(platforms))
	for _, platform := range platforms {
		platformSet[platform] = struct{}{}
	}
	result := make([]service.Account, 0, len(s.accounts))
	for _, account := range s.accounts {
		if _, ok := platformSet[account.Platform]; ok && account.IsSchedulable() {
			result = append(result, account)
		}
	}
	return result
}

type openAIHandlerResponseFactory func() *http.Response

type openAIHandlerHTTPUpstreamStub struct {
	calls       []int64
	requestBody map[int64][][]byte
	responses   map[int64][]openAIHandlerResponseFactory
}

func (s *openAIHandlerHTTPUpstreamStub) Do(req *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	return s.dispatch(req, accountID)
}

func (s *openAIHandlerHTTPUpstreamStub) DoWithTLS(req *http.Request, _ string, accountID int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.dispatch(req, accountID)
}

func (s *openAIHandlerHTTPUpstreamStub) dispatch(req *http.Request, accountID int64) (*http.Response, error) {
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
		return nil, errors.New("unexpected upstream call")
	}
	factory := series[0]
	if len(series) > 1 {
		s.responses[accountID] = series[1:]
	} else {
		delete(s.responses, accountID)
	}
	return factory(), nil
}

type errorAfterEOFReadCloser struct {
	reader *bytes.Reader
	err    error
	fired  bool
}

func (r *errorAfterEOFReadCloser) Read(p []byte) (int, error) {
	if r.fired {
		return 0, io.EOF
	}
	n, err := r.reader.Read(p)
	if err == io.EOF {
		if n > 0 {
			return n, nil
		}
		r.fired = true
		return 0, r.err
	}
	return n, err
}

func (r *errorAfterEOFReadCloser) Close() error {
	return nil
}

type openAIHandlerRuntimeFallbackFixture struct {
	handler   *OpenAIGatewayHandler
	usageRepo *gatewayMessagesUsageLogRepoStub
	upstream  *openAIHandlerHTTPUpstreamStub
	cleanup   func()
}

func newOpenAIHandlerRuntimeFallbackFixture(
	t *testing.T,
	groups map[int64]*service.Group,
	accounts []*service.Account,
	balances map[int64][]float64,
	responses map[int64][]openAIHandlerResponseFactory,
	maxSwitches int,
) *openAIHandlerRuntimeFallbackFixture {
	t.Helper()

	cfg := &config.Config{}
	cfg.Default.RateMultiplier = 1
	billingCacheSvc := service.NewBillingCacheService(&gatewayMessagesBillingCacheStub{balances: balances}, nil, nil, nil, nil, cfg)
	snapshotSvc := service.NewSchedulerSnapshotService(&gatewayMessagesSchedulerCacheStub{accounts: accounts}, nil, nil, nil, nil)
	usageRepo := &gatewayMessagesUsageLogRepoStub{}
	upstream := &openAIHandlerHTTPUpstreamStub{responses: responses}

	accountCopies := make([]service.Account, 0, len(accounts))
	for _, account := range accounts {
		if account != nil {
			accountCopies = append(accountCopies, *account)
		}
	}
	accountRepo := &openAIHandlerAccountRepoStub{accounts: accountCopies}
	gatewaySvc := service.NewOpenAIGatewayService(
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
		nil,
		billingCacheSvc,
		upstream,
		&service.DeferredService{},
		nil,
		nil,
		nil,
		nil,
	)

	handler := &OpenAIGatewayHandler{
		gatewayService:      gatewaySvc,
		billingCacheService: billingCacheSvc,
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper:   NewConcurrencyHelper(service.NewConcurrencyService(&gatewayMessagesConcurrencyCacheStub{}), SSEPingFormatComment, 0),
		maxAccountSwitches:  maxSwitches,
	}

	return &openAIHandlerRuntimeFallbackFixture{
		handler:   handler,
		usageRepo: usageRepo,
		upstream:  upstream,
		cleanup: func() {
			billingCacheSvc.Stop()
		},
	}
}

func newOpenAIHandlerStaticResponse(statusCode int, contentType, body string) openAIHandlerResponseFactory {
	return func() *http.Response {
		return &http.Response{
			StatusCode: statusCode,
			Header:     http.Header{"Content-Type": []string{contentType}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}
	}
}

func newOpenAIHandlerStreamingErrorResponse(body string, err error) openAIHandlerResponseFactory {
	return func() *http.Response {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body: &errorAfterEOFReadCloser{
				reader: bytes.NewReader([]byte(body)),
				err:    err,
			},
		}
	}
}

func newOpenAIResponsesContext(t *testing.T, group *service.Group, apiKey *service.APIKey, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodPost, "/openai/v1/responses", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), ctxkey.Group, group))
	c.Request = req
	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.UserID, Concurrency: 8})
	return c, rec
}

func newOpenAIChatCompletionsContext(t *testing.T, group *service.Group, apiKey *service.APIKey, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), ctxkey.Group, group))
	c.Request = req
	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.UserID, Concurrency: 8})
	return c, rec
}

func newOpenAIImagesContext(t *testing.T, group *service.Group, apiKey *service.APIKey, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), ctxkey.Group, group))
	c.Request = req
	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.UserID, Concurrency: 8})
	return c, rec
}

func resolveRuntimeFallbackAPIKeyForTest(_ context.Context, apiKey *service.APIKey, fallbackGroup *service.Group) (*service.APIKey, bool) {
	cloned := cloneAPIKeyWithFallbackGroup(apiKey, fallbackGroup)
	return cloned, cloned != nil
}

type openAIFallbackSwitchTestHarness struct {
	handler *OpenAIGatewayHandler
	cleanup func()
}

func newOpenAIFallbackSwitchTestHarness(
	t *testing.T,
	groups map[int64]*service.Group,
	balances map[int64][]float64,
) *openAIFallbackSwitchTestHarness {
	t.Helper()

	cfg := &config.Config{}
	billingCacheSvc := service.NewBillingCacheService(&gatewayMessagesBillingCacheStub{balances: balances}, nil, nil, nil, nil, cfg)
	gatewaySvc := service.NewOpenAIGatewayService(
		nil,
		&gatewayMessagesGroupRepoStub{groups: groups},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		cfg,
		nil,
		nil,
		service.NewBillingService(cfg, nil),
		nil,
		billingCacheSvc,
		nil,
		&service.DeferredService{},
		nil,
		nil,
		nil,
		nil,
	)

	return &openAIFallbackSwitchTestHarness{
		handler: &OpenAIGatewayHandler{
			gatewayService:      gatewaySvc,
			billingCacheService: billingCacheSvc,
		},
		cleanup: func() {
			billingCacheSvc.Stop()
		},
	}
}

func newOpenAIFallbackContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", strings.NewReader(`{"model":"gpt-5.1","stream":false}`))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, rec
}

func newOpenAIRecordUsageServiceForFallbackTest(usageRepo service.UsageLogRepository) *service.OpenAIGatewayService {
	cfg := &config.Config{}
	cfg.Default.RateMultiplier = 1
	return service.NewOpenAIGatewayService(
		nil,
		nil,
		usageRepo,
		nil,
		nil,
		nil,
		nil,
		nil,
		cfg,
		nil,
		nil,
		service.NewBillingService(cfg, nil),
		nil,
		&service.BillingCacheService{},
		nil,
		&service.DeferredService{},
		nil,
		nil,
		nil,
		nil,
	)
}

func TestOpenAIRuntimeFallbackSelectionFailure_SwitchesGroupAndAttributesUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	primaryGroupID := int64(5101)
	fallbackGroupID := int64(5102)
	userID := int64(7101)

	primaryGroup := &service.Group{
		ID:               primaryGroupID,
		Platform:         service.PlatformOpenAI,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
		FallbackGroupID:  &fallbackGroupID,
	}
	fallbackGroup := &service.Group{
		ID:               fallbackGroupID,
		Platform:         service.PlatformOpenAI,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
		IsFallbackGroup:  true,
	}
	apiKey := &service.APIKey{
		ID:      8101,
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

	harness := newOpenAIFallbackSwitchTestHarness(
		t,
		map[int64]*service.Group{
			primaryGroupID:  primaryGroup,
			fallbackGroupID: fallbackGroup,
		},
		map[int64][]float64{
			userID: {10},
		},
	)
	defer harness.cleanup()

	c, _ := newOpenAIFallbackContext(t)
	fallbackAPIKey, result := harness.handler.trySwitchToOpenAIFallbackGroup(c, zap.NewNop(), apiKey, false)

	require.Equal(t, openAIFallbackSwitched, result)
	require.NotNil(t, fallbackAPIKey)
	require.NotNil(t, fallbackAPIKey.GroupID)
	require.Equal(t, fallbackGroupID, *fallbackAPIKey.GroupID)
	require.Same(t, fallbackGroup, fallbackAPIKey.Group)

	usageRepo := &gatewayMessagesUsageLogRepoStub{}
	usageSvc := newOpenAIRecordUsageServiceForFallbackTest(usageRepo)
	err := usageSvc.RecordUsage(context.Background(), &service.OpenAIRecordUsageInput{
		Result: &service.OpenAIForwardResult{
			RequestID: "resp_fallback_usage",
			Usage: service.OpenAIUsage{
				InputTokens:  3,
				OutputTokens: 4,
			},
			Model:    "gpt-5.1",
			Duration: time.Second,
		},
		APIKey:           fallbackAPIKey,
		User:             fallbackAPIKey.User,
		Account:          &service.Account{ID: 6102, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth},
		OriginalGroupID:  &primaryGroupID,
		InboundEndpoint:  "/openai/v1/responses",
		UpstreamEndpoint: "https://api.openai.com/v1/responses",
	})
	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.NotNil(t, usageRepo.lastLog.GroupID)
	require.Equal(t, fallbackGroupID, *usageRepo.lastLog.GroupID)
	require.NotNil(t, usageRepo.lastLog.OriginalGroupID)
	require.Equal(t, primaryGroupID, *usageRepo.lastLog.OriginalGroupID)
}

func TestOriginalGroupIDForRuntimeFallback_OnlyRecordsRealGroupSwitch(t *testing.T) {
	originalGroupID := int64(101)
	fallbackGroupID := int64(202)

	require.Nil(t, originalGroupIDForRuntimeFallback(false, &originalGroupID, &fallbackGroupID))
	require.Nil(t, originalGroupIDForRuntimeFallback(true, nil, &fallbackGroupID))
	require.Nil(t, originalGroupIDForRuntimeFallback(true, &originalGroupID, &originalGroupID))

	actual := originalGroupIDForRuntimeFallback(true, &originalGroupID, &fallbackGroupID)
	require.NotNil(t, actual)
	require.Equal(t, originalGroupID, *actual)
}

func TestOpenAIRuntimeFallbackFailoverExhaustedRetryable_AttemptsFallback(t *testing.T) {
	require.True(t, shouldRetryOpenAIRuntimeFallback(&service.UpstreamFailoverError{StatusCode: 0}))
	require.True(t, shouldRetryOpenAIRuntimeFallback(&service.UpstreamFailoverError{StatusCode: http.StatusRequestTimeout}))
	require.True(t, shouldRetryOpenAIRuntimeFallback(&service.UpstreamFailoverError{StatusCode: http.StatusForbidden}))
	require.True(t, shouldRetryOpenAIRuntimeFallback(&service.UpstreamFailoverError{StatusCode: http.StatusTooManyRequests}))
	require.True(t, shouldRetryOpenAIRuntimeFallback(&service.UpstreamFailoverError{StatusCode: http.StatusBadGateway}))
	require.True(t, shouldAttemptOpenAIRuntimeFallback(false, 0, 0, &service.UpstreamFailoverError{StatusCode: http.StatusForbidden}))
	require.True(t, shouldAttemptOpenAIRuntimeFallback(false, 0, 0, &service.UpstreamFailoverError{StatusCode: http.StatusInternalServerError}))
}

func TestOpenAIRuntimeFallbackFailoverExhaustedNonRetryable_DoesNotFallback(t *testing.T) {
	require.False(t, shouldRetryOpenAIRuntimeFallback(&service.UpstreamFailoverError{StatusCode: http.StatusUnauthorized}))
	require.False(t, shouldRetryOpenAIRuntimeFallback(&service.UpstreamFailoverError{StatusCode: http.StatusBadRequest}))
	require.False(t, shouldRetryOpenAIRuntimeFallback(nil))
	require.False(t, shouldAttemptOpenAIRuntimeFallback(false, 0, 16, &service.UpstreamFailoverError{StatusCode: http.StatusTooManyRequests}))
}

func TestOpenAIRuntimeFallbackBillingFailure_WritesOnlyOneTerminalResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	primaryGroupID := int64(5401)
	fallbackGroupID := int64(5402)
	userID := int64(7401)

	primaryGroup := &service.Group{
		ID:               primaryGroupID,
		Platform:         service.PlatformOpenAI,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
		FallbackGroupID:  &fallbackGroupID,
	}
	fallbackGroup := &service.Group{
		ID:               fallbackGroupID,
		Platform:         service.PlatformOpenAI,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
		IsFallbackGroup:  true,
	}
	apiKey := &service.APIKey{
		ID:      8401,
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

	harness := newOpenAIFallbackSwitchTestHarness(
		t,
		map[int64]*service.Group{
			primaryGroupID:  primaryGroup,
			fallbackGroupID: fallbackGroup,
		},
		map[int64][]float64{
			userID: {0},
		},
	)
	defer harness.cleanup()

	c, rec := newOpenAIFallbackContext(t)
	fallbackAPIKey, result := harness.handler.trySwitchToOpenAIFallbackGroup(c, zap.NewNop(), apiKey, false)

	require.Nil(t, fallbackAPIKey)
	require.Equal(t, openAIFallbackHandled, result)
	require.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "insufficient balance")
	assert.NotContains(t, rec.Body.String(), "Service temporarily unavailable")
	assert.NotContains(t, rec.Body.String(), "No available accounts")
}

func TestOpenAIRuntimeFallbackResponses_RetryableExhaustedSwitchesToFallbackGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)

	primaryGroupID := int64(8501)
	fallbackGroupID := int64(8502)
	primaryAccountID := int64(8601)
	fallbackAccountID := int64(8602)
	userID := int64(8701)
	apiKeyID := int64(8801)

	primaryGroup := &service.Group{
		ID:                    primaryGroupID,
		Hydrated:              true,
		Platform:              service.PlatformCodex2API,
		AllowMessagesDispatch: true,
		Status:                service.StatusActive,
		SubscriptionType:      service.SubscriptionTypeStandard,
		FallbackGroupID:       &fallbackGroupID,
	}
	fallbackGroup := &service.Group{
		ID:                    fallbackGroupID,
		Hydrated:              true,
		Platform:              service.PlatformOpenAI,
		AllowMessagesDispatch: true,
		Status:                service.StatusActive,
		SubscriptionType:      service.SubscriptionTypeStandard,
		IsFallbackGroup:       true,
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

	fixture := newOpenAIHandlerRuntimeFallbackFixture(
		t,
		map[int64]*service.Group{
			primaryGroupID:  primaryGroup,
			fallbackGroupID: fallbackGroup,
		},
		[]*service.Account{
			{
				ID:          primaryAccountID,
				Name:        "primary-500",
				Platform:    service.PlatformOpenAI,
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
				Platform:    service.PlatformOpenAI,
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
		map[int64][]openAIHandlerResponseFactory{
			primaryAccountID: {
				newOpenAIHandlerStaticResponse(http.StatusInternalServerError, "application/json", `{"error":{"message":"primary exhausted"}}`),
			},
			fallbackAccountID: {
				newOpenAIHandlerStaticResponse(http.StatusOK, "text/event-stream",
					"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_fallback\",\"object\":\"response\",\"model\":\"gpt-5.1\",\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"id\":\"msg_fallback\",\"role\":\"assistant\",\"status\":\"completed\",\"content\":[{\"type\":\"output_text\",\"text\":\"from fallback\"}]}],\"usage\":{\"input_tokens\":3,\"output_tokens\":2,\"total_tokens\":5}}}\n\n"),
			},
		},
		0,
	)
	defer fixture.cleanup()

	c, rec := newOpenAIResponsesContext(t, primaryGroup, apiKey, `{"model":"gpt-5.1","stream":false,"input":"hello"}`)

	fixture.handler.Responses(c)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"id":"resp_fallback"`)
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

func TestOpenAIRuntimeFallbackResponses_SelectionExhaustedRetryableSwitchesToFallbackGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)

	primaryGroupID := int64(85011)
	fallbackGroupID := int64(85012)
	primaryAccountID := int64(86011)
	fallbackAccountID := int64(86012)
	userID := int64(87011)

	primaryGroup := &service.Group{
		ID:                    primaryGroupID,
		Hydrated:              true,
		Platform:              service.PlatformCodex2API,
		AllowMessagesDispatch: true,
		Status:                service.StatusActive,
		SubscriptionType:      service.SubscriptionTypeStandard,
		FallbackGroupID:       &fallbackGroupID,
	}
	fallbackGroup := &service.Group{
		ID:                    fallbackGroupID,
		Hydrated:              true,
		Platform:              service.PlatformOpenAI,
		AllowMessagesDispatch: true,
		Status:                service.StatusActive,
		SubscriptionType:      service.SubscriptionTypeStandard,
		IsFallbackGroup:       true,
	}
	apiKey := &service.APIKey{
		ID:      88011,
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

	fixture := newOpenAIHandlerRuntimeFallbackFixture(
		t,
		map[int64]*service.Group{
			primaryGroupID:  primaryGroup,
			fallbackGroupID: fallbackGroup,
		},
		[]*service.Account{
			{
				ID:          primaryAccountID,
				Name:        "primary-500-then-exhausted",
				Platform:    service.PlatformOpenAI,
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
				Platform:    service.PlatformOpenAI,
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
		map[int64][]openAIHandlerResponseFactory{
			primaryAccountID: {
				newOpenAIHandlerStaticResponse(http.StatusInternalServerError, "application/json", `{"error":{"message":"primary exhausted"}}`),
			},
			fallbackAccountID: {
				newOpenAIHandlerStaticResponse(http.StatusOK, "text/event-stream",
					"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_fallback_after_selection_exhausted\",\"object\":\"response\",\"model\":\"gpt-5.1\",\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"id\":\"msg_fallback\",\"role\":\"assistant\",\"status\":\"completed\",\"content\":[{\"type\":\"output_text\",\"text\":\"from fallback\"}]}],\"usage\":{\"input_tokens\":3,\"output_tokens\":2,\"total_tokens\":5}}}\n\n"),
			},
		},
		1,
	)
	defer fixture.cleanup()

	c, rec := newOpenAIResponsesContext(t, primaryGroup, apiKey, `{"model":"gpt-5.1","stream":false,"input":"hello"}`)

	fixture.handler.Responses(c)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"id":"resp_fallback_after_selection_exhausted"`)
	require.Equal(t, []int64{primaryAccountID, fallbackAccountID}, fixture.upstream.calls)
	require.NotNil(t, fixture.usageRepo.lastLog)
	require.NotNil(t, fixture.usageRepo.lastLog.GroupID)
	require.Equal(t, fallbackGroupID, *fixture.usageRepo.lastLog.GroupID)
}

func TestOpenAIRuntimeFallbackResponses_StreamStartedDoesNotSwitchGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)

	primaryGroupID := int64(8901)
	fallbackGroupID := int64(8902)
	primaryAccountID := int64(9001)
	fallbackAccountID := int64(9002)
	userID := int64(9101)

	primaryGroup := &service.Group{
		ID:                    primaryGroupID,
		Hydrated:              true,
		Platform:              service.PlatformCodex2API,
		AllowMessagesDispatch: true,
		Status:                service.StatusActive,
		SubscriptionType:      service.SubscriptionTypeStandard,
		FallbackGroupID:       &fallbackGroupID,
	}
	fallbackGroup := &service.Group{
		ID:                    fallbackGroupID,
		Hydrated:              true,
		Platform:              service.PlatformOpenAI,
		AllowMessagesDispatch: true,
		Status:                service.StatusActive,
		SubscriptionType:      service.SubscriptionTypeStandard,
		IsFallbackGroup:       true,
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

	fixture := newOpenAIHandlerRuntimeFallbackFixture(
		t,
		map[int64]*service.Group{
			primaryGroupID:  primaryGroup,
			fallbackGroupID: fallbackGroup,
		},
		[]*service.Account{
			{
				ID:          primaryAccountID,
				Name:        "primary-stream-error",
				Platform:    service.PlatformOpenAI,
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
				Platform:    service.PlatformOpenAI,
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
		map[int64][]openAIHandlerResponseFactory{
			primaryAccountID: {
				newOpenAIHandlerStreamingErrorResponse(
					"data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n",
					errors.New("stream read failed"),
				),
			},
			fallbackAccountID: {
				newOpenAIHandlerStaticResponse(http.StatusOK, "text/event-stream",
					"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_should_not_be_used\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"),
			},
		},
		0,
	)
	defer fixture.cleanup()

	c, rec := newOpenAIResponsesContext(t, primaryGroup, apiKey, `{"model":"gpt-5.1","stream":true,"input":"hello"}`)

	fixture.handler.Responses(c)

	require.Equal(t, []int64{primaryAccountID}, fixture.upstream.calls)
	assert.Contains(t, rec.Body.String(), `"delta":"hello"`)
	assert.NotContains(t, rec.Body.String(), "resp_should_not_be_used")
	assert.Nil(t, fixture.usageRepo.lastLog)
}

func TestOpenAIRuntimeFallbackChatCompletions_NonRetryableDoesNotFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testCases := []struct {
		name   string
		status int
	}{
		{name: "bad_request", status: http.StatusBadRequest},
		{name: "unauthorized", status: http.StatusUnauthorized},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			primaryGroupID := int64(9301)
			fallbackGroupID := int64(9302)
			primaryAccountID := int64(9401)
			fallbackAccountID := int64(9402)
			userID := int64(9501)

			primaryGroup := &service.Group{
				ID:               primaryGroupID,
				Hydrated:         true,
				Platform:         service.PlatformCodex2API,
				Status:           service.StatusActive,
				SubscriptionType: service.SubscriptionTypeStandard,
				FallbackGroupID:  &fallbackGroupID,
			}
			fallbackGroup := &service.Group{
				ID:               fallbackGroupID,
				Hydrated:         true,
				Platform:         service.PlatformOpenAI,
				Status:           service.StatusActive,
				SubscriptionType: service.SubscriptionTypeStandard,
				IsFallbackGroup:  true,
			}
			apiKey := &service.APIKey{
				ID:      9601,
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

			fixture := newOpenAIHandlerRuntimeFallbackFixture(
				t,
				map[int64]*service.Group{
					primaryGroupID:  primaryGroup,
					fallbackGroupID: fallbackGroup,
				},
				[]*service.Account{
					{
						ID:          primaryAccountID,
						Name:        "primary-non-retryable",
						Platform:    service.PlatformOpenAI,
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
						Platform:    service.PlatformOpenAI,
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
				map[int64][]openAIHandlerResponseFactory{
					primaryAccountID: {
						newOpenAIHandlerStaticResponse(tt.status, "application/json", `{"error":{"message":"non retryable upstream"}}`),
					},
					fallbackAccountID: {
						newOpenAIHandlerStaticResponse(http.StatusOK, "text/event-stream",
							"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_should_not_be_used\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"),
					},
				},
				0,
			)
			defer fixture.cleanup()

			c, rec := newOpenAIChatCompletionsContext(t, primaryGroup, apiKey, `{"model":"gpt-5.1","stream":false,"messages":[{"role":"user","content":"hello"}]}`)

			fixture.handler.ChatCompletions(c)

			require.NotEqual(t, http.StatusOK, rec.Code)
			require.Equal(t, []int64{primaryAccountID}, fixture.upstream.calls)
			assert.NotContains(t, rec.Body.String(), "resp_should_not_be_used")
			assert.Nil(t, fixture.usageRepo.lastLog)
		})
	}
}

func TestOpenAIRuntimeFallbackChatCompletions_ForbiddenSwitchesToFallbackGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)

	primaryGroupID := int64(9311)
	fallbackGroupID := int64(9312)
	primaryAccountID := int64(9411)
	fallbackAccountID := int64(9412)
	userID := int64(9511)

	primaryGroup := &service.Group{
		ID:               primaryGroupID,
		Hydrated:         true,
		Platform:         service.PlatformOpenAI,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
		FallbackGroupID:  &fallbackGroupID,
	}
	fallbackGroup := &service.Group{
		ID:               fallbackGroupID,
		Hydrated:         true,
		Platform:         service.PlatformOpenAI,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
		IsFallbackGroup:  true,
	}
	apiKey := &service.APIKey{
		ID:      9611,
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

	fixture := newOpenAIHandlerRuntimeFallbackFixture(
		t,
		map[int64]*service.Group{
			primaryGroupID:  primaryGroup,
			fallbackGroupID: fallbackGroup,
		},
		[]*service.Account{
			{
				ID:          primaryAccountID,
				Name:        "primary-403",
				Platform:    service.PlatformOpenAI,
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
				Platform:    service.PlatformOpenAI,
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
		map[int64][]openAIHandlerResponseFactory{
			primaryAccountID: {
				newOpenAIHandlerStaticResponse(http.StatusForbidden, "application/json", `{"error":{"message":"insufficient balance"}}`),
			},
			fallbackAccountID: {
				newOpenAIHandlerStaticResponse(http.StatusOK, "text/event-stream",
					"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_forbidden_fallback\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"),
			},
		},
		0,
	)
	defer fixture.cleanup()

	c, rec := newOpenAIChatCompletionsContext(t, primaryGroup, apiKey, `{"model":"gpt-5.1","stream":false,"messages":[{"role":"user","content":"hello"}]}`)

	fixture.handler.ChatCompletions(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, []int64{primaryAccountID, fallbackAccountID}, fixture.upstream.calls)
	assert.Contains(t, rec.Body.String(), "resp_forbidden_fallback")
	require.NotNil(t, fixture.usageRepo.lastLog)
	require.NotNil(t, fixture.usageRepo.lastLog.GroupID)
	require.Equal(t, fallbackGroupID, *fixture.usageRepo.lastLog.GroupID)
}

func TestOpenAIRuntimeFallbackImages_FallbackBillingFailureWritesOnlyOneTerminalResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	primaryGroupID := int64(9701)
	fallbackGroupID := int64(9702)
	userID := int64(9801)

	primaryGroup := &service.Group{
		ID:                    primaryGroupID,
		Hydrated:              true,
		Platform:              service.PlatformCodex2API,
		AllowMessagesDispatch: true,
		Status:                service.StatusActive,
		SubscriptionType:      service.SubscriptionTypeStandard,
		FallbackGroupID:       &fallbackGroupID,
	}
	fallbackGroup := &service.Group{
		ID:                    fallbackGroupID,
		Hydrated:              true,
		Platform:              service.PlatformOpenAI,
		AllowMessagesDispatch: true,
		Status:                service.StatusActive,
		SubscriptionType:      service.SubscriptionTypeStandard,
		IsFallbackGroup:       true,
	}
	apiKey := &service.APIKey{
		ID:      9901,
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

	fixture := newOpenAIHandlerRuntimeFallbackFixture(
		t,
		map[int64]*service.Group{
			primaryGroupID:  primaryGroup,
			fallbackGroupID: fallbackGroup,
		},
		nil,
		map[int64][]float64{
			userID: {1, 0},
		},
		map[int64][]openAIHandlerResponseFactory{},
		0,
	)
	defer fixture.cleanup()

	c, rec := newOpenAIImagesContext(t, primaryGroup, apiKey, `{"model":"gpt-image-1","prompt":"hello","response_format":"b64_json"}`)

	fixture.handler.Images(c)

	require.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "insufficient balance")
	assert.NotContains(t, rec.Body.String(), "No available compatible accounts")
	assert.Empty(t, fixture.upstream.calls)
	assert.Nil(t, fixture.usageRepo.lastLog)
}

func TestOpenAIRuntimeFallbackMessages_FallbackBillingFailureUsesAnthropicErrorWriter(t *testing.T) {
	gin.SetMode(gin.TestMode)

	primaryGroupID := int64(10001)
	fallbackGroupID := int64(10002)
	userID := int64(10101)

	primaryGroup := &service.Group{
		ID:                    primaryGroupID,
		Hydrated:              true,
		Platform:              service.PlatformCodex2API,
		AllowMessagesDispatch: true,
		Status:                service.StatusActive,
		SubscriptionType:      service.SubscriptionTypeStandard,
		FallbackGroupID:       &fallbackGroupID,
	}
	fallbackGroup := &service.Group{
		ID:                    fallbackGroupID,
		Hydrated:              true,
		Platform:              service.PlatformOpenAI,
		AllowMessagesDispatch: true,
		Status:                service.StatusActive,
		SubscriptionType:      service.SubscriptionTypeStandard,
		IsFallbackGroup:       true,
	}
	apiKey := &service.APIKey{
		ID:      10201,
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

	fixture := newOpenAIHandlerRuntimeFallbackFixture(
		t,
		map[int64]*service.Group{
			primaryGroupID:  primaryGroup,
			fallbackGroupID: fallbackGroup,
		},
		nil,
		map[int64][]float64{
			userID: {1, 0},
		},
		map[int64][]openAIHandlerResponseFactory{},
		0,
	)
	defer fixture.cleanup()

	c, rec := newGatewayMessagesContext(t, primaryGroup, apiKey)

	fixture.handler.Messages(c)

	require.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), `"type":"error"`)
	assert.Contains(t, rec.Body.String(), "insufficient balance")
	assert.NotContains(t, rec.Body.String(), "Service temporarily unavailable")
	assert.Empty(t, fixture.upstream.calls)
	assert.Nil(t, fixture.usageRepo.lastLog)
}

func TestOpenAIHandleStreamingAwareError_IncludesTraceIDInSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), ctxkey.TraceID, "trace-stream-1")
	ctx = context.WithValue(ctx, ctxkey.RequestID, "request-stream-1")
	c.Request = req.WithContext(ctx)

	h := &OpenAIGatewayHandler{}
	h.handleStreamingAwareError(c, http.StatusBadGateway, "upstream_error", "stream failed", true)

	lines := strings.Split(strings.TrimSuffix(w.Body.String(), "\n\n"), "\n")
	require.Len(t, lines, 2)
	jsonStr := strings.TrimPrefix(lines[1], "data: ")

	var parsed map[string]any
	err := json.Unmarshal([]byte(jsonStr), &parsed)
	require.NoError(t, err)
	assert.Equal(t, "trace-stream-1", parsed["trace_id"])
	assert.Equal(t, "request-stream-1", parsed["request_id"])
}

func TestHasOpenAIResponseStarted_TreatsUnwrittenRecorderAsNotStarted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	require.Equal(t, -1, c.Writer.Size())
	require.False(t, c.Writer.Written())
	require.False(t, hasOpenAIResponseStarted(c, false))

	c.String(http.StatusOK, "written")
	require.True(t, hasOpenAIResponseStarted(c, false))
}

func TestOpenAIHandleStreamingAwareError_NonStreaming(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), ctxkey.TraceID, "trace-response-1")
	ctx = context.WithValue(ctx, ctxkey.RequestID, "request-response-1")
	c.Request = req.WithContext(ctx)

	h := &OpenAIGatewayHandler{}
	h.handleStreamingAwareError(c, http.StatusBadGateway, "upstream_error", "test error", false)

	// 非流式应返回 JSON 响应
	assert.Equal(t, http.StatusBadGateway, w.Code)

	var parsed map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &parsed)
	require.NoError(t, err)
	errorObj, ok := parsed["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "upstream_error", errorObj["type"])
	assert.Equal(t, "test error", errorObj["message"])
	assert.Equal(t, "trace-response-1", parsed["trace_id"])
	assert.Equal(t, "request-response-1", parsed["request_id"])
}

func TestGatewayErrorResponseIncludesTraceID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), ctxkey.TraceID, "trace-claude-1")
	ctx = context.WithValue(ctx, ctxkey.RequestID, "request-claude-1")
	c.Request = req.WithContext(ctx)

	h := &GatewayHandler{}
	h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Service temporarily unavailable")

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var parsed map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &parsed)
	require.NoError(t, err)
	errorObj, ok := parsed["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "api_error", errorObj["type"])
	assert.Equal(t, "Service temporarily unavailable", errorObj["message"])
	assert.Equal(t, "trace-claude-1", parsed["trace_id"])
	assert.Equal(t, "request-claude-1", parsed["request_id"])
}

func TestReadRequestBodyWithPrealloc(t *testing.T) {
	payload := `{"model":"gpt-5","input":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(payload))
	req.ContentLength = int64(len(payload))

	body, err := pkghttputil.ReadRequestBodyWithPrealloc(req)
	require.NoError(t, err)
	require.Equal(t, payload, string(body))
}

func TestReadRequestBodyWithPrealloc_MaxBytesError(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(strings.Repeat("x", 8)))
	req.Body = http.MaxBytesReader(rec, req.Body, 4)

	_, err := pkghttputil.ReadRequestBodyWithPrealloc(req)
	require.Error(t, err)
	var maxErr *http.MaxBytesError
	require.ErrorAs(t, err, &maxErr)
}

func TestOpenAIEnsureForwardErrorResponse_WritesFallbackWhenNotWritten(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	h := &OpenAIGatewayHandler{}
	wrote := h.ensureForwardErrorResponse(c, false)

	require.True(t, wrote)
	require.Equal(t, http.StatusBadGateway, w.Code)

	var parsed map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &parsed)
	require.NoError(t, err)
	errorObj, ok := parsed["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "upstream_error", errorObj["type"])
	assert.Equal(t, "Upstream request failed", errorObj["message"])
}

func TestOpenAIEnsureForwardErrorResponse_DoesNotOverrideWrittenResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.String(http.StatusTeapot, "already written")

	h := &OpenAIGatewayHandler{}
	wrote := h.ensureForwardErrorResponse(c, false)

	require.False(t, wrote)
	require.Equal(t, http.StatusTeapot, w.Code)
	assert.Equal(t, "already written", w.Body.String())
}

func TestOpenAIEnsureForwardErrorResponse_CompactKeepaliveOnlyWritesResponseFailed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, EndpointResponses, nil)
	service.MarkOpenAICompactClientStream(c)

	stop := service.StartOpenAICompactSSEKeepalive(c, 5*time.Millisecond)
	defer stop()
	before := service.OpenAICompactKeepaliveAdjustedWrittenSize(c)
	require.Eventually(t, c.Writer.Written, time.Second, time.Millisecond)
	require.Equal(t, before, service.OpenAICompactKeepaliveAdjustedWrittenSize(c))

	h := &OpenAIGatewayHandler{}
	require.True(t, h.ensureForwardErrorResponse(c, false))
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "event: response.failed\n")
	require.NotContains(t, w.Body.String(), "event: error\n")
}

func TestOpenAIEnsureForwardErrorResponse_CompactMeaningfulOutputIsPreserved(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, EndpointResponses, nil)
	service.MarkOpenAICompactClientStream(c)

	stop := service.StartOpenAICompactSSEKeepalive(c, time.Hour)
	defer stop()
	_, err := c.Writer.WriteString("event: response.completed\ndata: {\"type\":\"response.completed\"}\n\n")
	require.NoError(t, err)

	h := &OpenAIGatewayHandler{}
	require.False(t, h.ensureForwardErrorResponse(c, false))
	require.Equal(t, 1, strings.Count(w.Body.String(), "event: response.completed\n"))
	require.NotContains(t, w.Body.String(), "event: response.failed\n")
}

func TestShouldLogOpenAIForwardFailureAsWarn(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("fallback_written_should_not_downgrade", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		require.False(t, shouldLogOpenAIForwardFailureAsWarn(c, true))
	})

	t.Run("context_nil_should_not_downgrade", func(t *testing.T) {
		require.False(t, shouldLogOpenAIForwardFailureAsWarn(nil, false))
	})

	t.Run("response_not_written_should_not_downgrade", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		require.False(t, shouldLogOpenAIForwardFailureAsWarn(c, false))
	})

	t.Run("response_already_written_should_downgrade", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		c.String(http.StatusForbidden, "already written")
		require.True(t, shouldLogOpenAIForwardFailureAsWarn(c, false))
	})
}

func TestOpenAIRecoverResponsesPanic_WritesFallbackResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	h := &OpenAIGatewayHandler{}
	streamStarted := false
	require.NotPanics(t, func() {
		func() {
			defer h.recoverResponsesPanic(c, &streamStarted)
			panic("test panic")
		}()
	})

	require.Equal(t, http.StatusBadGateway, w.Code)

	var parsed map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &parsed)
	require.NoError(t, err)

	errorObj, ok := parsed["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "upstream_error", errorObj["type"])
	assert.Equal(t, "Upstream request failed", errorObj["message"])
}

func TestOpenAIRecoverResponsesPanic_NoPanicNoWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	h := &OpenAIGatewayHandler{}
	streamStarted := false
	require.NotPanics(t, func() {
		func() {
			defer h.recoverResponsesPanic(c, &streamStarted)
		}()
	})

	require.False(t, c.Writer.Written())
	assert.Equal(t, "", w.Body.String())
}

func TestOpenAIRecoverResponsesPanic_DoesNotOverrideWrittenResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.String(http.StatusTeapot, "already written")

	h := &OpenAIGatewayHandler{}
	streamStarted := false
	require.NotPanics(t, func() {
		func() {
			defer h.recoverResponsesPanic(c, &streamStarted)
			panic("test panic")
		}()
	})

	require.Equal(t, http.StatusTeapot, w.Code)
	assert.Equal(t, "already written", w.Body.String())
}

func TestOpenAIMissingResponsesDependencies(t *testing.T) {
	t.Run("nil_handler", func(t *testing.T) {
		var h *OpenAIGatewayHandler
		require.Equal(t, []string{"handler"}, h.missingResponsesDependencies())
	})

	t.Run("all_dependencies_missing", func(t *testing.T) {
		h := &OpenAIGatewayHandler{}
		require.Equal(t,
			[]string{"gatewayService", "billingCacheService", "apiKeyService", "concurrencyHelper"},
			h.missingResponsesDependencies(),
		)
	})

	t.Run("all_dependencies_present", func(t *testing.T) {
		h := &OpenAIGatewayHandler{
			gatewayService:      &service.OpenAIGatewayService{},
			billingCacheService: &service.BillingCacheService{},
			apiKeyService:       &service.APIKeyService{},
			concurrencyHelper: &ConcurrencyHelper{
				concurrencyService: &service.ConcurrencyService{},
			},
		}
		require.Empty(t, h.missingResponsesDependencies())
	})
}

func TestOpenAIEnsureResponsesDependencies(t *testing.T) {
	t.Run("missing_dependencies_returns_503", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

		h := &OpenAIGatewayHandler{}
		ok := h.ensureResponsesDependencies(c, nil)

		require.False(t, ok)
		require.Equal(t, http.StatusServiceUnavailable, w.Code)
		var parsed map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &parsed)
		require.NoError(t, err)
		errorObj, exists := parsed["error"].(map[string]any)
		require.True(t, exists)
		assert.Equal(t, "api_error", errorObj["type"])
		assert.Equal(t, "Service temporarily unavailable", errorObj["message"])
	})

	t.Run("already_written_response_not_overridden", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		c.String(http.StatusTeapot, "already written")

		h := &OpenAIGatewayHandler{}
		ok := h.ensureResponsesDependencies(c, nil)

		require.False(t, ok)
		require.Equal(t, http.StatusTeapot, w.Code)
		assert.Equal(t, "already written", w.Body.String())
	})

	t.Run("dependencies_ready_returns_true_and_no_write", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

		h := &OpenAIGatewayHandler{
			gatewayService:      &service.OpenAIGatewayService{},
			billingCacheService: &service.BillingCacheService{},
			apiKeyService:       &service.APIKeyService{},
			concurrencyHelper: &ConcurrencyHelper{
				concurrencyService: &service.ConcurrencyService{},
			},
		}
		ok := h.ensureResponsesDependencies(c, nil)

		require.True(t, ok)
		require.False(t, c.Writer.Written())
		assert.Equal(t, "", w.Body.String())
	})
}

func TestResolveOpenAIForwardDefaultMappedModel(t *testing.T) {
	t.Run("prefers_explicit_fallback_model", func(t *testing.T) {
		apiKey := &service.APIKey{
			Group: &service.Group{DefaultMappedModel: "gpt-5.4"},
		}
		require.Equal(t, "gpt-5.2", resolveOpenAIForwardDefaultMappedModel(apiKey, " gpt-5.2 "))
	})

	t.Run("uses_group_default_when_explicit_fallback_absent", func(t *testing.T) {
		apiKey := &service.APIKey{
			Group: &service.Group{DefaultMappedModel: "gpt-5.4"},
		}
		require.Equal(t, "gpt-5.4", resolveOpenAIForwardDefaultMappedModel(apiKey, ""))
	})

	t.Run("returns_empty_without_group_default", func(t *testing.T) {
		require.Empty(t, resolveOpenAIForwardDefaultMappedModel(nil, ""))
		require.Empty(t, resolveOpenAIForwardDefaultMappedModel(&service.APIKey{}, ""))
		require.Empty(t, resolveOpenAIForwardDefaultMappedModel(&service.APIKey{
			Group: &service.Group{},
		}, ""))
	})
}

func TestResolveOpenAIMessagesDispatchMappedModel(t *testing.T) {
	t.Run("exact_claude_model_override_wins", func(t *testing.T) {
		apiKey := &service.APIKey{
			Group: &service.Group{
				MessagesDispatchModelConfig: service.OpenAIMessagesDispatchModelConfig{
					SonnetMappedModel: "gpt-5.2",
					ExactModelMappings: map[string]string{
						"claude-sonnet-4-5-20250929": "gpt-5.4-mini-high",
					},
				},
			},
		}
		require.Equal(t, "gpt-5.4-mini", resolveOpenAIMessagesDispatchMappedModel(apiKey, "claude-sonnet-4-5-20250929"))
	})

	t.Run("uses_family_default_when_no_override", func(t *testing.T) {
		apiKey := &service.APIKey{Group: &service.Group{}}
		require.Equal(t, "gpt-5.4", resolveOpenAIMessagesDispatchMappedModel(apiKey, "claude-opus-4-6"))
		require.Equal(t, "gpt-5.3-codex", resolveOpenAIMessagesDispatchMappedModel(apiKey, "claude-sonnet-4-5-20250929"))
		require.Equal(t, "gpt-5.4-mini", resolveOpenAIMessagesDispatchMappedModel(apiKey, "claude-haiku-4-5-20251001"))
	})

	t.Run("returns_empty_for_non_claude_or_missing_group", func(t *testing.T) {
		require.Empty(t, resolveOpenAIMessagesDispatchMappedModel(nil, "claude-sonnet-4-5-20250929"))
		require.Empty(t, resolveOpenAIMessagesDispatchMappedModel(&service.APIKey{}, "claude-sonnet-4-5-20250929"))
		require.Empty(t, resolveOpenAIMessagesDispatchMappedModel(&service.APIKey{Group: &service.Group{}}, "gpt-5.4"))
	})

	t.Run("does_not_fall_back_to_group_default_mapped_model", func(t *testing.T) {
		apiKey := &service.APIKey{
			Group: &service.Group{
				DefaultMappedModel: "gpt-5.4",
			},
		}
		require.Empty(t, resolveOpenAIMessagesDispatchMappedModel(apiKey, "gpt-5.4"))
		require.Equal(t, "gpt-5.3-codex", resolveOpenAIMessagesDispatchMappedModel(apiKey, "claude-sonnet-4-5-20250929"))
	})
}

func TestOpenAIResponses_MissingDependencies_ReturnsServiceUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5","stream":false}`))
	c.Request.Header.Set("Content-Type", "application/json")

	groupID := int64(2)
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
		ID:      10,
		GroupID: &groupID,
	})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
		UserID:      1,
		Concurrency: 1,
	})

	// 故意使用未初始化依赖，验证快速失败而不是崩溃。
	h := &OpenAIGatewayHandler{}
	require.NotPanics(t, func() {
		h.Responses(c)
	})

	require.Equal(t, http.StatusServiceUnavailable, w.Code)

	var parsed map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &parsed)
	require.NoError(t, err)

	errorObj, ok := parsed["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "api_error", errorObj["type"])
	assert.Equal(t, "Service temporarily unavailable", errorObj["message"])
}

func TestOpenAIResponses_SetsClientTransportHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", strings.NewReader(`{"model":"gpt-5"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h := &OpenAIGatewayHandler{}
	h.Responses(c)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.Equal(t, service.OpenAIClientTransportHTTP, service.GetOpenAIClientTransport(c))
}

func TestOpenAIResponses_RejectsMessageIDAsPreviousResponseID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", strings.NewReader(
		`{"model":"gpt-5.1","stream":false,"previous_response_id":"msg_123456","input":[{"type":"input_text","text":"hello"}]}`,
	))
	c.Request.Header.Set("Content-Type", "application/json")

	groupID := int64(2)
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
		ID:      101,
		GroupID: &groupID,
		User:    &service.User{ID: 1},
	})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
		UserID:      1,
		Concurrency: 1,
	})

	h := newOpenAIHandlerForPreviousResponseIDValidation(t, nil)
	h.Responses(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "previous_response_id must be a response.id")
}

func TestOpenAIResponsesWebSocket_SetsClientTransportWSWhenUpgradeValid(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/openai/v1/responses", nil)
	c.Request.Header.Set("Upgrade", "websocket")
	c.Request.Header.Set("Connection", "Upgrade")

	h := &OpenAIGatewayHandler{}
	h.ResponsesWebSocket(c)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.Equal(t, service.OpenAIClientTransportWS, service.GetOpenAIClientTransport(c))
}

func TestOpenAIResponsesWebSocket_InvalidUpgradeDoesNotSetTransport(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/openai/v1/responses", nil)

	h := &OpenAIGatewayHandler{}
	h.ResponsesWebSocket(c)

	require.Equal(t, http.StatusUpgradeRequired, w.Code)
	require.Equal(t, service.OpenAIClientTransportUnknown, service.GetOpenAIClientTransport(c))
}

func TestOpenAIResponsesWebSocket_RejectsMessageIDAsPreviousResponseID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := newOpenAIHandlerForPreviousResponseIDValidation(t, nil)
	wsServer := newOpenAIWSHandlerTestServer(t, h, middleware.AuthSubject{UserID: 1, Concurrency: 1})
	defer wsServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http")+"/openai/v1/responses", nil)
	cancelDial()
	require.NoError(t, err)
	defer func() {
		_ = clientConn.CloseNow()
	}()

	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(
		`{"type":"response.create","model":"gpt-5.1","stream":false,"previous_response_id":"msg_abc123"}`,
	))
	cancelWrite()
	require.NoError(t, err)

	readCtx, cancelRead := context.WithTimeout(context.Background(), 3*time.Second)
	_, _, err = clientConn.Read(readCtx)
	cancelRead()
	require.Error(t, err)
	var closeErr coderws.CloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, coderws.StatusPolicyViolation, closeErr.Code)
	require.Contains(t, strings.ToLower(closeErr.Reason), "previous_response_id")
}

func TestOpenAIResponsesWebSocket_PreviousResponseIDKindLoggedBeforeAcquireFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cache := &concurrencyCacheMock{
		acquireUserSlotFn: func(ctx context.Context, userID int64, maxConcurrency int, requestID string) (bool, error) {
			return false, errors.New("user slot unavailable")
		},
	}
	h := newOpenAIHandlerForPreviousResponseIDValidation(t, cache)
	wsServer := newOpenAIWSHandlerTestServer(t, h, middleware.AuthSubject{UserID: 1, Concurrency: 1})
	defer wsServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http")+"/openai/v1/responses", nil)
	cancelDial()
	require.NoError(t, err)
	defer func() {
		_ = clientConn.CloseNow()
	}()

	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(
		`{"type":"response.create","model":"gpt-5.1","stream":false,"previous_response_id":"resp_prev_123"}`,
	))
	cancelWrite()
	require.NoError(t, err)

	readCtx, cancelRead := context.WithTimeout(context.Background(), 3*time.Second)
	_, _, err = clientConn.Read(readCtx)
	cancelRead()
	require.Error(t, err)
	var closeErr coderws.CloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, coderws.StatusInternalError, closeErr.Code)
	require.Contains(t, strings.ToLower(closeErr.Reason), "failed to acquire user concurrency slot")
}

func TestSetOpenAIClientTransportHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	setOpenAIClientTransportHTTP(c)
	require.Equal(t, service.OpenAIClientTransportHTTP, service.GetOpenAIClientTransport(c))
}

func TestSetOpenAIClientTransportWS(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	setOpenAIClientTransportWS(c)
	require.Equal(t, service.OpenAIClientTransportWS, service.GetOpenAIClientTransport(c))
}

// TestOpenAIHandler_GjsonExtraction 验证 gjson 从请求体中提取 model/stream 的正确性
func TestOpenAIHandler_GjsonExtraction(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantModel  string
		wantStream bool
	}{
		{"正常提取", `{"model":"gpt-4","stream":true,"input":"hello"}`, "gpt-4", true},
		{"stream false", `{"model":"gpt-4","stream":false}`, "gpt-4", false},
		{"无 stream 字段", `{"model":"gpt-4"}`, "gpt-4", false},
		{"model 缺失", `{"stream":true}`, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(tt.body)
			modelResult := gjson.GetBytes(body, "model")
			model := ""
			if modelResult.Type == gjson.String {
				model = modelResult.String()
			}
			stream := gjson.GetBytes(body, "stream").Bool()
			require.Equal(t, tt.wantModel, model)
			require.Equal(t, tt.wantStream, stream)
		})
	}
}

// TestOpenAIHandler_GjsonValidation 验证修复后的 JSON 合法性和类型校验
func TestOpenAIHandler_GjsonValidation(t *testing.T) {
	// 非法 JSON 被 gjson.ValidBytes 拦截
	require.False(t, gjson.ValidBytes([]byte(`{invalid json`)))

	// model 为数字 → 类型不是 gjson.String，应被拒绝
	body := []byte(`{"model":123}`)
	modelResult := gjson.GetBytes(body, "model")
	require.True(t, modelResult.Exists())
	require.NotEqual(t, gjson.String, modelResult.Type)

	// model 为 null → 类型不是 gjson.String，应被拒绝
	body2 := []byte(`{"model":null}`)
	modelResult2 := gjson.GetBytes(body2, "model")
	require.True(t, modelResult2.Exists())
	require.NotEqual(t, gjson.String, modelResult2.Type)

	// stream 为 string → 类型既不是 True 也不是 False，应被拒绝
	body3 := []byte(`{"model":"gpt-4","stream":"true"}`)
	streamResult := gjson.GetBytes(body3, "stream")
	require.True(t, streamResult.Exists())
	require.NotEqual(t, gjson.True, streamResult.Type)
	require.NotEqual(t, gjson.False, streamResult.Type)

	// stream 为 int → 同上
	body4 := []byte(`{"model":"gpt-4","stream":1}`)
	streamResult2 := gjson.GetBytes(body4, "stream")
	require.True(t, streamResult2.Exists())
	require.NotEqual(t, gjson.True, streamResult2.Type)
	require.NotEqual(t, gjson.False, streamResult2.Type)
}

// TestOpenAIHandler_InstructionsInjection 验证 instructions 的 gjson/sjson 注入逻辑
func TestOpenAIHandler_InstructionsInjection(t *testing.T) {
	// 测试 1：无 instructions → 注入
	body := []byte(`{"model":"gpt-4"}`)
	existing := gjson.GetBytes(body, "instructions").String()
	require.Empty(t, existing)
	newBody, err := sjson.SetBytes(body, "instructions", "test instruction")
	require.NoError(t, err)
	require.Equal(t, "test instruction", gjson.GetBytes(newBody, "instructions").String())

	// 测试 2：已有 instructions → 不覆盖
	body2 := []byte(`{"model":"gpt-4","instructions":"existing"}`)
	existing2 := gjson.GetBytes(body2, "instructions").String()
	require.Equal(t, "existing", existing2)

	// 测试 3：空白 instructions → 注入
	body3 := []byte(`{"model":"gpt-4","instructions":"   "}`)
	existing3 := strings.TrimSpace(gjson.GetBytes(body3, "instructions").String())
	require.Empty(t, existing3)

	// 测试 4：sjson.SetBytes 返回错误时不应 panic
	// 正常 JSON 不会产生 sjson 错误，验证返回值被正确处理
	validBody := []byte(`{"model":"gpt-4"}`)
	result, setErr := sjson.SetBytes(validBody, "instructions", "hello")
	require.NoError(t, setErr)
	require.True(t, gjson.ValidBytes(result))
}

func newOpenAIHandlerForPreviousResponseIDValidation(t *testing.T, cache *concurrencyCacheMock) *OpenAIGatewayHandler {
	t.Helper()
	if cache == nil {
		cache = &concurrencyCacheMock{
			acquireUserSlotFn: func(ctx context.Context, userID int64, maxConcurrency int, requestID string) (bool, error) {
				return true, nil
			},
			acquireAccountSlotFn: func(ctx context.Context, accountID int64, maxConcurrency int, requestID string) (bool, error) {
				return true, nil
			},
		}
	}
	return &OpenAIGatewayHandler{
		gatewayService:      &service.OpenAIGatewayService{},
		billingCacheService: &service.BillingCacheService{},
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper:   NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second),
	}
}

func newOpenAIWSHandlerTestServer(t *testing.T, h *OpenAIGatewayHandler, subject middleware.AuthSubject) *httptest.Server {
	t.Helper()
	groupID := int64(2)
	apiKey := &service.APIKey{
		ID:      101,
		GroupID: &groupID,
		User:    &service.User{ID: subject.UserID},
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), apiKey)
		c.Set(string(middleware.ContextKeyUser), subject)
		c.Next()
	})
	router.GET("/openai/v1/responses", h.ResponsesWebSocket)
	return httptest.NewServer(router)
}
